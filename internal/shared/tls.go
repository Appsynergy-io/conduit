package shared

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"strings"
	"time"
)

// pqcCurvePreferences defines the set of allowed TLS key exchange mechanisms.
// Go's internal preference order selects PQC (X25519MLKEM768) when the client
// supports it; classical curves (X25519, P-256, P-384) cover older clients.
// P-256 and P-384 are required for Windows Schannel compatibility (PowerShell
// 5.1 / .NET Framework on Windows 10 does not support TLS 1.3, X25519MLKEM768,
// and some builds lack X25519 — without NIST curves the handshake fails with
// "An unexpected error occurred on a send").
var pqcCurvePreferences = []tls.CurveID{
	tls.X25519MLKEM768,
	tls.X25519,
	tls.CurveP256,
	tls.CurveP384,
}

// DevTLSResult holds the generated self-signed cert and fingerprint for dev mode.
type DevTLSResult struct {
	TLSConfig   *tls.Config
	Fingerprint string // SHA-256 fingerprint for agent pinning
}

// GenerateDevTLS creates a self-signed TLS 1.3 certificate for development.
// Uses ECDSA P-256 (classical, with warning — NIST SP 800-131A).
func GenerateDevTLS() (*DevTLSResult, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating ECDSA key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generating serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Conduit Dev"},
			CommonName:   "localhost",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("creating certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parsing certificate: %w", err)
	}

	tlsCert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
		Leaf:        cert,
	}

	fingerprint := sha256.Sum256(certDER)
	fpHex := hex.EncodeToString(fingerprint[:])

	tlsConfig := &tls.Config{
		Certificates:     []tls.Certificate{tlsCert},
		MinVersion:       tls.VersionTLS12,
		MaxVersion:       tls.VersionTLS13,
		CurvePreferences: pqcCurvePreferences,
	}

	return &DevTLSResult{
		TLSConfig:   tlsConfig,
		Fingerprint: formatFingerprint(fpHex),
	}, nil
}

// ProductionTLSConfig returns a TLS 1.3 config for production (certs loaded separately via ACME).
func ProductionTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS13,
		MaxVersion: tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
		CurvePreferences: pqcCurvePreferences,
	}
}

// PQCVerifyConnection returns a VerifyConnection callback that logs when a TLS
// connection falls back to classical key exchange instead of PQC.
// Per CLAUDE.md: "Every use of a classical algorithm where PQC was available is logged."
func PQCVerifyConnection(logger *slog.Logger) func(tls.ConnectionState) error {
	return func(cs tls.ConnectionState) error {
		if cs.CurveID != tls.X25519MLKEM768 && cs.CurveID != 0 {
			logger.Warn("classical TLS key exchange used instead of PQC",
				"negotiated_curve", cs.CurveID.String(),
				"expected", "X25519MLKEM768",
				"server_name", cs.ServerName,
				"tls_version", cs.Version,
				"reason", "client does not support ML-KEM",
			)
		}
		return nil
	}
}

// formatFingerprint converts a hex string to colon-separated pairs.
func formatFingerprint(hex string) string {
	var parts []string
	for i := 0; i < len(hex); i += 2 {
		end := i + 2
		if end > len(hex) {
			end = len(hex)
		}
		parts = append(parts, hex[i:end])
	}
	return strings.ToUpper(strings.Join(parts, ":"))
}
