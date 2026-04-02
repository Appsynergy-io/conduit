package shared

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"

	"golang.org/x/crypto/acme/autocert"
)

// NewACMEManager creates an autocert manager for Let's Encrypt certificate provisioning.
// Certificates are stored in certDir and automatically renewed before expiry.
func NewACMEManager(domain, certDir string, logger *slog.Logger) (*autocert.Manager, error) {
	if domain == "" {
		return nil, fmt.Errorf("domain is required for ACME")
	}
	if certDir == "" {
		return nil, fmt.Errorf("certDir is required for ACME")
	}

	// Ensure cert directory exists with restrictive permissions (NIST AC-6).
	if err := os.MkdirAll(certDir, 0700); err != nil {
		return nil, fmt.Errorf("creating cert directory %s: %w", certDir, err)
	}

	// ACME certs use ECDSA P-256 — classical algorithm required because
	// Let's Encrypt does not issue PQC certificates yet.
	// Per CLAUDE.md: "Classical available with warning (external party involved)."
	logger.Warn("ACME certificates use classical ECDSA P-256",
		"reason", "Let's Encrypt does not issue PQC certificates",
		"domain", domain,
	)

	return &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache(certDir),
		HostPolicy: autocert.HostWhitelist(domain),
	}, nil
}

// ACMETLSConfig creates a production TLS config that uses ACME for certificate provisioning.
// Preserves PQC curve preferences and TLS 1.3 enforcement from ProductionTLSConfig.
func ACMETLSConfig(mgr *autocert.Manager) *tls.Config {
	cfg := ProductionTLSConfig()
	cfg.GetCertificate = mgr.GetCertificate
	cfg.NextProtos = append(cfg.NextProtos, "h2", "http/1.1", "acme-tls/1")
	return cfg
}
