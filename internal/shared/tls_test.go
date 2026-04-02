package shared

import (
	"bytes"
	"crypto/tls"
	"log/slog"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProductionTLSConfig_CurvePreferences(t *testing.T) {
	cfg := ProductionTLSConfig()
	require.Len(t, cfg.CurvePreferences, 2)
	assert.Equal(t, tls.X25519MLKEM768, cfg.CurvePreferences[0], "PQC curve must be first preference")
	assert.Equal(t, tls.X25519, cfg.CurvePreferences[1], "X25519 classical fallback must be second")
}

func TestProductionTLSConfig_TLS13Only(t *testing.T) {
	cfg := ProductionTLSConfig()
	assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
	assert.Equal(t, uint16(tls.VersionTLS13), cfg.MaxVersion)
}

func TestProductionTLSConfig_CipherSuites(t *testing.T) {
	cfg := ProductionTLSConfig()
	require.Len(t, cfg.CipherSuites, 3)
	assert.Equal(t, tls.TLS_AES_256_GCM_SHA384, cfg.CipherSuites[0])
	assert.Equal(t, tls.TLS_AES_128_GCM_SHA256, cfg.CipherSuites[1])
	assert.Equal(t, tls.TLS_CHACHA20_POLY1305_SHA256, cfg.CipherSuites[2])
}

func TestDevTLS_CurvePreferences(t *testing.T) {
	result, err := GenerateDevTLS()
	require.NoError(t, err)
	require.Len(t, result.TLSConfig.CurvePreferences, 2)
	assert.Equal(t, tls.X25519MLKEM768, result.TLSConfig.CurvePreferences[0], "PQC curve must be first preference")
	assert.Equal(t, tls.X25519, result.TLSConfig.CurvePreferences[1], "X25519 classical fallback must be second")
}

func TestDevTLS_TLS13Only(t *testing.T) {
	result, err := GenerateDevTLS()
	require.NoError(t, err)
	assert.Equal(t, uint16(tls.VersionTLS13), result.TLSConfig.MinVersion)
	assert.Equal(t, uint16(tls.VersionTLS13), result.TLSConfig.MaxVersion)
}

func TestDevTLS_Fingerprint(t *testing.T) {
	result, err := GenerateDevTLS()
	require.NoError(t, err)
	assert.NotEmpty(t, result.Fingerprint)
	// SHA-256 fingerprint: 64 hex chars + 31 colons = 95 chars
	assert.Len(t, result.Fingerprint, 95)
}

func TestPQCVerifyConnection_NoWarningOnPQC(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	verify := PQCVerifyConnection(logger)
	err := verify(tls.ConnectionState{
		CurveID: tls.X25519MLKEM768,
	})
	assert.NoError(t, err)
	assert.Empty(t, buf.String(), "should not log when PQC is used")
}

func TestPQCVerifyConnection_WarningOnClassicalFallback(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	verify := PQCVerifyConnection(logger)
	err := verify(tls.ConnectionState{
		CurveID: tls.X25519,
	})
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "classical TLS key exchange used instead of PQC")
	assert.Contains(t, buf.String(), "X25519MLKEM768")
}

func TestPQCVerifyConnection_NoWarningOnZeroCurve(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	verify := PQCVerifyConnection(logger)
	err := verify(tls.ConnectionState{
		CurveID: 0,
	})
	assert.NoError(t, err)
	assert.Empty(t, buf.String(), "should not log when curve ID is zero (resumed session)")
}

func TestTLSHandshake_PQCNegotiated(t *testing.T) {
	// Generate a dev TLS config (has certs + PQC curves)
	result, err := GenerateDevTLS()
	require.NoError(t, err)

	serverCfg := result.TLSConfig.Clone()

	// Start a TLS listener
	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverCfg)
	require.NoError(t, err)
	defer ln.Close()

	// Accept one connection in background
	done := make(chan tls.ConnectionState, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		tlsConn := conn.(*tls.Conn)
		_ = tlsConn.Handshake()
		done <- tlsConn.ConnectionState()
	}()

	// Client dials with PQC support
	clientCfg := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
		CurvePreferences:   []tls.CurveID{tls.X25519MLKEM768, tls.X25519},
	}

	conn, err := tls.Dial("tcp", ln.Addr().String(), clientCfg)
	require.NoError(t, err)
	defer conn.Close()

	state := <-done
	assert.Equal(t, tls.X25519MLKEM768, state.CurveID, "PQC hybrid curve should be negotiated")
}

func TestTLSHandshake_ClassicalFallback(t *testing.T) {
	result, err := GenerateDevTLS()
	require.NoError(t, err)

	serverCfg := result.TLSConfig.Clone()

	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverCfg)
	require.NoError(t, err)
	defer ln.Close()

	done := make(chan tls.ConnectionState, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		tlsConn := conn.(*tls.Conn)
		_ = tlsConn.Handshake()
		done <- tlsConn.ConnectionState()
	}()

	// Client only supports classical X25519
	clientCfg := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
		CurvePreferences:   []tls.CurveID{tls.X25519},
	}

	conn, err := tls.Dial("tcp", ln.Addr().String(), clientCfg)
	require.NoError(t, err)
	defer conn.Close()

	state := <-done
	assert.Equal(t, tls.X25519, state.CurveID, "should fall back to classical X25519")
}

func TestFormatFingerprint(t *testing.T) {
	fp := formatFingerprint("abcdef0123456789")
	assert.Equal(t, "AB:CD:EF:01:23:45:67:89", fp)
}

// Ensure the listener interface works with net.Listener for server.go compatibility.
func TestTLSListener_NetListenerCompat(t *testing.T) {
	result, err := GenerateDevTLS()
	require.NoError(t, err)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	tlsLn := tls.NewListener(ln, result.TLSConfig)
	defer tlsLn.Close()

	// Verify it satisfies net.Listener
	var _ net.Listener = tlsLn
}
