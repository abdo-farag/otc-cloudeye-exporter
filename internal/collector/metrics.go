package collector

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abdo-farag/otc-cloudeye-exporter/internal/clients"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/config"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/constants"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/logs"
	cesModel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1/model"
)

// Data Types
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
	Name        string       `json:"name"`
	Attachments []Attachment `json:"attachments"`
}

type Attachment struct {
	ServerId string `json:"server_id"`
	Device   string `json:"device"` // like "/dev/vda"
}

type MetricError struct {
	Namespace string
	Operation string
	Err       error
}

func (e *MetricError) Error() string {
	return fmt.Sprintf("metric error in namespace %s during %s: %v", e.Namespace, e.Operation, e.Err)
}

// Label Builder
type LabelBuilder struct {
	labels map[string]string
}

func NewLabelBuilder(namespace string) *LabelBuilder {
	return &LabelBuilder{
		labels: map[string]string{constants.LabelNamespace: namespace},
	}
}

func (lb *LabelBuilder) AddDimensions(dims *[]cesModel.MetricsDimension) *LabelBuilder {
	for _, dim := range safeDimensions(dims) {
		key := strings.ToLower(dim.Name)
		value := strings.TrimSpace(dim.Value)
		lb.labels[key] = value
	}
	return lb
}

func (lb *LabelBuilder) AddLabel(key, value string) *LabelBuilder {
	lb.labels[key] = value
	return lb
}

func (lb *LabelBuilder) Build() map[string]string {
	return cloneMap(lb.labels)
}

