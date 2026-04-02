package shared

import (
	"bytes"
	"crypto/tls"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewACMEManager_Success(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))

	mgr, err := NewACMEManager("example.com", dir, logger)
	require.NoError(t, err)
	require.NotNil(t, mgr)
	assert.NotNil(t, mgr.Prompt)
	assert.NotNil(t, mgr.Cache)
	assert.NotNil(t, mgr.HostPolicy)
}

func TestNewACMEManager_EmptyDomain(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))

	mgr, err := NewACMEManager("", dir, logger)
	assert.Nil(t, mgr)
	assert.ErrorContains(t, err, "domain is required")
}

func TestNewACMEManager_EmptyCertDir(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))

	mgr, err := NewACMEManager("example.com", "", logger)
	assert.Nil(t, mgr)
	assert.ErrorContains(t, err, "certDir is required")
}

func TestNewACMEManager_CreatesCertDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "certs")
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))

	_, err := NewACMEManager("example.com", dir, logger)
	require.NoError(t, err)

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.Equal(t, os.FileMode(0700), info.Mode().Perm(), "cert dir must have 0700 permissions")
}

func TestNewACMEManager_LogsClassicalWarning(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	_, err := NewACMEManager("example.com", t.TempDir(), logger)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "ACME certificates use classical ECDSA P-256")
	assert.Contains(t, buf.String(), "Let's Encrypt")
}

func TestNewACMEManager_HostPolicy(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	mgr, err := NewACMEManager("example.com", t.TempDir(), logger)
	require.NoError(t, err)

	// Allowed host
	err = mgr.HostPolicy(nil, "example.com")
	assert.NoError(t, err)

	// Disallowed host
	err = mgr.HostPolicy(nil, "evil.com")
	assert.Error(t, err)
}

func TestACMETLSConfig_HasGetCertificate(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	mgr, err := NewACMEManager("example.com", t.TempDir(), logger)
	require.NoError(t, err)

	cfg := ACMETLSConfig(mgr)
	assert.NotNil(t, cfg.GetCertificate)
}

func TestACMETLSConfig_PreservesPQC(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	mgr, err := NewACMEManager("example.com", t.TempDir(), logger)
	require.NoError(t, err)

	cfg := ACMETLSConfig(mgr)
	require.Len(t, cfg.CurvePreferences, 2)
	assert.Equal(t, tls.X25519MLKEM768, cfg.CurvePreferences[0])
	assert.Equal(t, tls.X25519, cfg.CurvePreferences[1])
}

func TestACMETLSConfig_TLS13Only(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	mgr, err := NewACMEManager("example.com", t.TempDir(), logger)
	require.NoError(t, err)

	cfg := ACMETLSConfig(mgr)
	assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
	assert.Equal(t, uint16(tls.VersionTLS13), cfg.MaxVersion)
}

func TestACMETLSConfig_NextProtos(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	mgr, err := NewACMEManager("example.com", t.TempDir(), logger)
	require.NoError(t, err)

	cfg := ACMETLSConfig(mgr)
	assert.Contains(t, cfg.NextProtos, "h2")
	assert.Contains(t, cfg.NextProtos, "http/1.1")
	assert.Contains(t, cfg.NextProtos, "acme-tls/1")
}
