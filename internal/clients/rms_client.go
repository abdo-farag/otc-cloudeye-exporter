package clients

import (
	"encoding/json"
	"fmt"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/config"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/logs"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/global"
	rms "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rms/v1"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rms/v1/model"
	"reflect"
	"strings"
	"sync"
	"time"
)

type cachedRmsEntry struct {
	data      map[string]string
	timestamp time.Time
}

const (
	rmsCacheTTL       = 15 * time.Minute
	rmsCacheCleanTime = 30 * time.Minute
)

type rmsCacheType struct {
	m sync.Map
}

func (c *rmsCacheType) Get(key string) (map[string]string, bool) {
	val, ok := c.m.Load(key)
	if !ok {
		return nil, false
	}
	entry, ok := val.(cachedRmsEntry)
	if !ok || time.Since(entry.timestamp) > rmsCacheTTL {
		c.m.Delete(key)
		return nil, false
	}
	return entry.data, true
}

func (c *rmsCacheType) Set(key string, data map[string]string) {
	c.m.Store(key, cachedRmsEntry{
		data:      data,
		timestamp: time.Now(),
	})
}

func (c *rmsCacheType) Clean() {
	now := time.Now()
	c.m.Range(func(key, val any) bool {
		if entry, ok := val.(cachedRmsEntry); ok {
			if now.Sub(entry.timestamp) > rmsCacheTTL {
				c.m.Delete(key)
				logs.Infof("Evicted cached RMS entry: %s", key)
			}
		}
		return true
	})
}

var (
	rmsCache = &rmsCacheType{}
)

func startRmsCacheCleaner() {
	ticker := time.NewTicker(rmsCacheCleanTime)
	go func() {
		for range ticker.C {
			rmsCache.Clean()
		}
	}()
}

type RmsClient struct {
	client *rms.RmsClient
}

func InitRmsClient(cfg *config.Config, endpoint, region string) (*RmsClient, error) {
	logs.Infof("Initializing RMS client for region: %s, endpoint: %s", region, endpoint)
	auth, err := global.NewCredentialsBuilder().
		WithAk(cfg.Auth.AccessKey).
		WithSk(cfg.Auth.SecretKey).
		WithDomainId(cfg.Auth.DomainID).
		SafeBuild()
	if err != nil {
		return nil, fmt.Errorf("failed to build credentials: %w", err)
	}
	hcClient, err := rms.RmsClientBuilder().
		WithEndpoints([]string{endpoint}).
		WithCredential(auth).
		WithHttpConfig(config.GetHttpConfig().WithIgnoreSSLVerification(cfg.Global.IgnoreSSLVerify)).
		SafeBuild()
	if err != nil {
		return nil, fmt.Errorf("failed to build RMS client: %w", err)
	}
	cacheCleaner.Do(startRmsCacheCleaner)
	logs.Infof("RMS client initialized for region: %s", region)
	return &RmsClient{client: rms.NewRmsClient(hcClient)}, nil
}

// GetResourceByID fetches resource metadata, using cache when possible.
func (r *RmsClient) GetResourceByID(resourceID, resourceName string) (map[string]string, error) {
	cacheKey := buildCacheKey(resourceID, resourceName)
	if cacheKey == "" {
		return nil, fmt.Errorf("either resourceID or resourceName must be provided")
	}
	// Try cache first
	if data, ok := rmsCache.Get(cacheKey); ok {
		logs.Debugf("Cache hit for resource: %s", cacheKey)
		return data, nil
	}
	logs.Debugf("Cache miss for resource: %s", cacheKey)
	resource, err := r.lookupResource(resourceID, resourceName)
	if err != nil {
		return nil, err
	}
	// Cache with all possible keys for quick lookup next time
	cacheResource(resource, cacheKey)
	return resource, nil
}

func (r *RmsClient) lookupResource(resourceID, resourceName string) (map[string]string, error) {
	limit := int32(200)
	req := &model.ListAllResourcesRequest{Limit: &limit}
	if resourceID != "" {
		req.Id = &resourceID
	}
	if resourceName != "" {
		req.Name = &resourceName
	}
	for {
		resp, err := r.client.ListAllResources(req)
		if err != nil {
			return nil, fmt.Errorf("RMS lookup failed for %s: %w", resourceID+resourceName, err)
		}
		if resp.Resources == nil || len(*resp.Resources) == 0 {
			break
		}
		for _, res := range *resp.Resources {
			if matchesResource(&res, resourceID, resourceName) {
				info := structToStringMap(res)
				mergeTags(info, res.Tags)
				return info, nil
			}
		}
		if resp.PageInfo == nil || resp.PageInfo.NextMarker == nil || *resp.PageInfo.NextMarker == "" {
			break
		}
		req.Marker = resp.PageInfo.NextMarker
	}
	return nil, fmt.Errorf("resource %s not found", resourceID+resourceName)
}

func matchesResource(res *model.ResourceEntity, resourceID, resourceName string) bool {
	if resourceID != "" && res.Id != nil && *res.Id == resourceID {
		return true
	}
	if resourceName != "" && res.Name != nil && *res.Name == resourceName {
		return true
	}
	return false
}

// ================== UTILS ===================
func buildCacheKey(resourceID, resourceName string) string {
	if resourceID != "" {
		return "id:" + resourceID
	}
	if resourceName != "" {
		return "name:" + resourceName
	}
	return ""
}

func cacheResource(info map[string]string, cacheKey string) {
	rmsCache.Set(cacheKey, info)
	if id, ok := info["id"]; ok && id != "" && cacheKey != "id:"+id {
		rmsCache.Set("id:"+id, info)
	}
	if name, ok := info["name"]; ok && name != "" && cacheKey != "name:"+name {
		rmsCache.Set("name:"+name, info)
	}
}

// structToStringMap converts struct to map[string]string via JSON (preferable if all fields are tagged).
func structToStringMap(input interface{}) map[string]string {
	out := make(map[string]string)
	// Use JSON as primary
	b, err := json.Marshal(input)
	if err == nil {
		json.Unmarshal(b, &out)
	}
	// Fallback: reflect for missing fields (rare)
	v := reflect.ValueOf(input)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		name := strings.ToLower(field.Name)
		if _, ok := out[name]; !ok {
			val := v.Field(i)
			out[name] = fmt.Sprintf("%v", val.Interface())
		}
	}
	return out
}

// mergeTags merges tag map into info map with prefix
func mergeTags(info map[string]string, tags map[string]string) {
	for k, v := range tags {
		info["tag_"+k] = v
	}
}

// ListAllResources fetches all resources from RMS.
func (r *RmsClient) ListAllResources() ([]map[string]string, error) {
	var results []map[string]string
	limit := int32(200)
	req := &model.ListAllResourcesRequest{Limit: &limit}
	for {
		resp, err := r.client.ListAllResources(req)
		if err != nil {
			return nil, fmt.Errorf("RMS ListAllResources error: %w", err)
		}
		if resp.Resources != nil {
			for _, res := range *resp.Resources {
				info := structToStringMap(res)
				mergeTags(info, res.Tags)
				results = append(results, info)
			}
		}
		if resp.PageInfo == nil || resp.PageInfo.NextMarker == nil || *resp.PageInfo.NextMarker == "" {
			break
		}
		req.Marker = resp.PageInfo.NextMarker
	}
	return results, nil
}
