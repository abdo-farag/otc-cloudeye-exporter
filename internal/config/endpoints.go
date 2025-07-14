package config

import (
	"fmt"
	"strings"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/abdo-farag/otc-cloudeye-exporter/internal/logs"
)

type EndpointConfig struct {
	Region   string            `yaml:"region"`
	Services map[string]string `yaml:"services"`
}

// LoadEndpointConfig loads and parses endpoints.yaml
func LoadEndpointConfig(path string) (*EndpointConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		logs.Errorf("Failed to read endpoint config %s: %v", path, err)
		return nil, err
	}

	var cfg EndpointConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		logs.Errorf("Failed to parse endpoint config %s: %v", path, err)
		return nil, err
	}

	return &cfg, nil
}

// GetServiceEndpoint returns the full endpoint URL with region filled in
func (e *EndpointConfig) GetServiceEndpoint(service string) (string, error) {
	tpl, ok := e.Services[service]
	if !ok {
		logs.Warnf("Service %q not found in endpoint config for region %s", service, e.Region)
		return "", fmt.Errorf("service %q not found in endpoint config", service)
	}
	return strings.ReplaceAll(tpl, "{region}", e.Region), nil
}
