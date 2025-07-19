package collectors

import (
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/config"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/clients"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/logs"
)

// If MetricExport is in clients:
type CloudEyeCollector struct {
	client   *clients.Clients
	cfg      *config.Config
	services []string
}

func NewCloudEyeCollector(cfg *config.Config, services []string) *CloudEyeCollector {
	return &CloudEyeCollector{
		cfg:      cfg,
		services: services,
	}
}

func (c *CloudEyeCollector) AttachClient(client *clients.Clients) {
	c.client = client
}

// Describe is a no-op because we use dynamic metrics
func (c *CloudEyeCollector) Describe(ch chan<- *prometheus.Desc) {}

// Collect scrapes CloudEye and publishes metrics to Prometheus
func (c *CloudEyeCollector) Collect(ch chan<- prometheus.Metric) {
	if c.client == nil {
		logs.Warn("No CloudEye client attached to collector")
		return
	}

	for _, namespace := range c.services {
		metricData := ExportMetricValuesBatch(c.client, c.cfg, namespace)

		// Keep track of seen metrics to avoid duplicates
		seenMetrics := make(map[string]struct{})

		for _, m := range metricData {
			// Create labels slice for the metric (without constant labels)
			labels := make([]string, 0, len(m.Labels)) // Only variable labels here
			values := make([]string, 0, len(m.Labels))

			// Add existing labels from the metric (this will be the variable labels)
			for k, v := range m.Labels {
				// Exclude constant labels from being added to the variable labels
				if k != "resource_id" && k != "resource_name" && k != "unit" {
					labels = append(labels, k)
					values = append(values, v)
				}
			}

			// Extract resource_id, resource_name, and unit from the metric data if available
			resourceID, resourceName := m.Labels["resource_id"], m.Labels["resource_name"]
			unit := m.Unit

			// If any of those values are missing, provide a fallback value
			if resourceID == "" {
				resourceID = "unknown"
			}
			if resourceName == "" {
				resourceName = "unknown"
			}
			if unit == "" {
				unit = "unknown"
			}

			// Define constant labels and values (resource_id, resource_name, unit)
			constantLabels := []string{"resource_id", "resource_name", "unit"}
			constantValues := []string{resourceID, resourceName, unit}

			// Convert namespace and metric name to the desired format
			parts := strings.Split(namespace, ".")
			service := strings.ToLower(parts[len(parts)-1])
			metricNameLower := strings.ToLower(m.MetricName)

			// Create the full metric name with prefix
			metricName := fmt.Sprintf("%s_%s", service, metricNameLower)

			// Check if the metric with the same labels was already sent
			labelKey := fmt.Sprintf("%s-%s-%s-%s-%s", metricName, resourceID, resourceName, unit, m.MetricName)
			if _, exists := seenMetrics[labelKey]; exists {
				continue
			}

			// Add the metric to the seen metrics map to prevent future duplicates
			seenMetrics[labelKey] = struct{}{}

			desc := prometheus.NewDesc(metricName, "Metric description", append(labels, constantLabels...), nil)

			// Log each metric as it is published
			logs.Debugf("Publishing metric: %s value=%.2f labels=%v", metricName, m.Value, append(values, constantValues...))

			// Send the metric to Prometheus with the most recent value
			ch <- prometheus.MustNewConstMetric(
				desc,
				prometheus.GaugeValue,
				m.Value,
				append(values, constantValues...)...,
			)
		}
	}
}
