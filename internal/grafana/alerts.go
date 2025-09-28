package grafana

import (
	"fmt"
	"strings"
	"time"

	"github.com/abdo-farag/otc-cloudeye-exporter/internal/collector"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/logs"
	cesModel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1/model"
)

// AlertRule represents a Grafana alert rule
type AlertRule struct {
	UID             string            `json:"uid"`
	Title           string            `json:"title"`
	Condition       string            `json:"condition"`
	Data            []AlertQuery      `json:"data"`
	IntervalSeconds int64             `json:"intervalSeconds"`
	NoDataState     string            `json:"noDataState"`
	ExecErrState    string            `json:"execErrState"`
	For             string            `json:"for"`
	Annotations     map[string]string `json:"annotations"`
	Labels          map[string]string `json:"labels"`
}

// AlertQuery represents a query used in alert rules
type AlertQuery struct {
	RefID             string            `json:"refId"`
	QueryType         string            `json:"queryType"`
	RelativeTimeRange RelativeTimeRange `json:"relativeTimeRange"`
	Model             AlertQueryModel   `json:"model"`
}

// RelativeTimeRange defines the time range for alert queries
type RelativeTimeRange struct {
	From int64 `json:"from"`
	To   int64 `json:"to"`
}

// AlertQueryModel contains the actual query expression
type AlertQueryModel struct {
	Expr          string `json:"expr"`
	IntervalMs    int64  `json:"intervalMs"`
	MaxDataPoints int64  `json:"maxDataPoints"`
	RefID         string `json:"refId"`
}

// AlertRuleGroup contains a group of related alert rules
type AlertRuleGroup struct {
	Name     string      `json:"name"`
	Interval string      `json:"interval"`
	Rules    []AlertRule `json:"rules"`
}

// AlertBundle contains all alert rules for a namespace
type AlertBundle struct {
	Groups []AlertRuleGroup `json:"groups"`
}

// AlertThresholds defines common thresholds for different metric types
type AlertThresholds struct {
	CPUWarning      float64
	CPUCritical     float64
	MemoryWarning   float64
	MemoryCritical  float64
	DiskWarning     float64
	DiskCritical    float64
	NetworkWarning  float64
	NetworkCritical float64
}

func DefaultThresholds() AlertThresholds {
	return AlertThresholds{
		CPUWarning:      80.0,
		CPUCritical:     95.0,
		MemoryWarning:   85.0,
		MemoryCritical:  95.0,
		DiskWarning:     80.0,
		DiskCritical:    90.0,
		NetworkWarning:  1000000000,
		NetworkCritical: 5000000000,
	}
}

func NewAlertBundle(namespace string) *AlertBundle {
	logs.Infof("Creating new alert bundle for namespace: %s", namespace)
	return &AlertBundle{
		Groups: []AlertRuleGroup{},
	}
}

func (ab *AlertBundle) AddFromMetricValues(ns string, metrics []collector.MetricExport) {
	logs.Infof("Adding alert rules from %d metric exports for namespace: %s", len(metrics), ns)
	if len(metrics) == 0 {
		logs.Warnf("No metrics provided for alert rule creation in namespace: %s", ns)
		return
	}
	grouped := ab.groupMetricsByType(metrics)
	service := strings.ToLower(ns[strings.LastIndex(ns, ".")+1:])
	thresholds := DefaultThresholds()
	for metricType, metricList := range grouped {
		logs.Debugf("Creating alert group for metric type: %s (%d metrics)", metricType, len(metricList))
		group := AlertRuleGroup{
			Name:     fmt.Sprintf("%s_%s_alerts", service, metricType),
			Interval: "1m",
			Rules:    []AlertRule{},
		}
		for _, metric := range metricList {
			warningRule := ab.createAlertRule(ns, service, metric, "warning", thresholds)
			criticalRule := ab.createAlertRule(ns, service, metric, "critical", thresholds)
			if warningRule != nil {
				group.Rules = append(group.Rules, *warningRule)
				logs.Debugf("Added warning alert rule for metric: %s", metric.MetricName)
			}
			if criticalRule != nil {
				group.Rules = append(group.Rules, *criticalRule)
				logs.Debugf("Added critical alert rule for metric: %s", metric.MetricName)
			}
		}
		if len(group.Rules) > 0 {
			ab.Groups = append(ab.Groups, group)
			logs.Infof("Added alert group '%s' with %d rules", group.Name, len(group.Rules))
		}
	}
	logs.Infof("Successfully created %d alert groups for namespace: %s", len(ab.Groups), ns)
}

