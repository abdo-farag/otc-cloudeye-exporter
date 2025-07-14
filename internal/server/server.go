package server

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
)

// Config holds server-level configuration
type Config struct {
	EnableHTTPS bool
	HTTPPort    string // e.g., ":8080"
	HTTPSPort   string // e.g., ":8443"
	CertFile    string
	KeyFile     string
}

// Start launches both HTTP and HTTPS servers
func Start(cfg Config, handler http.Handler) error {
	errs := make(chan error, 2)

	// 1. Start HTTP server (no redirect, serves real content)
	go func() {
		log.Printf("🌐 Starting HTTP server on %s", cfg.HTTPPort)
		err := http.ListenAndServe(cfg.HTTPPort, handler)
		errs <- fmt.Errorf("HTTP server error: %w", err)
	}()

	// 2. Start HTTPS server (if enabled)
	if cfg.EnableHTTPS {
		go func() {
			log.Printf("🔐 Starting HTTPS server on %s", cfg.HTTPSPort)
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

	// Wait for first error
	return <-errs
}
