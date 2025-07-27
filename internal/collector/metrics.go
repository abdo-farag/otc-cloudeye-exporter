package collector

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/abdo-farag/otc-cloudeye-exporter/internal/clients"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/config"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/logs"
	cesModel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1/model"
)

// -----------------------------
// Data Types
// -----------------------------

type MetricExport struct {
	MetricName string
	Labels     map[string]string
	Value      float64
	Unit       string
	Timestamp  time.Time
}

type RetryConfig struct {
	MaxRetries        int
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	BackoffMultiplier float64
	RetryableErrors   []string
}

type Volume struct {
	Id          string       `json:"id"`
	Attachments []Attachment `json:"attachments"`
}

type Attachment struct {
	ServerId string `json:"server_id"`
	Device   string `json:"device"` // like "/dev/vda"
}

// -----------------------------
// Retry Logic
// -----------------------------

func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:        5,
		InitialBackoff:    5 * time.Second,
		MaxBackoff:        2 * time.Minute,
		BackoffMultiplier: 2.0,
		RetryableErrors: []string{
			"408", "429", "500", "503", "timeout",
			"connection reset", "connection refused",
		},
	}
}

func (rc *RetryConfig) shouldRetry(err error, attempt int) bool {
	if err == nil || attempt >= rc.MaxRetries {
		return false
	}
	errStr := strings.ToLower(err.Error())
	for _, retryableErr := range rc.RetryableErrors {
		if strings.Contains(errStr, retryableErr) {
			return true
		}
	}
	return false
}

func (rc *RetryConfig) getBackoffDuration(attempt int) time.Duration {
	backoff := float64(rc.InitialBackoff) * math.Pow(rc.BackoffMultiplier, float64(attempt))
	if backoff > float64(rc.MaxBackoff) {
		backoff = float64(rc.MaxBackoff)
	}
	return time.Duration(backoff)
}

func withRetry[T any](operation func() (T, error), config *RetryConfig, operationName string) (T, error) {
	var result T
	var err error
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := config.getBackoffDuration(attempt - 1)
			logs.Warnf("Retrying %s (attempt %d/%d) after %v", operationName, attempt, config.MaxRetries, backoff)
			time.Sleep(backoff)
		}
		result, err = operation()
		if err == nil {
			if attempt > 0 {
				logs.Infof("Successfully completed %s after %d retries", operationName, attempt)
			}
			return result, nil
		}
		if !config.shouldRetry(err, attempt) {
			logs.Errorf("Non-retryable error for %s: %v", operationName, err)
			break
		}
		logs.Warnf("Retryable error for %s (attempt %d): %v", operationName, attempt+1, err)
	}
	return result, fmt.Errorf("operation %s failed after %d attempts: %w", operationName, config.MaxRetries+1, err)
}

// -----------------------------
// Metric Export Logic (main entry)
// -----------------------------

func ExportMetricValuesBatch(client *clients.Clients, cfg *config.Config, namespace string) []MetricExport {
	retryConfig := DefaultRetryConfig()
	metrics, err := withRetry(
		func() ([]cesModel.MetricInfoList, error) {
			return FetchAllMetricDefinitions(client, namespace)
		},
		retryConfig,
		fmt.Sprintf("fetch metrics for namespace %s", namespace),
	)
	if err != nil {
		logs.Errorf("Failed to list metrics after retries: %v", err)
		return nil
	}
	if len(metrics) == 0 {
		logs.Warnf("No metrics found in namespace %s", namespace)
		return nil
	}
	logs.Infof("Listed %d metrics in namespace %s", len(metrics), namespace)

	start := time.Now().Add(-1*time.Hour).Unix() * 1000
	end := time.Now().Unix() * 1000
	period := "1"

	batchResp, err := withRetry(
		func() (*[]cesModel.BatchMetricData, error) {
			return fetchMetricTimeSeries(client, metrics, start, end, period)
		},
		retryConfig,
		fmt.Sprintf("fetch time series data for namespace %s", namespace),
	)
	if err != nil {
		logs.Errorf("Batch metric query failed after retries: %v", err)
		return nil
	}
	if batchResp == nil || len(*batchResp) == 0 {
		logs.Warnf("No time series data returned for namespace %s", namespace)
		return nil
	}

	var (
		results []MetricExport
		mu      sync.Mutex
		wg      sync.WaitGroup
	)

	for _, m := range *batchResp {
		if m.MetricName == "" {
			logs.Warn("Metric with empty name found, skipping")
			continue
		}
		wg.Add(1)
		go func(m cesModel.BatchMetricData) {
			defer wg.Done()
			labels, resourceID := extractLabelsAndResourceID(m, namespace)
			labels, resourceID = handleEVSIfNeeded(labels, resourceID, namespace, client)
			labels = handleOBSIfNeeded(labels, m, namespace, client, retryConfig)
			labels = enrichWithRMSIfNeeded(labels, resourceID, namespace, client, cfg, retryConfig)

			if _, exists := labels["resource_name"]; !exists {
				labels["resource_name"] = "unknown"
			}

			unit := safeUnit(m.Unit)
			localResults := convertDatapointsToExports(m, labels, unit)

			mu.Lock()
			results = append(results, localResults...)
			mu.Unlock()
		}(m)
	}
	wg.Wait()
	logs.Infof("Exported %d metric values for namespace %s", len(results), namespace)
	return results
}

