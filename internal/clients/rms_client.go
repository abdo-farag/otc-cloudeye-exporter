package clients

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/abdo-farag/otc-cloudeye-exporter/internal/config"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/logs"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/global"
	rms "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rms/v1"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rms/v1/model"
)

type cachedRmsEntry struct {
	data      map[string]string
	timestamp time.Time
}

var (
	rmsCache     sync.Map
	rmsCacheTTL  = 15 * time.Minute
	cacheCleaner sync.Once
)

type RmsClient struct {
	client *rms.RmsClient
}

// NewRmsClient initializes an RMS client
func InitRmsClient(cfg *config.Config, endpoint string, region string) (*RmsClient, error) {
	logs.Infof("Initializing RMS client for region: %s, endpoint: %s", region, endpoint)

	auth, err := global.NewCredentialsBuilder().
		WithAk(cfg.Auth.AccessKey).
		WithSk(cfg.Auth.SecretKey).
		WithDomainId(cfg.Auth.DomainID).
		SafeBuild()
	if err != nil {
		logs.Errorf("Failed to build credentials for RMS client: %v", err)
		return nil, fmt.Errorf("failed to build credentials: %w", err)
	}

	hcClient, err := rms.RmsClientBuilder().
		WithEndpoints([]string{endpoint}).
		WithCredential(auth).
		WithHttpConfig(config.GetHttpConfig().WithIgnoreSSLVerification(cfg.Global.IgnoreSSLVerify)).
		SafeBuild()
	if err != nil {
		logs.Errorf("Failed to build RMS client for region %s: %v", region, err)
		return nil, fmt.Errorf("failed to build RMS client: %w", err)
	}

	client := rms.NewRmsClient(hcClient)

	// Start cleaner only once
	cacheCleaner.Do(startRmsCacheCleaner)

	logs.Infof("RMS client initialized successfully for region: %s", region)

	return &RmsClient{client: client}, nil
}

// GetResourceByID fetches and caches metadata about a resource
func (r *RmsClient) GetResourceByID(resourceID string, resourceName string) (map[string]string, error) {
	// Determine cache key based on what we're searching for
	var cacheKey string
	if resourceID != "" {
		cacheKey = "id:" + resourceID
	} else if resourceName != "" {
		cacheKey = "name:" + resourceName
	} else {
		logs.Warnf("Either resourceID or resourceName must be provided")
		return nil, fmt.Errorf("either resourceID or resourceName must be provided")
	}

	// Check cache first
	if entry, ok := rmsCache.Load(cacheKey); ok {
		if cached, ok := entry.(cachedRmsEntry); ok {
			if time.Since(cached.timestamp) < rmsCacheTTL {
				logs.Debugf("Cache hit for resource: %s", cacheKey)
				return cached.data, nil
			}
		}
	}

	logs.Debugf("Cache miss for resource: %s, fetching from RMS", cacheKey)

	limit := int32(200)
	req := &model.ListAllResourcesRequest{
		Limit: &limit,
	}

	// Set search parameters
	if resourceID != "" {
		req.Id = &resourceID
	}
	if resourceName != "" {
		req.Name = &resourceName
	}

	for {
		resp, err := r.client.ListAllResources(req)
		if err != nil {
			searchTerm := resourceID
			if searchTerm == "" {
				searchTerm = resourceName
			}
			logs.Errorf("RMS lookup failed for %s: %v", searchTerm, err)
			return nil, fmt.Errorf("RMS lookup failed for %s: %w", searchTerm, err)
		}

		if resp.Resources == nil || len(*resp.Resources) == 0 {
			logs.Infof("No resources found for %s", cacheKey)
			break
		}

		for _, res := range *resp.Resources {
			// Check if this matches what we're looking for
			var isMatch bool
			if resourceID != "" && res.Id != nil && *res.Id == resourceID {
				isMatch = true
			} else if resourceName != "" && res.Name != nil && *res.Name == resourceName {
				isMatch = true
			}

			if isMatch {
				info := structToStringMap(res)

				// Add tags
				if res.Tags != nil {
					for k, v := range res.Tags {
						info["tag_"+k] = v
					}
				}

				// Cache with the original search key
				rmsCache.Store(cacheKey, cachedRmsEntry{
					data:      info,
					timestamp: time.Now(),
				})

				// Also cache with both ID and name if available for future lookups
				if res.Id != nil && *res.Id != "" {
					idKey := "id:" + *res.Id
					if idKey != cacheKey {
						rmsCache.Store(idKey, cachedRmsEntry{
							data:      info,
							timestamp: time.Now(),
						})
					}
				}
				if res.Name != nil && *res.Name != "" {
					nameKey := "name:" + *res.Name
					if nameKey != cacheKey {
						rmsCache.Store(nameKey, cachedRmsEntry{
							data:      info,
							timestamp: time.Now(),
						})
					}
				}

				logs.Infof("Resource found and cached: %s", cacheKey)
				return info, nil
			}
		}

		if resp.PageInfo == nil || resp.PageInfo.NextMarker == nil {
			break
		}
		req.Marker = resp.PageInfo.NextMarker
	}

	searchTerm := resourceID
	if searchTerm == "" {
		searchTerm = resourceName
	}
	logs.Errorf("Resource %s not found", searchTerm)
	return nil, fmt.Errorf("resource %s not found", searchTerm)
}

// structToStringMap converts any struct to a map[string]string
func structToStringMap(input interface{}) map[string]string {
	result := make(map[string]string)
	v := reflect.ValueOf(input)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		name := strings.ToLower(field.Name)
		value := v.Field(i)

		if value.Kind() == reflect.Ptr && !value.IsNil() {
			value = value.Elem()
		}

		switch value.Kind() {
		case reflect.Map, reflect.Struct, reflect.Slice:
			jsonBytes, err := json.Marshal(value.Interface())
			if err == nil {
				result[name] = string(jsonBytes)
			} else {
				result[name] = fmt.Sprintf("%v", value.Interface())
			}
		default:
			result[name] = fmt.Sprintf("%v", value.Interface())
		}
	}
	return result
}

// startRmsCacheCleaner runs periodically to evict old entries
func startRmsCacheCleaner() {
	ticker := time.NewTicker(30 * time.Minute)
	go func() {
		for range ticker.C {
			now := time.Now()
			rmsCache.Range(func(key, val any) bool {
				if entry, ok := val.(cachedRmsEntry); ok {
					if now.Sub(entry.timestamp) > rmsCacheTTL {
						rmsCache.Delete(key)
						logs.Infof("Evicted cached RMS entry: %s", key)
					}
				}
				return true
			})
		}
	}()
}

// ListAllResources fetches all resources from RMS
func (r *RmsClient) ListAllResources() ([]map[string]string, error) {
	var results []map[string]string
	limit := int32(200)
	req := &model.ListAllResourcesRequest{
		Limit: &limit,
	}
	for {
		resp, err := r.client.ListAllResources(req)
		if err != nil {
			return nil, fmt.Errorf("RMS ListAllResources error: %w", err)
		}
		if resp.Resources != nil {
			for _, res := range *resp.Resources {
				info := structToStringMap(res)
				// Add tags if needed
				if res.Tags != nil {
					for k, v := range res.Tags {
						info["tag_"+k] = v
					}
				}
				results = append(results, info)
			}
		}
		// Paging
		if resp.PageInfo == nil || resp.PageInfo.NextMarker == nil || *resp.PageInfo.NextMarker == "" {
			break
		}
		req.Marker = resp.PageInfo.NextMarker
	}
	return results, nil
}
