package grafana

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/abdo-farag/otc-cloudeye-exporter/internal/collector"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/logs"
	cesModel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1/model"
)

type Dashboard struct {
	Title      string     `json:"title"`
	UID        string     `json:"uid,omitempty"`
	Schema     int        `json:"schemaVersion"`
	Templating Templating `json:"templating"`
	Panels     []Panel    `json:"panels"`
}

type Templating struct {
	List []TemplateVar `json:"list"`
}

type TemplateVar struct {
	Name       string          `json:"name"`
	Label      string          `json:"label"`
	Type       string          `json:"type"`
	Query      string          `json:"query"`
	Multi      bool            `json:"multi"`
	IncludeAll bool            `json:"includeAll"`
	Current    TemplateCurrent `json:"current"`
}

type TemplateCurrent struct {
	Text  string `json:"text"`
	Value string `json:"value"`
}

type Panel struct {
	Id          int           `json:"id"`
	Title       string        `json:"title"`
	Type        string        `json:"type"`
	GridPos     GridPosition  `json:"gridPos"`
	Targets     []PanelTarget `json:"targets"`
	FieldConfig *FieldConfig  `json:"fieldConfig,omitempty"`
	Repeat      string        `json:"repeat,omitempty"`
}

type GridPosition struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

type PanelTarget struct {
	Expr         string `json:"expr"`
	RefID        string `json:"refId"`
	LegendFormat string `json:"legendFormat,omitempty"`
}

type FieldConfig struct {
	Defaults FieldDefaults `json:"defaults"`
}

type FieldDefaults struct {
	Unit string `json:"unit"`
}

func generateNumericUID(length int) string {
	rand.Seed(time.Now().UnixNano())
	uid := ""
	for i := 0; i < length; i++ {
		uid += fmt.Sprintf("%d", rand.Intn(10))
	}
	logs.Debugf("Generated dashboard UID: %s", uid)
	return uid
}

func NewDefaultDashboard(namespace string) *Dashboard {
	logs.Infof("Creating new Grafana dashboard for namespace: %s", namespace)
	
	dashboard := &Dashboard{
		Title:  fmt.Sprintf("CloudEye - %s", namespace),
		UID:    generateNumericUID(12),
		Schema: 36,
		Templating: Templating{
			List: []TemplateVar{
				newTemplateVar("domain_name", "Domain Name", namespace),
				newTemplateVar("project_name", "Project Name", namespace),
				newTemplateVar("resource_name", "Resource Name", namespace),
			},
		},
		Panels: []Panel{},
	}
	
	logs.Debugf("Dashboard created with UID: %s, Title: %s", dashboard.UID, dashboard.Title)
	return dashboard
}

func newTemplateVar(name, label, namespace string) TemplateVar {
	var query string

	if name == "resource_name" {
		// Label-only match: no metric name used
		query = fmt.Sprintf(`label_values({namespace="%s"}, resource_name)`, namespace)
	} else {
		query = fmt.Sprintf("label_values(%s)", name)
	}

	logs.Debugf("Creating template variable: name=%s, label=%s, query=%s", name, label, query)

	return TemplateVar{
		Name:       name,
		Label:      label,
		Type:       "query",
		Query:      query,
		Multi:      true,
		IncludeAll: true,
		Current: TemplateCurrent{
			Text:  "All",
			Value: "$__all",
		},
	}
}

// AddFromMetricValues builds panels from ExportMetricValuesBatch data
func (d *Dashboard) AddFromMetricValues(ns string, metrics []collector.MetricExport) {
	logs.Infof("Adding panels from %d metric exports for namespace: %s", len(metrics), ns)
	
	if len(metrics) == 0 {
		logs.Warnf("No metrics provided for dashboard creation in namespace: %s", ns)
		return
	}

	grouped := map[string][]collector.MetricExport{}

	// Group by metric name
	for _, m := range metrics {
		grouped[m.MetricName] = append(grouped[m.MetricName], m)
	}

	logs.Debugf("Grouped metrics into %d unique metric names", len(grouped))

	x, y, panelID := 0, 0, 0
	service := strings.ToLower(ns[strings.LastIndex(ns, ".")+1:])

	for metricName, exports := range grouped {
		if len(exports) == 0 {
			logs.Warnf("No exports found for metric: %s", metricName)
			continue
		}

		unit := exports[0].Unit
		logs.Debugf("Processing metric: %s with unit: %s (%d exports)", metricName, unit, len(exports))
		
		panelType := determinePanelType(unit)
		title := fmt.Sprintf("%s (%s)", formatTitle(metricName), unit)

		panel := Panel{
			Id:    panelID,
			Title: title,
			Type:  panelType,
			GridPos: GridPosition{
				X: x,
				Y: y,
				W: 24,
				H: 8,
			},
			FieldConfig: &FieldConfig{
				Defaults: FieldDefaults{Unit: unit},
			},
			Targets: []PanelTarget{{
				Expr:         fmt.Sprintf(`%s_%s{namespace="%s", domain_name=~"$domain_name", project_name=~"$project_name", resource_name=~"$resource_name"}`, service, metricName, ns),
				RefID:        "A",
				LegendFormat: "{{resource_name}}",
			}},
		}

		d.Panels = append(d.Panels, panel)
		logs.Infof("Added panel: ID=%d, Title='%s', Type=%s at position (x=%d, y=%d)", panelID, title, panelType, x, y)
		
		panelID++
		y += 8
	}
	
	logs.Infof("Successfully added %d panels to dashboard for namespace: %s", len(d.Panels), ns)
}