// Retry Logic
func RetryConfigFromConfig(cfg *config.Config) *RetryConfig {
	g := cfg.Global
	return &RetryConfig{
		MaxRetries:        g.APIMaxRetries,
		InitialBackoff:    time.Duration(g.APIRetryInitialDelaySeconds) * time.Second,
		MaxBackoff:        time.Duration(g.APIRetryMaxDelaySeconds) * time.Second,
		BackoffMultiplier: g.APIRetryBackoffMultiplier,
		RetryableErrors:   constants.RetryableErrors,
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

// Input Validation
func validateInputs(client *clients.Clients, namespace, projectName string) error {
	if client == nil {
		return errors.New("client cannot be nil")
	}
	if namespace == "" {
		return errors.New("namespace cannot be empty")
	}
	if projectName == "" {
		return errors.New("projectName cannot be empty")
	}
	return nil
}

// Metric Export Logic (main entry)
func ExportMetricValuesBatch(client *clients.Clients, cfg *config.Config, namespace string, projectName string) []MetricExport {
	// Input validation
	if err := validateInputs(client, namespace, projectName); err != nil {
		logs.Errorf("Input validation failed for namespace %s: %v", namespace, err)
		return nil
	}

	// Fetch metric definitions
	metrics, err := fetchMetricDefinitions(client, namespace, cfg)
	if err != nil {
		logs.Errorf("Failed to fetch metric definitions for namespace %s: %v", namespace, err)
		return nil
	}

	if len(metrics) == 0 {
		logs.Warnf("No metrics found in namespace %s in project %s", namespace, projectName)
		return nil
	}
	logs.Infof("Listed %d metrics in namespace %s in project %s", len(metrics), namespace, projectName)

	// Fetch time series data
	batchData, err := fetchTimeSeriesData(client, metrics, cfg)
	if err != nil {
		logs.Errorf("Failed to fetch time series data for namespace %s: %v", namespace, err)
		return nil
	}

	if batchData == nil || len(*batchData) == 0 {
		logs.Warnf("No time series data returned for namespace %s", namespace)
		return nil
	}

	// Process and enrich metrics
	results := processMetrics(client, cfg, namespace, batchData)

	// Get unique metrics and log count
	uniqueCount := logUniqueMetricsCount(results, namespace)
	logs.Infof("Exported %d metric series for namespace %s", uniqueCount, namespace)

	return results
}

// Helper Functions for Main Logic
func fetchMetricDefinitions(client *clients.Clients, namespace string, cfg *config.Config) ([]cesModel.MetricInfoList, error) {
	retryConfig := RetryConfigFromConfig(cfg)
	return withRetry(
		func() ([]cesModel.MetricInfoList, error) {
			return FetchAllMetricDefinitions(client, namespace, cfg)
		},
		retryConfig,
		fmt.Sprintf("fetch metrics for namespace %s", namespace),
	)
}

func fetchTimeSeriesData(client *clients.Clients, metrics []cesModel.MetricInfoList, cfg *config.Config) (*[]cesModel.BatchMetricData, error) {
	windowMs := cfg.Global.MetricQueryWindowMs
	start := time.Now().Add(-time.Duration(windowMs)*time.Millisecond).Unix() * 1000
	end := time.Now().Unix() * 1000
	period := strconv.Itoa(cfg.Global.MetricQueryPeriodMinutes)

	retryConfig := RetryConfigFromConfig(cfg)
	return withRetry(
		func() (*[]cesModel.BatchMetricData, error) {
			return fetchMetricTimeSeries(client, metrics, cfg, start, end, period)
		},
		retryConfig,
		"fetch time series data",
	)
}

func processMetrics(client *clients.Clients, cfg *config.Config, namespace string, batchData *[]cesModel.BatchMetricData) []MetricExport {
	var (
		results []MetricExport
		mu      sync.Mutex
		wg      sync.WaitGroup
	)
	for _, m := range *batchData {
		if m.MetricName == "" {
			logs.Warn("Metric with empty name found, skipping")
			continue
		}

		// Skip metrics that start with specific prefixes to avoid duplicates
		if shouldSkipMetric(m.MetricName, namespace) {
			logs.Debugf("Skipping duplicate metric: %s in namespace %s", m.MetricName, namespace)
			continue
		}

		wg.Add(1)
		go func(m cesModel.BatchMetricData) {
			defer wg.Done()
			// Extract and enrich labels
			labels, resourceID := extractLabelsAndResourceID(m, namespace)
			labels, resourceID = handleEVSIfNeeded(labels, resourceID, namespace, client)
			labels = handleOBSIfNeeded(labels, m, namespace, client, RetryConfigFromConfig(cfg))
			labels = enrichWithRMSIfNeeded(labels, resourceID, namespace, client, cfg, RetryConfigFromConfig(cfg))
			// Ensure resource_name exists
			if _, exists := labels[constants.LabelResourceName]; !exists {
				labels[constants.LabelResourceName] = constants.ResourceIDUnknown
			}
			unit := safeUnit(m.Unit)
			localResults := convertDatapointsToExports(m, labels, unit)
			mu.Lock()
			results = append(results, localResults...)
			mu.Unlock()
		}(m)
	}
	wg.Wait()
	return results
}

func logUniqueMetricsCount(results []MetricExport, namespace string) int {
	uniqueMetrics := make(map[string]struct{})
	for _, m := range results {
		labelPairs := make([]string, 0, len(m.Labels))
		for k, v := range m.Labels {
			if k == constants.LabelUnit {
				continue // Don't include unit in uniqueness
			}
			labelPairs = append(labelPairs, fmt.Sprintf("%s=%s", k, v))
		}
		sort.Strings(labelPairs)
		key := fmt.Sprintf("%s|%s", m.MetricName, strings.Join(labelPairs, "|"))
		uniqueMetrics[key] = struct{}{}
	}
	return len(uniqueMetrics)
}

// Metric Definition / Fetch Logic
func FetchAllMetricDefinitions(client *clients.Clients, namespace string, cfg *config.Config) ([]cesModel.MetricInfoList, error) {
	limit := int32(cfg.Global.MetricQueryPageLimit)
	req := &cesModel.ListMetricsRequest{
		Limit:     &limit,
		Namespace: Ptr(namespace),
	}
	var result []cesModel.MetricInfoList
	retryConfig := RetryConfigFromConfig(cfg)
	for {
		resp, err := withRetry(
			func() (*cesModel.ListMetricsResponse, error) {
				return client.CloudEyeV1.ListMetrics(req)
			},
			retryConfig,
			fmt.Sprintf("list metrics for namespace %s", namespace),
		)
		if err != nil {
			return nil, fmt.Errorf("ListMetrics error after retries: %w", err)
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

func fetchMetricTimeSeries(client *clients.Clients, metrics []cesModel.MetricInfoList, cfg *config.Config, from, to int64, period string) (*[]cesModel.BatchMetricData, error) {
	batchMetrics := buildBatchMetrics(metrics)
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
	retryConfig := RetryConfigFromConfig(cfg)
	resp, err := withRetry(
		func() (*cesModel.BatchListMetricDataResponse, error) {
			return client.CloudEyeV1.BatchListMetricData(req)
		},
		retryConfig,
		"batch list metric data",
	)
	if err != nil {
		return nil, fmt.Errorf("BatchListMetricData error after retries: %w", err)
	}
	return resp.Metrics, nil
}

func buildBatchMetrics(metrics []cesModel.MetricInfoList) []cesModel.MetricInfo {
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
	return batchMetrics
}

// -----------------------------
// Label/Resource Enrichment
// -----------------------------
func extractLabelsAndResourceID(m cesModel.BatchMetricData, namespace string) (map[string]string, string) {
	labelBuilder := NewLabelBuilder(namespace)
	labelBuilder.AddDimensions(m.Dimensions)
	labels := labelBuilder.Build()
	// Find resource ID
	resourceID := findResourceID(labels)
	// Handle special cases for OBS
	if resourceID == "" {
		resourceID = handleSpecialResourceID(labels, m, namespace)
	}
	if resourceID == "" {
		resourceID = constants.ResourceIDUnknown
		logs.Warnf("No valid resource_id found for metric %s, using '%s'", m.MetricName, resourceID)
	}
	labels[constants.LabelResourceID] = resourceID
	return labels, resourceID
}

func findResourceID(labels map[string]string) string {
	for key, value := range labels {
		if strings.HasSuffix(key, "_id") &&
			key != "tenant_id" && key != "project_id" && key != "domain_id" && key != "user_id" {
			if value != constants.ResourceIDUnknown && value != "" {
				return value
			}
		}
	}
	// Try to find any non-meta dimension
	for key, value := range labels {
		if key != "tenant_id" && key != "project_id" && key != "domain_id" &&
			key != "user_id" && key != constants.LabelNamespace {
			if value != "" && value != constants.ResourceIDUnknown {
				return value
			}
		}
	}
	return ""
}

func handleSpecialResourceID(labels map[string]string, m cesModel.BatchMetricData, namespace string) string {
	if namespace == constants.NamespaceOBS {
		bucketName := getBucketNameFromDimensions(m.Dimensions)
		apiName := getAPINameFromDimensions(m.Dimensions)
		if bucketName == "" && apiName == "" {
			labels[constants.LabelResourceID] = constants.ResourceIDTotal
			labels[constants.LabelResourceName] = constants.ResourceIDTotal
			labels["scope"] = constants.ResourceIDTotal
			return constants.ResourceIDTotal
		} else if apiName != "" {
			labels["operation"] = apiName
			return apiName
		}
	}
	return ""
}

func getAPINameFromDimensions(dims *[]cesModel.MetricsDimension) string {
	for _, dim := range safeDimensions(dims) {
		if strings.ToLower(dim.Name) == "api_name" {
			return dim.Value
		}
	}
	return ""
}

func enrichWithRMSIfNeeded(labels map[string]string, resourceID, namespace string, client *clients.Clients, cfg *config.Config, retryConfig *RetryConfig) map[string]string {
	if !shouldEnrichWithRMS(client, resourceID, namespace) {
		return labels
	}
	rmsResource, err := withRetry(
		func() (map[string]string, error) {
			return client.RMS.GetResourceByID(resourceID, "")
		},
		retryConfig,
		fmt.Sprintf("get RMS resource info for %s", resourceID),
	)
	if err != nil {
		logs.Warnf("Failed to fetch RMS info for %s after retries: %v", resourceID, err)
		return labels
	}
	if rmsResource == nil {
		return labels
	}
	return applyRMSEnrichment(labels, rmsResource, client, cfg)
}

func shouldEnrichWithRMS(client *clients.Clients, resourceID, namespace string) bool {
	return client.RMS != nil &&
		resourceID != "" &&
		resourceID != constants.ResourceIDUnknown &&
		!shouldSkipRMSLookup(namespace, resourceID)
}

func applyRMSEnrichment(labels map[string]string, rmsResource map[string]string, client *clients.Clients, cfg *config.Config) map[string]string {
	if cfg.Global.ExportRMSLabels[constants.LabelResourceName] {
		if name := rmsResource["name"]; name != "" {
			labels[constants.LabelResourceName] = name
		} else if name := rmsResource["resource_name"]; name != "" {
			labels[constants.LabelResourceName] = name
		}
	}
	if cfg.Global.ExportRMSLabels[constants.LabelProjectID] {
		labels[constants.LabelProjectID] = client.ProjectID
	}
	if cfg.Global.ExportRMSLabels[constants.LabelProjectName] {
		labels[constants.LabelProjectName] = client.ProjectName
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
		labels[constants.LabelResourceID] = rmsID
	}
	return labels
}

func handleEVSIfNeeded(labels map[string]string, resourceID, namespace string, client *clients.Clients) (map[string]string, string) {
	if !strings.Contains(namespace, "EVS") {
		return labels, resourceID
	}
	lastDash := strings.LastIndex(resourceID, "-")
	if lastDash <= 0 || lastDash >= len(resourceID)-1 {
		return labels, resourceID
	}
	vmID := resourceID[:lastDash]
	device := resourceID[lastDash+1:]
	actualDiskID, diskName := lookupEVSID(client, vmID, device)
	if actualDiskID != "" {
		labels[constants.LabelResourceID] = actualDiskID
		if diskName != "" {
			labels["disk_name"] = diskName
		}
		return labels, actualDiskID
	}
	logs.Warnf("Failed to resolve EVS disk ID for %s", resourceID)
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
	if namespace != constants.NamespaceOBS {
		return labels
	}
	bucketName := getBucketNameFromDimensions(m.Dimensions)
	if bucketName != "" {
		labels["bucket_name"] = bucketName
		return enrichOBSBucketInfo(labels, bucketName, client)
	}
	// Handle service-level metrics
	if tenantID, exists := labels["tenant_id"]; exists && labels[constants.LabelResourceID] == tenantID {
		labels[constants.LabelResourceName] = "obs_service"
		labels[constants.LabelResourceID] = "obs_service"
	} else {
		labels[constants.LabelResourceName] = labels[constants.LabelResourceID]
	}
	return labels
}

func enrichOBSBucketInfo(labels map[string]string, bucketName string, client *clients.Clients) map[string]string {
	if client.OBS == nil {
		return labels
	}
	// Try to get bucket tags
	if tags, err := client.OBS.GetBucketTags(bucketName); err == nil {
		for k, v := range tags {
			labels["tag_"+k] = v
		}
		logs.Debugf("Added %d bucket tags to labels for bucket %s", len(tags), bucketName)
	} else {
		logs.Warnf("Could not fetch tags for OBS bucket %s: %v", bucketName, err)
	}
	// Try to get bucket info
	if info, err := client.OBS.GetBucketInfo(bucketName); err == nil {
		for k, v := range info {
			labels[k] = v
		}
		logs.Debugf("Added bucket info labels for bucket %s", bucketName)
	} else {
		logs.Warnf("Could not fetch info for OBS bucket %s: %v", bucketName, err)
	}
	return labels
}

// -----------------------------
// Metric Data Conversion
// -----------------------------
func convertDatapointsToExports(m cesModel.BatchMetricData, labels map[string]string, unit string) []MetricExport {
	if len(m.Datapoints) == 0 {
		return createSingleExport(m.MetricName, labels, unit, 0, time.Now())
	}
	localResults := make([]MetricExport, 0, len(m.Datapoints))
	for _, dp := range m.Datapoints {
		value := 0.0
		if dp.Average != nil {
			value = *dp.Average
		}
		timestamp := time.Unix(0, dp.Timestamp*int64(time.Millisecond))
		exports := createSingleExport(m.MetricName, labels, unit, value, timestamp)
		localResults = append(localResults, exports...)
	}
	return localResults
}

func createSingleExport(metricName string, labels map[string]string, unit string, value float64, timestamp time.Time) []MetricExport {
	labelsWithUnit := cloneMap(labels)
	if unit != "" {
		labelsWithUnit[constants.LabelUnit] = unit
	}
	return []MetricExport{{
		MetricName: metricName,
		Labels:     labelsWithUnit,
		Value:      value,
		Unit:       unit,
		Timestamp:  timestamp,
	}}
}

// shouldSkipMetric determines if a metric should be skipped to avoid duplicates
func shouldSkipMetric(metricName, namespace string) bool {
	switch namespace {
	case constants.NamespaceAGT: // AGT.ECS namespace
		// Skip metrics that start with "id_" as they are duplicates of "d_" metrics
		if strings.HasPrefix(metricName, "id_") {
			return true
		}

		// You can add more specific filters for AGT.ECS if needed
		skipPrefixes := []string{
			"id_", // Skip all id_ prefixed metrics
		}

		for _, prefix := range skipPrefixes {
			if strings.HasPrefix(metricName, prefix) {
				return true
			}
		}
	}
	return false
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

func cloneMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func isOBSOperationName(value string) bool {
	for _, op := range constants.OBSOperations {
		if value == op {
			return true
		}
	}
	return false
}

func getBucketNameFromDimensions(dims *[]cesModel.MetricsDimension) string {
	for _, dim := range safeDimensions(dims) {
		if strings.ToLower(dim.Name) == "bucket_name" {
			return dim.Value
		}
	}
	return ""
}

func shouldSkipRMSLookup(namespace string, resourceID string) bool {
	if resourceID == "" || resourceID == constants.ResourceIDUnknown {
		return true
	}
	// Avoid RMS lookups for OBS operation metrics
	if namespace == constants.NamespaceOBS && isOBSOperationName(resourceID) {
		return true
	}
	return false
}