func (ab *AlertBundle) AddFromMetricInfo(ns string, metrics []cesModel.MetricInfoList) {
	logs.Infof("Adding alert rules from %d CES metrics for namespace: %s", len(metrics), ns)
	if len(metrics) == 0 {
		logs.Warnf("No CES metrics provided for alert rule creation in namespace: %s", ns)
		return
	}
	service := strings.ToLower(ns[strings.LastIndex(ns, ".")+1:])
	thresholds := DefaultThresholds()
	grouped := ab.groupCESMetricsByType(metrics)
	for metricType, metricList := range grouped {
		logs.Debugf("Creating CES alert group for metric type: %s (%d metrics)", metricType, len(metricList))
		group := AlertRuleGroup{
			Name:     fmt.Sprintf("%s_%s_alerts", service, metricType),
			Interval: "1m",
			Rules:    []AlertRule{},
		}
		for _, metric := range metricList {
			warningRule := ab.createCESAlertRule(ns, service, metric, "warning", thresholds)
			criticalRule := ab.createCESAlertRule(ns, service, metric, "critical", thresholds)
			if warningRule != nil {
				group.Rules = append(group.Rules, *warningRule)
				logs.Debugf("Added warning CES alert rule for metric: %s", metric.MetricName)
			}
			if criticalRule != nil {
				group.Rules = append(group.Rules, *criticalRule)
				logs.Debugf("Added critical CES alert rule for metric: %s", metric.MetricName)
			}
		}
		if len(group.Rules) > 0 {
			ab.Groups = append(ab.Groups, group)
			logs.Infof("Added CES alert group '%s' with %d rules", group.Name, len(group.Rules))
		}
	}
	logs.Infof("Successfully created %d CES alert groups for namespace: %s", len(ab.Groups), ns)
}

func (ab *AlertBundle) groupMetricsByType(metrics []collector.MetricExport) map[string][]collector.MetricExport {
	logs.Debugf("Grouping %d metrics by type", len(metrics))
	grouped := make(map[string][]collector.MetricExport)
	for _, metric := range metrics {
		metricType := ab.determineMetricType(metric.MetricName)
		grouped[metricType] = append(grouped[metricType], metric)
	}
	logs.Debugf("Grouped metrics into %d types", len(grouped))
	return grouped
}

func (ab *AlertBundle) groupCESMetricsByType(metrics []cesModel.MetricInfoList) map[string][]cesModel.MetricInfoList {
	logs.Debugf("Grouping %d CES metrics by type", len(metrics))
	grouped := make(map[string][]cesModel.MetricInfoList)
	for _, metric := range metrics {
		metricType := ab.determineMetricType(metric.MetricName)
		grouped[metricType] = append(grouped[metricType], metric)
	}
	logs.Debugf("Grouped CES metrics into %d types", len(grouped))
	return grouped
}

func (ab *AlertBundle) determineMetricType(metricName string) string {
	lowerName := strings.ToLower(metricName)
	switch {
	case strings.Contains(lowerName, "cpu"):
		return "cpu"
	case strings.Contains(lowerName, "memory") || strings.Contains(lowerName, "mem"):
		return "memory"
	case strings.Contains(lowerName, "disk") || strings.Contains(lowerName, "storage"):
		return "disk"
	case strings.Contains(lowerName, "network") || strings.Contains(lowerName, "net") || strings.Contains(lowerName, "bandwidth") || strings.Contains(lowerName, "bytes"):
		return "network"
	case strings.Contains(lowerName, "connection") || strings.Contains(lowerName, "conn"):
		return "connection"
	case strings.Contains(lowerName, "error") || strings.Contains(lowerName, "fail"):
		return "error"
	default:
		return "general"
	}
}

func (ab *AlertBundle) createAlertRule(ns, service string, metric collector.MetricExport, severity string, thresholds AlertThresholds) *AlertRule {
	metricType := ab.determineMetricType(metric.MetricName)
	threshold := ab.getThreshold(metricType, severity, thresholds)
	if threshold == 0 {
		logs.Debugf("No threshold defined for metric type '%s' with severity '%s'", metricType, severity)
		return nil
	}
	operator := ab.getOperator(metricType)
	uid := generateAlertUID(service, metric.MetricName, severity)
	logs.Debugf("Creating alert rule: UID=%s, Metric=%s, Severity=%s, Threshold=%.2f", uid, metric.MetricName, severity, threshold)
	rule := &AlertRule{
		UID:             uid,
		Title:           fmt.Sprintf("%s %s %s Alert", formatTitle(metric.MetricName), strings.Title(service), strings.Title(severity)),
		Condition:       "B",
		IntervalSeconds: 60,
		NoDataState:     "NoData",
		ExecErrState:    "Alerting",
		For:             ab.getAlertDuration(severity),
		Data: []AlertQuery{
			{
				RefID:             "A",
				QueryType:         "",
				RelativeTimeRange: RelativeTimeRange{From: 600, To: 0},
				Model: AlertQueryModel{
					Expr:          fmt.Sprintf(`%s_%s{namespace="%s"}`, service, metric.MetricName, ns),
					IntervalMs:    1000,
					MaxDataPoints: 43200,
					RefID:         "A",
				},
			},
			{
				RefID:             "B",
				QueryType:         "",
				RelativeTimeRange: RelativeTimeRange{From: 0, To: 0},
				Model: AlertQueryModel{
					Expr:          fmt.Sprintf("last(A) %s %.2f", operator, threshold),
					IntervalMs:    1000,
					MaxDataPoints: 43200,
					RefID:         "B",
				},
			},
		},
		Annotations: map[string]string{
			"description": fmt.Sprintf("%s %s is %s %.2f%s for {{ $labels.resource_name }}", formatTitle(metric.MetricName), strings.Title(service), ab.getOperatorText(operator), threshold, metric.Unit),
			"summary":     fmt.Sprintf("%s %s Alert", formatTitle(metric.MetricName), strings.Title(severity)),
		},
		Labels: map[string]string{
			"severity":  severity,
			"service":   service,
			"namespace": ns,
			"metric":    metric.MetricName,
		},
	}
	return rule
}

