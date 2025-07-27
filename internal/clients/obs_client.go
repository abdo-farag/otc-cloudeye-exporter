package clients

import (
	"fmt"
	"sync"
	"time"

	"github.com/abdo-farag/otc-cloudeye-exporter/internal/config"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/logs"
	obs "github.com/huaweicloud/huaweicloud-sdk-go-obs/obs"
)

type cachedObsEntry struct {
	tags      map[string]string
	timestamp time.Time
}

var (
	obsTagCache     sync.Map
	obsTagCacheTTL  = 15 * time.Minute
	obsCacheCleaner sync.Once
)

type ObsClient struct {
	client *obs.ObsClient
}

// NewObsClient initializes an OBS client
func NewObsClient(cfg *config.Config, endpoint string) (*ObsClient, error) {
	logs.Infof("Initializing OBS client for endpoint: %s", endpoint)
	obsClient, err := obs.New(cfg.Auth.AccessKey, cfg.Auth.SecretKey, endpoint)
	if err != nil {
		logs.Errorf("Failed to create OBS client for endpoint %s: %v", endpoint, err)
		return nil, fmt.Errorf("failed to create OBS client: %w", err)
	}

	// Start cache cleaner only once
	obsCacheCleaner.Do(startObsTagCacheCleaner)
	logs.Infof("OBS client initialized successfully for endpoint: %s", endpoint)

	return &ObsClient{client: obsClient}, nil
}

// GetBucketTags fetches and caches bucket tags
func (o *ObsClient) GetBucketTags(bucketName string) (map[string]string, error) {
	if bucketName == "" {
		logs.Warnf("GetBucketTags called with empty bucket name")
		return nil, fmt.Errorf("bucket name cannot be empty")
	}

	cacheKey := "bucket:" + bucketName

	// Check cache first
	if entry, ok := obsTagCache.Load(cacheKey); ok {
		if cached, ok := entry.(cachedObsEntry); ok {
			if time.Since(cached.timestamp) < obsTagCacheTTL {
				logs.Debugf("OBS bucket tag cache hit for %s", bucketName)
				return cached.tags, nil
			}
		}
	}

	logs.Debugf("OBS bucket tag cache miss for %s, querying API", bucketName)

	output, err := o.client.GetBucketTagging(bucketName)
	if err != nil {
		// If bucket has no tags, OBS returns an error - this is normal
		if obsErr, ok := err.(obs.ObsError); ok {
			if obsErr.Code == "NoSuchTagSet" || obsErr.StatusCode == 404 {
				logs.Infof("Bucket %s has no tags (NoSuchTagSet/404)", bucketName)
				obsTagCache.Store(cacheKey, cachedObsEntry{
					tags:      make(map[string]string),
					timestamp: time.Now(),
				})
				return make(map[string]string), nil
			}
		}
		logs.Errorf("Failed to get bucket tags for %s: %v", bucketName, err)
		return nil, fmt.Errorf("failed to get bucket tags for %s: %w", bucketName, err)
	}

	// Convert tags to map
	tags := make(map[string]string)
	for _, tag := range output.Tags {
		if tag.Key != "" {
			tags[tag.Key] = tag.Value
		}
	}

	obsTagCache.Store(cacheKey, cachedObsEntry{
		tags:      tags,
		timestamp: time.Now(),
	})

	logs.Infof("Fetched and cached %d tags for bucket %s", len(tags), bucketName)
	return tags, nil
}

// GetBucketInfo fetches bucket location and other metadata
func (o *ObsClient) GetBucketInfo(bucketName string) (map[string]string, error) {
	if bucketName == "" {
		logs.Warnf("GetBucketInfo called with empty bucket name")
		return nil, fmt.Errorf("bucket name cannot be empty")
	}

	cacheKey := "info:" + bucketName

	// Check cache first
	if entry, ok := obsTagCache.Load(cacheKey); ok {
		if cached, ok := entry.(cachedObsEntry); ok {
			if time.Since(cached.timestamp) < obsTagCacheTTL {
				logs.Debugf("OBS bucket info cache hit for %s", bucketName)
				return cached.tags, nil
			}
		}
	}

	logs.Debugf("OBS bucket info cache miss for %s, querying API", bucketName)

	locationOutput, err := o.client.GetBucketLocation(bucketName)
	if err != nil {
		logs.Errorf("Failed to get bucket location for %s: %v", bucketName, err)
		return nil, fmt.Errorf("failed to get bucket location for %s: %w", bucketName, err)
	}

	info := map[string]string{
		"bucket_name": bucketName,
		"location":    locationOutput.Location,
	}

	obsTagCache.Store(cacheKey, cachedObsEntry{
		tags:      info,
		timestamp: time.Now(),
	})

	logs.Infof("Fetched and cached location info for bucket %s", bucketName)
	return info, nil
}

// Close closes the OBS client
func (o *ObsClient) Close() {
	if o.client != nil {
		logs.Infof("Closing OBS client")
		o.client.Close()
	}
}

// startObsTagCacheCleaner runs periodically to evict old entries
func startObsTagCacheCleaner() {
	ticker := time.NewTicker(30 * time.Minute)
	go func() {
		for range ticker.C {
			now := time.Now()
			obsTagCache.Range(func(key, val any) bool {
				if entry, ok := val.(cachedObsEntry); ok {
					if now.Sub(entry.timestamp) > obsTagCacheTTL {
						logs.Debugf("Evicting expired OBS cache entry: %s", key)
						obsTagCache.Delete(key)
					}
				}
				return true
			})
		}
	}()
}
