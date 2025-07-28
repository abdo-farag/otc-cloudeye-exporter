package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/abdo-farag/otc-cloudeye-exporter/internal/config"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/logs"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/server"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/clients"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/grafana"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/collector"
)

// Global state for health checks
var (
	isReady    int32 // 0 = not ready, 1 = ready
	startTime  time.Time
)

// HealthStatus represents the health check response
type HealthStatus struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Uptime    string            `json:"uptime,omitempty"`
	Checks    map[string]string `json:"checks,omitempty"`
}

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
	for key, url := range endpointCfg.Services {
		// Replace {region} with actual region string!
		endpoint := strings.ReplaceAll(url, "{region}", endpointCfg.Region)
		serviceEndpoints[key] = endpoint
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
			collector := collector.NewCloudEyeCollector(cfg, namespaces)
			collector.AttachClient(client)
			reg.MustRegister(collector)
		}

		promhttp.HandlerFor(reg, promhttp.HandlerOpts{}).ServeHTTP(w, r)
	}
}

// grafanaDashboardHandler handles the /dashboard endpoint logic for dashboard preview.
func grafanaDashboardHandler(cfg *config.Config, projectClients []*clients.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("ns")
		if query == "" {
			http.Error(w, "Missing 'ns' (namespace) parameter", http.StatusBadRequest)
			return
		}

		namespaces := strings.Split(query, ",")
		if len(namespaces) != 1 {
			http.Error(w, "Only one namespace is allowed at a time", http.StatusBadRequest)
			return
		}
		namespace := namespaces[0]

		if !strings.HasPrefix(namespace, "SYS.") {
			http.Error(w, "Invalid namespace", http.StatusBadRequest)
			return
		}

		var exports []collector.MetricExport
		for _, client := range projectClients {
			exports = collector.ExportMetricValuesBatch(client, cfg, namespace)
			if len(exports) > 0 {
				logs.Infof("✅ Exported %d metric values from namespace %s", len(exports), namespace)
				break
			}
		}

		if len(exports) == 0 {
			http.Error(w, "No metric data found", http.StatusNotFound)
			return
		}

		board := grafana.NewDefaultDashboard(namespace)
		board.AddFromMetricValues(namespace, exports)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(board)
	}
}

// grafanaAlertsHandler handles the /alert endpoint logic for alerts preview.
func grafanaAlertsHandler(cfg *config.Config, projectClients []*clients.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("ns")
		if query == "" {
			http.Error(w, "Missing 'ns' (namespace) parameter", http.StatusBadRequest)
			return
		}

		namespaces := strings.Split(query, ",")
		if len(namespaces) != 1 {
			http.Error(w, "Only one namespace is allowed at a time", http.StatusBadRequest)
			return
		}
		namespace := namespaces[0]

		if !strings.HasPrefix(namespace, "SYS.") {
			http.Error(w, "Invalid namespace", http.StatusBadRequest)
			return
		}

		var exports []collector.MetricExport
		for _, client := range projectClients {
			exports = collector.ExportMetricValuesBatch(client, cfg, namespace)
			if len(exports) > 0 {
				logs.Infof("✅ Exported %d metric values for alerts from namespace %s", len(exports), namespace)
				break
			}
		}

		if len(exports) == 0 {
			http.Error(w, "No metric data found", http.StatusNotFound)
			return
		}

		alerts := grafana.NewAlertBundle(namespace)
		alerts.AddFromMetricValues(namespace, exports)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(alerts)
	}
}

// healthHandler handles the /health endpoint for Docker and K8s health checks
func healthHandler(projectClients []*clients.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		status := HealthStatus{
			Status:    "healthy",
			Timestamp: time.Now(),
			Uptime:    time.Since(startTime).String(),
			Checks:    make(map[string]string),
		}

		// Basic health checks
		status.Checks["server"] = "ok"
		
		// Check if clients are available (basic connectivity)
		if len(projectClients) > 0 {
			status.Checks["clients"] = "ok"
		} else {
			status.Checks["clients"] = "no_clients"
			status.Status = "degraded"
		}

		// Return appropriate HTTP status
		if status.Status == "healthy" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(status)
	}
}

// readinessHandler handles the /ready endpoint for K8s readiness probes
func readinessHandler(projectClients []*clients.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		if atomic.LoadInt32(&isReady) == 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(HealthStatus{
				Status:    "not_ready",
				Timestamp: time.Now(),
				Checks:    map[string]string{"initialization": "in_progress"},
			})
			return
		}

		status := HealthStatus{
			Status:    "ready",
			Timestamp: time.Now(),
			Checks:    make(map[string]string),
		}

		// Check readiness criteria
		status.Checks["server"] = "ready"
		
		if len(projectClients) > 0 {
			status.Checks["clients"] = "ready"
		} else {
			status.Checks["clients"] = "no_clients"
			status.Status = "not_ready"
		}

		if status.Status == "ready" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(status)
	}
}

// livenessHandler handles the /live endpoint for K8s liveness probes
func livenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		
		json.NewEncoder(w).Encode(HealthStatus{
			Status:    "alive",
			Timestamp: time.Now(),
			Uptime:    time.Since(startTime).String(),
		})
	}
}

func main() {
	// Initialize start time for uptime tracking
	startTime = time.Now()
	
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

	// Mark as ready after successful initialization
	atomic.StoreInt32(&isReady, 1)

	// --- Step 4: Register HTTP endpoints ---
	// Prometheus metrics endpoint
	http.HandleFunc(cfg.Global.MetricPath, prometheusHandler(cfg, projectClients, parsedNamespaces))
	
	// Grafana dashboard preview endpoint
	http.HandleFunc("/dashboards", grafanaDashboardHandler(cfg, projectClients))
	
	// Grafana alerts preview endpoint
	http.HandleFunc("/alerts", grafanaAlertsHandler(cfg, projectClients))

	// Kubernetes-standard health check endpoints
	http.HandleFunc("/health", healthHandler(projectClients))      // General health check
	http.HandleFunc("/healthz", healthHandler(projectClients))     // K8s health check alias
	http.HandleFunc("/ready", readinessHandler(projectClients))    // K8s readiness probe
	http.HandleFunc("/readyz", readinessHandler(projectClients))   // K8s readiness probe alias
	http.HandleFunc("/live", livenessHandler())                    // K8s liveness probe
	http.HandleFunc("/livez", livenessHandler())                   // K8s liveness probe alias

	// --- Step 5: Start Server ---
	logs.Infof("📡 Prometheus metrics at: %s?ns=%s", cfg.Global.MetricPath, cfg.Global.Namespaces)
	logs.Infof("📊 Grafana Dashboard preview at: /dashboards?ns=<namespace>")
	logs.Infof("🚨 Grafana Alerts preview at: /alerts?ns=<namespace>")
	logs.Infof("🏥 Health endpoints: /health, /ready, /live (with /healthz, /readyz, /livez aliases)")

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