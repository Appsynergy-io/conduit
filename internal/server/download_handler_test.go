package server_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/appsynergy-io/conduit/internal/auth"
	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/server"
	"github.com/appsynergy-io/conduit/internal/shared"
)

// newTestServerWithBinaries creates a test server with a temp binaries directory.
func newTestServerWithBinaries(t *testing.T) (*server.Server, string) {
	t.Helper()

	database, err := db.New(context.Background(), ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })

	jwtMgr, err := auth.NewJWTManager("test", 15*time.Minute, 24*time.Hour)
	require.NoError(t, err)

	binDir := t.TempDir()

	cfg := &shared.Config{
		Server: shared.ServerConfig{
			Mode:        "dev",
			HTTPAddr:    ":0",
			BinariesDir: binDir,
		},
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	srv := server.New(cfg, database, jwtMgr, nil, logger, nil)
	return srv, binDir
}

// seedBinary creates a fake binary file in the binaries directory.
func seedBinary(t *testing.T, binDir, osName, arch string) {
	t.Helper()
	filename := "conduit-" + osName + "-" + arch
	if osName == "windows" {
		filename += ".exe"
	}
	err := os.WriteFile(filepath.Join(binDir, filename), []byte("fake-binary-content"), 0755)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// GET /api/v1/download/agent — Binary Download
// ---------------------------------------------------------------------------

func TestDownloadAgent_Success(t *testing.T) {
	srv, binDir := newTestServerWithBinaries(t)
	seedBinary(t, binDir, "linux", "amd64")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/download/agent?os=linux&arch=amd64", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/octet-stream", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Content-Disposition"), "conduit-linux-amd64")
	assert.Equal(t, "fake-binary-content", w.Body.String())
}

func TestDownloadAgent_Windows(t *testing.T) {
	srv, binDir := newTestServerWithBinaries(t)
	seedBinary(t, binDir, "windows", "amd64")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/download/agent?os=windows&arch=amd64", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Disposition"), "conduit-windows-amd64.exe")
}

func TestDownloadAgent_MissingParams(t *testing.T) {
	srv, _ := newTestServerWithBinaries(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/download/agent?os=linux", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDownloadAgent_UnsupportedOS(t *testing.T) {
	srv, _ := newTestServerWithBinaries(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/download/agent?os=freebsd&arch=amd64", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDownloadAgent_PathTraversal(t *testing.T) {
	srv, _ := newTestServerWithBinaries(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/download/agent?os=../../../etc&arch=passwd", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDownloadAgent_NotFound(t *testing.T) {
	srv, _ := newTestServerWithBinaries(t)

	// No binary seeded for darwin/arm64
	req := httptest.NewRequest(http.MethodGet, "/api/v1/download/agent?os=darwin&arch=arm64", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDownloadAgent_NoBinariesDir(t *testing.T) {
	// Server with empty binaries dir
	database, err := db.New(context.Background(), ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })

	jwtMgr, err := auth.NewJWTManager("test", 15*time.Minute, 24*time.Hour)
	require.NoError(t, err)

	cfg := &shared.Config{
		Server: shared.ServerConfig{Mode: "dev", HTTPAddr: ":0"},
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	srv := server.New(cfg, database, jwtMgr, nil, logger, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/download/agent?os=linux&arch=amd64", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ---------------------------------------------------------------------------
// GET /install.sh — Install Script
// ---------------------------------------------------------------------------

func TestInstallScript_Success(t *testing.T) {
	srv, _ := newTestServerWithBinaries(t)

	req := httptest.NewRequest(http.MethodGet, "/install.sh", nil)
	req.Host = "conduit.example.com"
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/plain; charset=utf-8", w.Header().Get("Content-Type"))

	body := w.Body.String()
	assert.Contains(t, body, "#!/bin/sh")
	assert.Contains(t, body, "conduit")
	assert.Contains(t, body, "uname")
	assert.Contains(t, body, "curl")
}

// ---------------------------------------------------------------------------
// GET /api/v1/download/agent/platforms — List Available Binaries
// ---------------------------------------------------------------------------

func TestListPlatforms_Empty(t *testing.T) {
	srv, _ := newTestServerWithBinaries(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/download/agent/platforms", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	platforms := resp["platforms"].([]interface{})
	assert.Empty(t, platforms)
}

func TestListPlatforms_WithBinaries(t *testing.T) {
	srv, binDir := newTestServerWithBinaries(t)
	seedBinary(t, binDir, "linux", "amd64")
	seedBinary(t, binDir, "darwin", "arm64")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/download/agent/platforms", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	platforms := resp["platforms"].([]interface{})
	assert.Len(t, platforms, 2)

	// Verify installScript URL is present
	assert.NotEmpty(t, resp["installScript"])

	// Verify devMode flag is present for dev-mode server
	assert.Equal(t, true, resp["devMode"])
}

func TestListPlatforms_DevModeFlag(t *testing.T) {
	// Production mode server
	database, err := db.New(context.Background(), ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })

	jwtMgr, err := auth.NewJWTManager("test", 15*time.Minute, 24*time.Hour)
	require.NoError(t, err)

	cfg := &shared.Config{
		Server: shared.ServerConfig{Mode: "production", HTTPAddr: ":0", BinariesDir: t.TempDir()},
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	srv := server.New(cfg, database, jwtMgr, nil, logger, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/download/agent/platforms", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, false, resp["devMode"])
}

func TestInstallScript_DevModeIncludesInsecureFlags(t *testing.T) {
	srv, _ := newTestServerWithBinaries(t) // dev mode

	req := httptest.NewRequest(http.MethodGet, "/install.sh", nil)
	req.Host = "64.112.14.6:8443"
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	body := w.Body.String()
	assert.Contains(t, body, `DEV_INSECURE="--dev-insecure"`)
	assert.Contains(t, body, "-sSLk")
}
