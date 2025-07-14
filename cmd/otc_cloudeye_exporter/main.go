package main

import (
	"flag"
	"log"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/abdo-farag/otc-cloudeye-exporter/internal/config"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/server"
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
			log.Printf("⚠️ No endpoint found for namespace %q in endpoints.yml", ns)
			continue
		}
		serviceEndpoints[ns] = endpoint
	}
	return serviceEndpoints
}

// prometheusHandler handles the /metrics endpoint logic.
func prometheusHandler(defaultNamespaces []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var namespaces []string
		if ns := r.URL.Query().Get("ns"); ns != "" {
			namespaces = strings.Split(ns, ",")
			log.Printf("🔍 Requested namespaces: %v", namespaces)
		} else {
			namespaces = defaultNamespaces
			log.Printf("🔍 Using static namespaces: %v", namespaces)
		}

		reg := prometheus.NewRegistry()
		// TODO: Register your collectors with reg here!

		promhttp.HandlerFor(reg, promhttp.HandlerOpts{}).ServeHTTP(w, r)
	}
}

func main() {
	// --- Step 1: Load config ---
	var configPath string
	flag.StringVar(&configPath, "config", "clouds.yaml", "Path to the config YAML file")
	flag.Parse()

	cfg, endpointCfg, err := loadConfigs(configPath, "endpoints.yml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	parsedNamespaces := parseNamespaces(cfg.Global.Namespaces)
	serviceEndpoints := getServiceEndpoints(parsedNamespaces, endpointCfg)
	log.Printf("Using service endpoints: %v", serviceEndpoints)


	// TODO: Initialize OTC clients here. Remove the dummy error!

	// --- Step 2: Register Prometheus metrics endpoint ---
	http.HandleFunc(cfg.Global.MetricPath, prometheusHandler(parsedNamespaces))

	// --- Step 3: Start Server ---
	log.Printf("🚀 Exporter listening on HTTP %s | HTTPS %s (if enabled)", cfg.Global.Port, cfg.Global.HTTPSPort)
	log.Printf("📡 CloudEye metrics at: %s?ns=%s", cfg.Global.MetricPath, cfg.Global.Namespaces)

	srvCfg := server.Config{
		EnableHTTPS: cfg.Global.EnableHTTPS,
		HTTPPort:    cfg.Global.Port,
		HTTPSPort:   cfg.Global.HTTPSPort,
		CertFile:    cfg.Global.TLSCert,
		KeyFile:     cfg.Global.TLSKey,
	}
	if err := server.Start(srvCfg, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
