package collector

import (
	"fmt"
	"strings"

	"github.com/abdo-farag/otc-cloudeye-exporter/internal/clients"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/config"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/constants"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/logs"
	"github.com/prometheus/client_golang/prometheus"
)

type CloudEyeCollector struct {
	client   *clients.Clients
	cfg      *config.Config
	services []string
}

func NewCloudEyeCollector(cfg *config.Config, services []string) *CloudEyeCollector {
	// Validate services against supported namespaces
	validServices := make([]string, 0, len(services))
	for _, service := range services {
		if isValidNamespace(service) {
			validServices = append(validServices, service)
		} else {
			logs.Warnf("Invalid/unsupported namespace: %s", service)
		}
	}

	return &CloudEyeCollector{
		cfg:      cfg,
		services: validServices,
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
		metricData := ExportMetricValuesBatch(c.client, c.cfg, namespace, c.client.ProjectName)
		// Keep track of seen metrics to avoid duplicates
		seenMetrics := make(map[string]struct{})
		for _, m := range metricData {
			// Create labels slice for the metric (without constant labels)
			labels := make([]string, 0, len(m.Labels))
			values := make([]string, 0, len(m.Labels))
			// Add existing labels from the metric (exclude constant labels)
			for k, v := range m.Labels {
				if !isConstantLabel(k) {
					labels = append(labels, k)
					values = append(values, v)
				}
			}
			// Extract constant labels with fallbacks
			resourceID := getValueOrDefault(m.Labels[constants.LabelResourceID], constants.ResourceIDUnknown)
			resourceName := getValueOrDefault(m.Labels[constants.LabelResourceName], constants.ResourceIDUnknown)
			unit := getValueOrDefault(m.Unit, constants.ResourceIDUnknown)
			// Define constant labels and values
			constantLabels := []string{constants.LabelResourceID, constants.LabelResourceName, constants.LabelUnit}
			constantValues := []string{resourceID, resourceName, unit}
			// Create metric name using namespace mapping
			metricName := createMetricName(namespace, m.MetricName)
			// Check for duplicates
			labelKey := fmt.Sprintf("%s-%s-%s-%s-%s", metricName, resourceID, resourceName, unit, m.MetricName)
			if _, exists := seenMetrics[labelKey]; exists {
				continue
			}
			seenMetrics[labelKey] = struct{}{}
			desc := prometheus.NewDesc(metricName, "CloudEye metric", append(labels, constantLabels...), nil)
			logs.Debugf("Publishing metric: %s value=%.2f labels=%v", metricName, m.Value, append(values, constantValues...))
			ch <- prometheus.MustNewConstMetric(
				desc,
				prometheus.GaugeValue,
				m.Value,
				append(values, constantValues...)...,
			)
		}
	}
}

// Helper functions

func isValidNamespace(namespace string) bool {
	for _, validNs := range constants.AllNamespaces {
		if namespace == validNs {
			return true
		}
	}
	return false
}

func isConstantLabel(key string) bool {
	constantLabels := []string{constants.LabelResourceID, constants.LabelResourceName, constants.LabelUnit}
	for _, label := range constantLabels {
		if key == label {
			return true
		}
	}
	return false
}

func getValueOrDefault(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

func createMetricName(namespace, metricName string) string {
	// Convert namespace to service name
	parts := strings.Split(namespace, ".")
	service := strings.ToLower(parts[len(parts)-1])
	metricNameLower := strings.ToLower(metricName)
	return fmt.Sprintf("%s_%s", service, metricNameLower)
}
