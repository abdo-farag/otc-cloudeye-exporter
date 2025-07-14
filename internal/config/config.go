package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/global"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/sdkerr"
	iam "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3/model"

	"github.com/abdo-farag/otc-cloudeye-exporter/internal/logs"
)

// ---------- Struct Definitions ----------

type ProjectConfig struct {
	Name string `yaml:"name"`
	ID   string `yaml:"id,omitempty"`
}

type CloudAuth struct {
	Projects   []ProjectConfig `yaml:"projects"`
	DomainName string          `yaml:"domain_name"`
	DomainID   string          `yaml:"domain_id"`
	AccessKey  string          `yaml:"access_key"`
	SecretKey  string          `yaml:"secret_key"`
	Region     string          `yaml:"region"`
	AuthURL    string          `yaml:"auth_url"`
}

type Global struct {
	Port                        string `yaml:"port"`
	EnableHTTPS                 bool   `yaml:"enable_https"`
	HTTPSPort                   string `yaml:"https_port"`
	TLSCert                     string `yaml:"tls_cert"`
	TLSKey                      string `yaml:"tls_key"`
	Prefix                      string `yaml:"prefix"`
	MetricPath                  string `yaml:"metric_path"`
	ScrapeBatchSize             int    `yaml:"scrape_batch_size"`
	ResourceSyncIntervalMinutes int    `yaml:"resource_sync_interval_minutes"`
	RmsRetryTimes               int    `yaml:"rms_retry_times"`
	Namespaces                  string `yaml:"namespaces"`
	EndpointsConfPath           string `yaml:"endpoints_conf_path"`
	IgnoreSSLVerify             bool   `yaml:"ignore_ssl_verify"`

	HttpSchema string            `yaml:"proxy_schema"`
	HttpHost   string            `yaml:"proxy_host"`
	HttpPort   int               `yaml:"proxy_port"`
	UserName   string            `yaml:"proxy_username"`
	Password   string            `yaml:"proxy_password"`

	ExportRMSLabels             map[string]bool `yaml:"export_rms_labels"`
}

type Config struct {
	Auth   CloudAuth `yaml:"auth"`
	Global Global    `yaml:"global"`
}

type ProjectInfo struct {
	Name string
	ID   string
}

var AppConfig *Config

// ---------- Substitute Environment Variables in Auth Fields ----------

func substituteEnvVars(val string) string {
	re := regexp.MustCompile(`^\$\{(\w+)\}$`)
	if m := re.FindStringSubmatch(val); len(m) == 2 {
		if v := os.Getenv(m[1]); v != "" {
			return v
		}
	}
	// Optionally: fallback for "OS_VARNAME" (if you want to support it without ${...})
	if strings.HasPrefix(val, "OS_") && strings.ToUpper(val) == val {
		if v := os.Getenv(val); v != "" {
			return v
		}
	}
	return val
}

func resolveAuthEnv(auth *CloudAuth) {
	auth.DomainID   = substituteEnvVars(auth.DomainID)
	auth.DomainName = substituteEnvVars(auth.DomainName)
	auth.AccessKey  = substituteEnvVars(auth.AccessKey)
	auth.SecretKey  = substituteEnvVars(auth.SecretKey)
}

// ---------- Load Config ----------

func LoadConfig(path string) (*Config, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	logs.Infof("✅ Loaded config from %s", path)

	// Substitute env vars in Auth fields if present
	resolveAuthEnv(&cfg.Auth)

	// Fill project IDs if missing
	if err := resolveProjectIDs(&cfg.Auth); err != nil {
		return nil, fmt.Errorf("resolving project IDs failed: %w", err)
	}

	AppConfig = &cfg
	return AppConfig, nil
}

// ---------- Resolve Project IDs ----------

func resolveProjectIDs(auth *CloudAuth) error {
	allProjects, err := fetchAllProjects(*auth)
	if err != nil {
		return err
	}

	regionPrefix := auth.Region + "_" // e.g. "eu-de_"
	projectMap := make(map[string]string)

	// Filter and map only matching projects
	for _, p := range allProjects {
		if p.Name == auth.Region || strings.HasPrefix(p.Name, regionPrefix) {
			projectMap[p.Name] = p.ID
		}
	}

	// If no projects specified, use all matching the region
	if len(auth.Projects) == 0 {
		for name, id := range projectMap {
			auth.Projects = append(auth.Projects, ProjectConfig{Name: name, ID: id})
		}
	} else {
		// Fill missing IDs
		for i, proj := range auth.Projects {
			if proj.ID == "" {
				if id, ok := projectMap[proj.Name]; ok {
					auth.Projects[i].ID = id
					logs.Infof("ℹ️ Resolved project %s to ID %s", proj.Name, id)
				} else {
					logs.Warnf("⚠️ Project name %s not found for region %s", proj.Name, auth.Region)
				}
			}
		}
	}

	return nil
}

// ---------- Fetch All Projects ----------

func fetchAllProjects(auth CloudAuth) ([]ProjectInfo, error) {
	creds, err := global.NewCredentialsBuilder().
		WithAk(auth.AccessKey).
		WithSk(auth.SecretKey).
		WithDomainId(auth.DomainID).
		SafeBuild()
	if err != nil {
		return nil, fmt.Errorf("failed to build credentials: %w", err)
	}

	iamEndpoint := auth.AuthURL
	if iamEndpoint == "" {
		iamEndpoint = fmt.Sprintf("https://iam.%s.otc.t-systems.com", auth.Region)
	}

	hc, _ := iam.IamClientBuilder().
		WithEndpoints([]string{iamEndpoint}).
		WithCredential(creds).
		SafeBuild()

	client := iam.NewIamClient(hc)

	req := &model.KeystoneListProjectsRequest{}
	resp, err := client.KeystoneListProjects(req)
	if err != nil {
		if se, ok := err.(*sdkerr.ServiceResponseError); ok {
			return nil, fmt.Errorf("IAM API error: %s", se.ErrorMessage)
		}
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	var result []ProjectInfo
	for _, proj := range *resp.Projects {
		if proj.Name != "" && proj.Id != "" {
			result = append(result, ProjectInfo{
				Name: proj.Name,
				ID:   proj.Id,
			})
		}
	}
	return result, nil
}