func (ab *AlertBundle) createCESAlertRule(ns, service string, metric cesModel.MetricInfoList, severity string, thresholds AlertThresholds) *AlertRule {
	metricType := ab.determineMetricType(metric.MetricName)
	threshold := ab.getThreshold(metricType, severity, thresholds)
	if threshold == 0 {
		logs.Debugf("No threshold defined for CES metric type '%s' with severity '%s'", metricType, severity)
		return nil
	}
	operator := ab.getOperator(metricType)
	uid := generateAlertUID(service, metric.MetricName, severity)
	logs.Debugf("Creating CES alert rule: UID=%s, Metric=%s, Severity=%s, Threshold=%.2f", uid, metric.MetricName, severity, threshold)
	rule := &AlertRule{
		UID:             uid,
		Title:           fmt.Sprintf("%s %s %s Alert", formatTitle(metric.MetricName), strings.Title(service), strings.Title(severity)),
		Condition:       "B",
		IntervalSeconds: 60,
		NoDataState:     "NoData",
		ExecErrState:    "Alerting",
		For:             ab.getAlertDuration(severity),
		Data: []AlertQuery{
			{
				RefID:             "A",
				QueryType:         "",
				RelativeTimeRange: RelativeTimeRange{From: 600, To: 0},
				Model: AlertQueryModel{
					Expr:          fmt.Sprintf(`%s_%s{namespace="%s"}`, service, metric.MetricName, ns),
					IntervalMs:    1000,
					MaxDataPoints: 43200,
					RefID:         "A",
				},
			},
			{
				RefID:             "B",
				QueryType:         "",
				RelativeTimeRange: RelativeTimeRange{From: 0, To: 0},
				Model: AlertQueryModel{
					Expr:          fmt.Sprintf("last(A) %s %.2f", operator, threshold),
					IntervalMs:    1000,
					MaxDataPoints: 43200,
					RefID:         "B",
				},
			},
		},
		Annotations: map[string]string{
			"description": fmt.Sprintf("%s %s is %s %.2f for {{ $labels.resource_name }}", formatTitle(metric.MetricName), strings.Title(service), ab.getOperatorText(operator), threshold),
			"summary":     fmt.Sprintf("%s %s Alert", formatTitle(metric.MetricName), strings.Title(severity)),
		},
		Labels: map[string]string{
			"severity":  severity,
			"service":   service,
			"namespace": ns,
			"metric":    metric.MetricName,
		},
	}
	return rule
}

func generateAlertUID(service, metricName, severity string) string {
	return fmt.Sprintf("%s_%s_%s_%d", service, metricName, severity, time.Now().Unix())
}

func (ab *AlertBundle) getThreshold(metricType, severity string, thresholds AlertThresholds) float64 {
	switch metricType {
	case "cpu":
		if severity == "warning" {
			return thresholds.CPUWarning
		}
		return thresholds.CPUCritical
	case "memory":
		if severity == "warning" {
			return thresholds.MemoryWarning
		}
		return thresholds.MemoryCritical
	case "disk":
		if severity == "warning" {
			return thresholds.DiskWarning
		}
		return thresholds.DiskCritical
	case "network":
		if severity == "warning" {
			return thresholds.NetworkWarning
		}
		return thresholds.NetworkCritical
	default:
		return 0
	}
}

func (ab *AlertBundle) getOperator(metricType string) string {
	switch metricType {
	case "error":
		return ">"
	default:
		return ">"
	}
}

func (ab *AlertBundle) getOperatorText(operator string) string {
	switch operator {
	case ">":
		return "above"
	case "<":
		return "below"
	case ">=":
		return "at or above"
	case "<=":
		return "at or below"
	case "==":
		return "equal to"
	case "!=":
		return "not equal to"
	default:
		return "above"
	}
}

func (ab *AlertBundle) getAlertDuration(severity string) string {
	switch severity {
	case "critical":
		return "1m"
	case "warning":
		return "5m"
	default:
		return "5m"
	}
}