// -----------------------------
// Metric Definition / Fetch Logic
// -----------------------------

func FetchAllMetricDefinitions(client *clients.Clients, namespace string) ([]cesModel.MetricInfoList, error) {
	limit := int32(1000)
	req := &cesModel.ListMetricsRequest{
		Limit:     &limit,
		Namespace: Ptr(namespace),
	}
	var result []cesModel.MetricInfoList
	retryConfig := DefaultRetryConfig()

	for {
		resp, err := withRetry(
			func() (*cesModel.ListMetricsResponse, error) {
				return client.CloudEyeV1.ListMetrics(req)
			},
			retryConfig,
			fmt.Sprintf("list metrics for namespace %s", namespace),
		)
		if err != nil {
			logs.Errorf("ListMetrics error after retries: %v", err)
			return nil, err
		}
		if resp.Metrics == nil || len(*resp.Metrics) == 0 {
			break
		}
		result = append(result, *resp.Metrics...)
		if resp.MetaData == nil || resp.MetaData.Marker == "" {
			break
		}
		req.Start = Ptr(resp.MetaData.Marker)
	}
	return result, nil
}

func fetchMetricTimeSeries(client *clients.Clients, metrics []cesModel.MetricInfoList, from, to int64, period string) (*[]cesModel.BatchMetricData, error) {
	var batchMetrics []cesModel.MetricInfo
	for _, m := range metrics {
		if m.MetricName == "" || m.Namespace == "" || len(m.Dimensions) == 0 {
			continue
		}
		batchMetrics = append(batchMetrics, cesModel.MetricInfo{
			MetricName: m.MetricName,
			Namespace:  m.Namespace,
			Dimensions: m.Dimensions,
		})
	}
	if len(batchMetrics) == 0 {
		logs.Warn("No valid metrics to query.")
		return nil, nil
	}
	req := &cesModel.BatchListMetricDataRequest{
		Body: &cesModel.BatchListMetricDataRequestBody{
			Metrics: batchMetrics,
			From:    from,
			To:      to,
			Period:  period,
			Filter:  "average",
		},
	}
	retryConfig := DefaultRetryConfig()
	resp, err := withRetry(
		func() (*cesModel.BatchListMetricDataResponse, error) {
			return client.CloudEyeV1.BatchListMetricData(req)
		},
		retryConfig,
		"batch list metric data",
	)
	if err != nil {
		logs.Errorf("BatchListMetricData error after retries: %v", err)
		return nil, err
	}
	return resp.Metrics, nil
}

// -----------------------------
// Label/Resource Enrichment
// -----------------------------