// Grouped panel addition
func (d *Dashboard) AddAllMetricsGrouped(ns string, metrics []cesModel.MetricInfoList) {
	logs.Infof("Adding grouped metrics for namespace: %s (%d total metrics)", ns, len(metrics))
	
	if len(metrics) == 0 {
		logs.Warnf("No metrics provided for grouped dashboard creation in namespace: %s", ns)
		return
	}

	grouped := map[string]cesModel.MetricInfoList{}

	for _, m := range metrics {
		if _, exists := grouped[m.MetricName]; !exists {
			grouped[m.MetricName] = m
		}
	}

	logs.Debugf("Grouped %d metrics into %d unique metric names", len(metrics), len(grouped))

	var gaugeMetrics, timeSeriesMetrics []cesModel.MetricInfoList
	for _, m := range grouped {
		if determinePanelType(m.Unit) == "gauge" {
			gaugeMetrics = append(gaugeMetrics, m)
		} else {
			timeSeriesMetrics = append(timeSeriesMetrics, m)
		}
	}

	logs.Infof("Categorized metrics: %d gauge panels, %d timeseries panels", len(gaugeMetrics), len(timeSeriesMetrics))

	x, y := 0, 0
	panelID := 0
	maxY := 0

	// Add gauge panels
	for _, m := range gaugeMetrics {
		for _, dim := range m.Dimensions {
			if dim.Name == "resource_name" {
				width := 6
				logs.Debugf("Adding gauge panel for metric: %s at position (x=%d, y=%d)", m.MetricName, x, y)
				
				d.AddGaugePerResourcePanel(
					strings.ToLower(ns[strings.LastIndex(ns, ".")+1:]),
					m.MetricName, ns, m.Unit,
					panelID, x, y, width,
				)
				panelID++
				if x == 18 {
					x = 0
					y += 8
				} else {
					x += 6
				}
				if y > maxY {
					maxY = y
				}
				break // Only need one resource_name dimension
			}
		}
	}

	// Reset layout for timeseries panels
	y = maxY + 8
	x = 0

	logs.Debugf("Starting timeseries panels at y position: %d", y)

	for _, m := range timeSeriesMetrics {
		logs.Debugf("Adding timeseries panel for metric: %s at position (x=%d, y=%d)", m.MetricName, x, y)
		d.AddMetricPanel(ns, m, panelID, x, y, 24)
		panelID++
		y += 8
	}
	
	logs.Infof("Successfully added %d total panels (%d gauges, %d timeseries) to dashboard", panelID, len(gaugeMetrics), len(timeSeriesMetrics))
}

func (d *Dashboard) AddMetricPanel(ns string, m cesModel.MetricInfoList, id, x, y, width int) {
	height := 8

	unit := m.Unit
	panelType := determinePanelType(unit)
	service := strings.ToLower(ns[strings.LastIndex(ns, ".")+1:])
	title := fmt.Sprintf("%s (%s)", formatTitle(m.MetricName), unit)

	logs.Debugf("Creating metric panel: ID=%d, Metric=%s, Type=%s, Unit=%s", id, m.MetricName, panelType, unit)

	panel := Panel{
		Id:    id,
		Title: title,
		Type:  panelType,
		GridPos: GridPosition{
			X: x,
			Y: y,
			W: width,
			H: height,
		},
		Targets: []PanelTarget{{
			Expr:         fmt.Sprintf(`%s_%s{namespace="%s", domain_name=~"$domain_name", project_name=~"$project_name", resource_name=~"$resource_name"}`, service, m.MetricName, ns),
			RefID:        "A",
			LegendFormat: "{{resource_name}}",
		}},
		FieldConfig: &FieldConfig{
			Defaults: FieldDefaults{
				Unit: unit,
			},
		},
	}
	d.Panels = append(d.Panels, panel)
	logs.Debugf("Added metric panel: %s with dimensions at grid position (x=%d, y=%d, w=%d, h=%d)", title, x, y, width, height)
}

func (d *Dashboard) AddGaugePerResourcePanel(service, metricName, ns, unit string, id, x, y, width int) {
	height := 8
	title := fmt.Sprintf("%s (%s)", formatTitle(metricName), unit)

	logs.Debugf("Creating gauge panel: ID=%d, Metric=%s, Service=%s, Unit=%s", id, metricName, service, unit)

	panel := Panel{
		Id:    id,
		Title: title,
		Type:  "gauge",
		GridPos: GridPosition{
			X: x,
			Y: y,
			W: width,
			H: height,
		},
		Targets: []PanelTarget{{
			Expr:         fmt.Sprintf(`%s_%s{namespace="%s", resource_name=~"$resource_name"}`, service, metricName, ns),
			RefID:        "A",
			LegendFormat: "{{resource_name}}",
		}},
		FieldConfig: &FieldConfig{
			Defaults: FieldDefaults{
				Unit: unit,
			},
		},
	}
	d.Panels = append(d.Panels, panel)
	logs.Debugf("Added gauge panel: %s at grid position (x=%d, y=%d, w=%d, h=%d)", title, x, y, width, height)
}

func formatTitle(metric string) string {
	parts := strings.Split(metric, "_")
	for i := range parts {
		parts[i] = strings.Title(strings.ToLower(parts[i]))
	}
	formattedTitle := strings.Join(parts, " ")
	logs.Debugf("Formatted metric title: '%s' -> '%s'", metric, formattedTitle)
	return formattedTitle
}

func determinePanelType(unit string) string {
	var panelType string
	switch unit {
	case "%":
		panelType = "gauge"
	default:
		panelType = "timeseries"
	}
	logs.Debugf("Determined panel type for unit '%s': %s", unit, panelType)
	return panelType
}