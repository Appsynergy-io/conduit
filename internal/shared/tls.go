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
	"math/big"
	"net"
	"strings"
	"time"
)

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
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
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
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
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