func extractLabelsAndResourceID(m cesModel.BatchMetricData, namespace string) (map[string]string, string) {
	labels := make(map[string]string)
	var resourceID string
	labels["namespace"] = namespace
	for _, dim := range safeDimensions(m.Dimensions) {
		key := strings.ToLower(dim.Name)
		value := strings.TrimSpace(dim.Value)
		labels[key] = value
		if strings.HasSuffix(key, "_id") && key != "tenant_id" && key != "project_id" && key != "domain_id" {
			if resourceID == "" || (value != "unknown" && value != "") {
				resourceID = value
			}
		}
	}
	if resourceID == "" {
		firstDimValue := firstDimensionValue(m.Dimensions)
		if firstDimValue != "" {
			labels["resource_id"] = firstDimValue
			resourceID = firstDimValue
		} else {
			labels["resource_id"] = "unknown"
			resourceID = "unknown"
			logs.Warnf("No valid resource_id found for metric %s, using 'unknown'", m.MetricName)
		}
	} else {
		labels["resource_id"] = resourceID
	}
	return labels, resourceID
}

func enrichWithRMSIfNeeded(labels map[string]string, resourceID, namespace string, client *clients.Clients, cfg *config.Config, retryConfig *RetryConfig) map[string]string {
	if client.RMS != nil && resourceID != "" && resourceID != "unknown" && !shouldSkipRMSLookup(namespace, resourceID) {
		rmsResource, err := withRetry(
			func() (map[string]string, error) {
				return client.RMS.GetResourceByID(resourceID, "")
			},
			retryConfig,
			fmt.Sprintf("get RMS resource info for %s", resourceID),
		)
		if rmsResource != nil && err == nil {
			if cfg.Global.ExportRMSLabels["resource_name"] {
				if name := rmsResource["name"]; name != "" {
					labels["resource_name"] = name
				} else if name := rmsResource["resource_name"]; name != "" {
					labels["resource_name"] = name
				}
			}
			if cfg.Global.ExportRMSLabels["project_id"] {
				if pid := rmsResource["projectid"]; pid != "" {
					labels["project_id"] = pid
				}
			}
			if cfg.Global.ExportRMSLabels["project_name"] {
				if pname := rmsResource["projectname"]; pname != "" {
					labels["project_name"] = pname
				}
			}
			if cfg.Global.ExportRMSLabels["domain_name"] {
				labels["domain_name"] = cfg.Auth.DomainName
			}
			if cfg.Global.ExportRMSLabels["tags"] {
				for key, value := range rmsResource {
					if strings.HasPrefix(key, "tag_") && value != "" {
						labels[key] = value
					}
				}
			}
			if rmsID := rmsResource["id"]; rmsID != "" {
				labels["resource_id"] = rmsID
			}
		} else if err != nil {
			logs.Warnf("Failed to fetch RMS info for %s after retries: %v", resourceID, err)
		}
	}
	return labels
}

func handleEVSIfNeeded(labels map[string]string, resourceID, namespace string, client *clients.Clients) (map[string]string, string) {
	if strings.Contains(namespace, "EVS") {
		lastDash := strings.LastIndex(resourceID, "-")
		if lastDash > 0 && lastDash < len(resourceID)-1 {
			vmID := resourceID[:lastDash]
			device := resourceID[lastDash+1:]
			actualDiskID, diskName := lookupEVSID(client, vmID, device)
			if actualDiskID != "" {
				labels["resource_id"] = actualDiskID
				if diskName != "" {
					labels["disk_name"] = diskName
				}
				return labels, actualDiskID
			}
			logs.Warnf("Failed to resolve EVS disk ID for %s", resourceID)
		}
	}
	return labels, resourceID
}

func lookupEVSID(client *clients.Clients, vmID, device string) (string, string) {
	volumes, err := client.ListVolumes()
	if err != nil {
		logs.Errorf("Error fetching EVS volumes: %v", err)
		return "", ""
	}
	devicePath := "/dev/" + device
	for _, vol := range volumes {
		for _, attach := range vol.Attachments {
			if attach.ServerId == vmID && attach.Device == devicePath {
				return vol.Id, vol.Name
			}
		}
	}
	return "", ""
}

