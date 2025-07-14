package server

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"

	"github.com/abdo-farag/otc-cloudeye-exporter/internal/logs"
)

// Config holds server-level configuration
type Config struct {
	EnableHTTPS bool
	HTTPPort    string // e.g., ":8080"
	HTTPSPort   string // e.g., ":8443"
	CertFile    string
	KeyFile     string
}

// fileExists returns true if the file exists and is not a directory
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Start launches both HTTP and HTTPS servers (HTTPS only if certs are present)
func Start(cfg Config, handler http.Handler) error {
	errs := make(chan error, 2)

	// 1. Start HTTP server
	go func() {
		logs.Infof("🌐 Starting HTTP server on %s", cfg.HTTPPort)
		err := http.ListenAndServe(cfg.HTTPPort, handler)
		errs <- fmt.Errorf("HTTP server error: %w", err)
	}()

	// 2. Conditionally start HTTPS server
	if cfg.EnableHTTPS {
		if !fileExists(cfg.CertFile) || !fileExists(cfg.KeyFile) {
			logs.Warnf("HTTPS enabled, but cert file (%s) or key file (%s) does not exist. Skipping HTTPS server.", cfg.CertFile, cfg.KeyFile)
		} else {
			go func() {
				logs.Infof("🔐 Starting HTTPS server on %s", cfg.HTTPSPort)
				server := &http.Server{
					Addr:    cfg.HTTPSPort,
					Handler: handler,
					TLSConfig: &tls.Config{
						MinVersion: tls.VersionTLS12,
					},
				}
				err := server.ListenAndServeTLS(cfg.CertFile, cfg.KeyFile)
				errs <- fmt.Errorf("HTTPS server error: %w", err)
			}()
		}
	}

	// Wait for first error
	return <-errs
}
