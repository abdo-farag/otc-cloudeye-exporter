package main

import (
	"flag"
	"net/http"
	"strings"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/abdo-farag/otc-cloudeye-exporter/internal/config"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/logs"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/server"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/clients"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/collector"
)

// parseNamespaces splits a comma-separated list of namespaces into a slice.
func parseNamespaces(ns string) []string {
	if ns == "" {
		return nil
	}
	return strings.Split(ns, ",")
}

// loadConfigs loads global and endpoint configs.
func loadConfigs(globalConfigPath, endpointConfigPath string) (*config.Config, *config.EndpointConfig, error) {
	cfg, err := config.LoadConfig(globalConfigPath)
	if err != nil {
		return nil, nil, err
	}
	endpointCfg, err := config.LoadEndpointConfig(endpointConfigPath)
	if err != nil {
		return nil, nil, err
	}
	return cfg, endpointCfg, nil
}

// getServiceEndpoints builds a namespace->endpoint map.
func getServiceEndpoints(parsedNamespaces []string, endpointCfg *config.EndpointConfig) map[string]string {
	serviceEndpoints := make(map[string]string)
	for _, ns := range parsedNamespaces {
		endpoint, err := endpointCfg.GetServiceEndpoint(ns)
		if err != nil {
			logs.Warnf("No endpoint found for namespace %q in endpoints.yml", ns)
			continue
		}
		serviceEndpoints[ns] = endpoint
	}
	return serviceEndpoints
}

// validateProject checks if a project exists in the configured region
func validateProject(auth *config.CloudAuth, projectName string) error {
	// Fetch the list of all projects using the full CloudAuth struct (not just the region)
	allProjects, err := config.FetchAllProjects(*auth)
	if err != nil {
		logs.Errorf("Error fetching projects for region %s: %v", auth.Region, err)
		return err
	}

	// Log the fetched projects for debugging purposes
	logs.Infof("Fetched Projects: %v", allProjects)

	// Check if the project exists
	for _, proj := range allProjects {
		if proj.Name == projectName {
			return nil // Project found
		}
	}

	// If project is not found, log the error and return
	logs.Errorf("Project %s not found in region %s", projectName, auth.Region)
	return fmt.Errorf("project %s not found in region %s", projectName, auth.Region)
}

// prometheusHandler handles the /metrics endpoint logic.
func prometheusHandler(cfg *config.Config, projectClients []*clients.Clients, defaultNamespaces []string) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var namespaces []string
        if ns := r.URL.Query().Get("ns"); ns != "" {
            namespaces = strings.Split(ns, ",")
            logs.Infof("Requested namespaces: %v", namespaces)
        } else {
            namespaces = defaultNamespaces
            logs.Infof("Using static namespaces: %v", namespaces)
        }

        reg := prometheus.NewRegistry()

        // Register your collectors for each client
        for _, client := range projectClients {
            collector := collectors.NewCloudEyeCollector(cfg, namespaces)
            collector.AttachClient(client)
            reg.MustRegister(collector)
        }

        promhttp.HandlerFor(reg, promhttp.HandlerOpts{}).ServeHTTP(w, r)
    }
}


func main() {
	// --- Step 0: Initialize logging ---
	logs.InitLog("logs.yml")

	// --- Step 1: Load config ---
	var configPath string
	flag.StringVar(&configPath, "config", "clouds.yml", "Path to the config YAML file")
	flag.Parse()

	cfg, endpointCfg, err := loadConfigs(configPath, "endpoints.yml")
	if err != nil {
		logs.Fatalf("Failed to load config: %v", err)
	}

	parsedNamespaces := parseNamespaces(cfg.Global.Namespaces)
	serviceEndpoints := getServiceEndpoints(parsedNamespaces, endpointCfg)
	logs.Infof("Using service endpoints: %v", serviceEndpoints)

	// Log endpoint configuration for each namespace
	for _, ns := range parsedNamespaces {
		endpoint, err := endpointCfg.GetServiceEndpoint(ns)
		if err != nil {
			logs.Warnf("No endpoint found for namespace %q in endpoints.yml", ns)
			continue
		}
		serviceEndpoints[ns] = endpoint
	}

	// --- Step 2: Validate Projects Before Initializing Clients ---
	// Validate that projects are correct before trying to initialize any clients
	for _, project := range cfg.Auth.Projects {
		// Pass the whole `CloudAuth` struct to validate the project
		if err := validateProject(&cfg.Auth, project.Name); err != nil {
			// Log the error but do not stop processing other projects
			logs.Errorf("Skipping project %s: %v", project.Name, err)
			continue // Skip this project and continue with the next one
		}
	}

	// --- Step 3: Initialize project clients with service endpoints ---
	projectClients, err := clients.NewClientsWithEndpoints(cfg, &config.EndpointConfig{
		Region:   endpointCfg.Region,
		Services: serviceEndpoints,
	})
	if err != nil {
		logs.Fatalf("Failed to initialize OTC clients: %v", err)
	}
	logs.Infof("OTC clients initialized successfully for %d projects", len(projectClients))

	// --- Step 2: Register Prometheus metrics endpoint ---
	http.HandleFunc(cfg.Global.MetricPath, prometheusHandler(cfg, projectClients, parsedNamespaces))

	// --- Step 3: Start Server ---
	logs.Infof("📡 CloudEye metrics at: %s?ns=%s", cfg.Global.MetricPath, cfg.Global.Namespaces)

	// Ensure the clients are properly closed after server starts or an error happens
	defer func() {
		logs.Infof("Shutting down and closing clients...")
		for _, client := range projectClients {
			client.Close()
		}
		logs.Info("All clients closed.")
	}()
	
	srvCfg := server.Config{
		EnableHTTPS: cfg.Global.EnableHTTPS,
		HTTPPort:    cfg.Global.Port,
		HTTPSPort:   cfg.Global.HTTPSPort,
		CertFile:    cfg.Global.TLSCert,
		KeyFile:     cfg.Global.TLSKey,
	}
	if err := server.Start(srvCfg, nil); err != nil {
		logs.Fatalf("Server failed: %v", err)
	}
}