func handleOBSIfNeeded(labels map[string]string, m cesModel.BatchMetricData, namespace string, client *clients.Clients, retryConfig *RetryConfig) map[string]string {
	if namespace == "SYS.OBS" {
		bucketName := getBucketNameFromDimensions(m.Dimensions)
		if bucketName != "" {
			labels["bucket_name"] = bucketName

			// ---- Enrich with bucket tags and info ----
			if client.OBS != nil {
				// 1. Try to get tags (handle error gracefully)
				if tags, err := client.OBS.GetBucketTags(bucketName); err == nil {
					for k, v := range tags {
						// Add as label (optional: prefix, like "tag_")
						labels["tag_"+k] = v
					}
					logs.Debugf("Added %d bucket tags to labels for bucket %s", len(tags), bucketName)
				} else {
					logs.Warnf("Could not fetch tags for OBS bucket %s: %v", bucketName, err)
				}
				// 2. Try to get info (like region/location)
				if info, err := client.OBS.GetBucketInfo(bucketName); err == nil {
					for k, v := range info {
						labels[k] = v
					}
					logs.Debugf("Added bucket info labels for bucket %s", bucketName)
				} else {
					logs.Warnf("Could not fetch info for OBS bucket %s: %v", bucketName, err)
				}
			}
			// ---- ----
		} else {
			if tenantID, exists := labels["tenant_id"]; exists && labels["resource_id"] == tenantID {
				labels["resource_name"] = "obs_service"
				labels["resource_id"] = "obs_service"
			} else {
				labels["resource_name"] = labels["resource_id"]
			}
		}
	}
	return labels
}

// -----------------------------
// Metric Data Conversion
// -----------------------------

func convertDatapointsToExports(m cesModel.BatchMetricData, labels map[string]string, unit string) []MetricExport {
	localResults := make([]MetricExport, 0)
	if len(m.Datapoints) == 0 {
		labelsWithUnit := cloneMap(labels)
		if unit != "" {
			labelsWithUnit["unit"] = unit
		}
		localResults = append(localResults, MetricExport{
			MetricName: m.MetricName,
			Labels:     labelsWithUnit,
			Value:      0,
			Unit:       unit,
			Timestamp:  time.Now(),
		})
	} else {
		for _, dp := range m.Datapoints {
			value := 0.0
			if dp.Average != nil {
				value = *dp.Average
			}
			timestamp := time.Unix(0, dp.Timestamp*int64(time.Millisecond))
			labelsWithUnit := cloneMap(labels)
			if unit != "" {
				labelsWithUnit["unit"] = unit
			}
			localResults = append(localResults, MetricExport{
				MetricName: m.MetricName,
				Labels:     labelsWithUnit,
				Value:      value,
				Unit:       unit,
				Timestamp:  timestamp,
			})
		}
	}
	return localResults
}

// -----------------------------
// Helpers (kept for clarity/reuse)
// -----------------------------

func Ptr[T any](v T) *T { return &v }

func safeUnit(u *string) string {
	if u == nil {
		return ""
	}
	return *u
}

func safeDimensions(dims *[]cesModel.MetricsDimension) []cesModel.MetricsDimension {
	if dims == nil {
		return nil
	}
	return *dims
}

func firstDimensionValue(dims *[]cesModel.MetricsDimension) string {
	if dims == nil || len(*dims) == 0 {
		return ""
	}
	return (*dims)[0].Value
}

func cloneMap(in map[string]string) map[string]string {
	out := make(map[string]string)
	for k, v := range in {
		out[k] = v
	}
	return out
}

func isOBSOperationName(value string) bool {
	obsOperations := []string{
		"HEAD_OBJECT", "PUT_OBJECT", "DELETE_OBJECT", "GET_OBJECT",
		"PUT_PART", "POST_UPLOAD_INIT", "POST_UPLOAD_COMPLETE",
		"LIST_OBJECTS", "DELETE_OBJECTS", "COPY_OBJECT",
		"HEAD_BUCKET", "LIST_BUCKET_OBJECTS", "LIST_BUCKET_UPLOADS",
		"GET_BUCKET_LOCATION", "GET_BUCKET_POLICY", "PUT_BUCKET_POLICY",
		"DELETE_BUCKET_POLICY", "GET_BUCKET_ACL", "PUT_BUCKET_ACL",
	}
	for _, op := range obsOperations {
		if value == op {
			return true
		}
	}
	return false
}

func getBucketNameFromDimensions(dims *[]cesModel.MetricsDimension) string {
	if dims == nil {
		return ""
	}
	for _, dim := range *dims {
		if strings.ToLower(dim.Name) == "bucket_name" {
			return dim.Value
		}
	}
	return ""
}

func shouldSkipRMSLookup(namespace string, resourceID string) bool {
	return resourceID == "" || resourceID == "unknown"
}