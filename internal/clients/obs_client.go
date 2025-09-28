package clients

import (
	"fmt"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/config"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/logs"
	obs "github.com/huaweicloud/huaweicloud-sdk-go-obs/obs"
	"sync"
	"time"
)

// =============== CACHE ======================

type cachedObsEntry struct {
	data      map[string]string
	timestamp time.Time
}

const (
	obsCacheTTL       = 15 * time.Minute
	obsCacheCleanTime = 30 * time.Minute
)

type obsCacheType struct {
	m sync.Map
}

func (c *obsCacheType) Get(key string) (map[string]string, bool) {
	val, ok := c.m.Load(key)
	if !ok {
		return nil, false
	}
	entry, ok := val.(cachedObsEntry)
	if !ok || time.Since(entry.timestamp) > obsCacheTTL {
		c.m.Delete(key)
		return nil, false
	}
	return entry.data, true
}

func (c *obsCacheType) Set(key string, data map[string]string) {
	c.m.Store(key, cachedObsEntry{
		data:      data,
		timestamp: time.Now(),
	})
}

func (c *obsCacheType) Clean() {
	now := time.Now()
	c.m.Range(func(key, val any) bool {
		if entry, ok := val.(cachedObsEntry); ok {
			if now.Sub(entry.timestamp) > obsCacheTTL {
				logs.Debugf("Evicting expired OBS cache entry: %s", key)
				c.m.Delete(key)
			}
		}
		return true
	})
}

var (
	obsCache = &obsCacheType{}
)

// Starts the background cache cleaner only once
func startObsCacheCleaner() {
	ticker := time.NewTicker(obsCacheCleanTime)
	go func() {
		for range ticker.C {
			obsCache.Clean()
		}
	}()
}

// =============== CLIENT ======================

type ObsClient struct {
	client *obs.ObsClient
}

// InitObsClient initializes an OBS client
func InitObsClient(cfg *config.Config, endpoint string) (*ObsClient, error) {
	logs.Infof("Initializing OBS client for endpoint: %s", endpoint)
	obsClient, err := obs.New(cfg.Auth.AccessKey, cfg.Auth.SecretKey, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create OBS client: %w", err)
	}
	// Start cache cleaner only once
	cacheCleaner.Do(startObsCacheCleaner)
	logs.Infof("OBS client initialized for endpoint: %s", endpoint)
	return &ObsClient{client: obsClient}, nil
}

// GetBucketTags fetches and caches bucket tags
func (o *ObsClient) GetBucketTags(bucketName string) (map[string]string, error) {
	if bucketName == "" {
		return nil, fmt.Errorf("bucket name cannot be empty")
	}
	cacheKey := "tags:" + bucketName
	// Check cache first
	if data, ok := obsCache.Get(cacheKey); ok {
		logs.Debugf("OBS bucket tag cache hit for %s", bucketName)
		return data, nil
	}
	logs.Debugf("OBS bucket tag cache miss for %s, querying API", bucketName)
	output, err := o.client.GetBucketTagging(bucketName)
	if err != nil {
		// No tags is normal
		if obsErr, ok := err.(obs.ObsError); ok {
			if obsErr.Code == "NoSuchTagSet" || obsErr.StatusCode == 404 {
				logs.Infof("Bucket %s has no tags", bucketName)
				obsCache.Set(cacheKey, map[string]string{})
				return map[string]string{}, nil
			}
		}
		return nil, fmt.Errorf("failed to get bucket tags for %s: %w", bucketName, err)
	}
	tags := make(map[string]string)
	for _, tag := range output.Tags {
		if tag.Key != "" {
			tags[tag.Key] = tag.Value
		}
	}
	obsCache.Set(cacheKey, tags)
	logs.Infof("Fetched and cached %d tags for bucket %s", len(tags), bucketName)
	return tags, nil
}

// GetBucketInfo fetches bucket location and other metadata
func (o *ObsClient) GetBucketInfo(bucketName string) (map[string]string, error) {
	if bucketName == "" {
		return nil, fmt.Errorf("bucket name cannot be empty")
	}
	cacheKey := "info:" + bucketName
	// Check cache first
	if data, ok := obsCache.Get(cacheKey); ok {
		logs.Debugf("OBS bucket info cache hit for %s", bucketName)
		return data, nil
	}
	logs.Debugf("OBS bucket info cache miss for %s, querying API", bucketName)
	locationOutput, err := o.client.GetBucketLocation(bucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket location for %s: %w", bucketName, err)
	}
	info := map[string]string{
		"bucket_name": bucketName,
		"location":    locationOutput.Location,
	}
	obsCache.Set(cacheKey, info)
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
