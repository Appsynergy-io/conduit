package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/appsynergy-io/conduit/internal/apierror"
)

// Allowed OS and architecture combinations for agent binary downloads.
// Validated against allowlists to prevent path traversal (OWASP A03, V12).
var (
	allowedOS   = map[string]bool{"linux": true, "darwin": true, "windows": true}
	allowedArch = map[string]bool{"amd64": true, "arm64": true}
	safeNameRe  = regexp.MustCompile(`^[a-z0-9]+$`)
)

// handleDownloadAgent serves agent binaries for a given OS/arch.
// GET /api/v1/download/agent?os=linux&arch=amd64
// Public endpoint (no auth) — enables curl one-liners for agent install.
func (s *Server) handleDownloadAgent(w http.ResponseWriter, r *http.Request) {
	osParam := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("os")))
	archParam := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("arch")))

	if osParam == "" || archParam == "" {
		apierror.BadRequest(w, r, "os and arch query parameters are required.", nil)
		return
	}

	// Strict allowlist validation (OWASP A03 — path traversal prevention)
	if !allowedOS[osParam] || !allowedArch[archParam] {
		apierror.BadRequest(w, r, "Unsupported os/arch combination.", nil)
		return
	}
	if !safeNameRe.MatchString(osParam) || !safeNameRe.MatchString(archParam) {
		apierror.BadRequest(w, r, "Invalid os/arch format.", nil)
		return
	}

	// Build filename: conduit-linux-amd64, conduit-windows-amd64.exe
	filename := fmt.Sprintf("conduit-%s-%s", osParam, archParam)
	downloadName := filename
	if osParam == "windows" {
		filename += ".exe"
		downloadName += ".exe"
	}

	binDir := s.cfg.Server.BinariesDir
	if binDir == "" {
		apierror.NotFound(w, r, "Binary hosting is not configured.", nil)
		return
	}

	// filepath.Clean prevents path traversal (V12, A03)
	fullPath := filepath.Join(filepath.Clean(binDir), filename)

	// Verify the resolved path is within the binaries directory
	absDir, err := filepath.Abs(binDir)
	if err != nil {
		apierror.Internal(w, r, fmt.Errorf("resolving binaries dir: %w", err))
		return
	}
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		apierror.Internal(w, r, fmt.Errorf("resolving binary path: %w", err))
		return
	}
	if !strings.HasPrefix(absPath, absDir+string(os.PathSeparator)) && absPath != absDir {
		apierror.BadRequest(w, r, "Invalid path.", nil)
		return
	}

	f, err := os.Open(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			apierror.NotFound(w, r, fmt.Sprintf("Agent binary not available for %s/%s.", osParam, archParam), nil)
			return
		}
		apierror.Internal(w, r, fmt.Errorf("opening binary: %w", err))
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		apierror.Internal(w, r, fmt.Errorf("stat binary: %w", err))
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", downloadName))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
	w.Header().Set("Cache-Control", "public, max-age=3600")

	http.ServeContent(w, r, downloadName, stat.ModTime(), f)
}

// handleInstallScript serves a POSIX shell script that detects OS/arch,
// downloads the agent binary, and runs `conduit join`.
// GET /install.sh
// Public endpoint — used in curl|sh one-liners.
func (s *Server) handleInstallScript(w http.ResponseWriter, r *http.Request) {
	// Determine the server's base URL for download links
	baseURL := s.serverBaseURL(r)
	devMode := s.cfg.Server.Mode == "dev"

	script := generateInstallScript(baseURL, devMode)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(script))
}

// serverBaseURL returns the public base URL of this server.
func (s *Server) serverBaseURL(r *http.Request) string {
	if s.cfg.Server.Domain != "" {
		return "https://" + s.cfg.Server.Domain
	}
	// Dev mode: use the request host
	scheme := "https"
	host := r.Host
	if host == "" {
		host = "localhost" + s.cfg.Server.HTTPAddr
	}
	return scheme + "://" + host
}

// generateInstallScript returns a POSIX-compatible install script.
// When devMode is true, the script auto-enables insecure flags for self-signed certs.
func generateInstallScript(baseURL string, devMode bool) string {
	devInsecureDefault := `DEV_INSECURE=""`
	curlFlag := "-sSL"
	if devMode {
		devInsecureDefault = `DEV_INSECURE="--dev-insecure"`
		curlFlag = "-sSLk"
	}

	return `#!/bin/sh
# Conduit Agent Installer
# Usage: curl ` + curlFlag + ` ` + baseURL + `/install.sh | sh -s -- <join-token>
set -e

CONDUIT_URL="` + baseURL + `"

# --- Parse arguments ---
` + devInsecureDefault + `
TOKEN=""
for arg in "$@"; do
  case "$arg" in
    --dev-insecure) DEV_INSECURE="--dev-insecure" ;;
    *) TOKEN="$arg" ;;
  esac
done

if [ -z "$TOKEN" ]; then
  echo "Usage: curl -sSL ${CONDUIT_URL}/install.sh | sh -s -- [--dev-insecure] <join-token>"
  exit 1
fi

# --- Detect OS ---
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux)  OS="linux" ;;
  darwin) OS="darwin" ;;
  *)      echo "Unsupported OS: $OS"; exit 1 ;;
esac

# --- Detect architecture ---
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)  ARCH="amd64" ;;
  aarch64|arm64)  ARCH="arm64" ;;
  *)              echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

echo "Detected: ${OS}/${ARCH}"

# --- Download binary ---
DOWNLOAD_URL="${CONDUIT_URL}/api/v1/download/agent?os=${OS}&arch=${ARCH}"
INSTALL_DIR="/usr/local/bin"
BINARY="${INSTALL_DIR}/conduit"

echo "Downloading conduit agent from ${DOWNLOAD_URL}..."

CURL_OPTS="-sSL -f"
if [ -n "$DEV_INSECURE" ]; then
  CURL_OPTS="$CURL_OPTS -k"
fi

if command -v curl >/dev/null 2>&1; then
  curl $CURL_OPTS -o "$BINARY" "$DOWNLOAD_URL"
elif command -v wget >/dev/null 2>&1; then
  WGET_OPTS="-q -O"
  if [ -n "$DEV_INSECURE" ]; then
    WGET_OPTS="$WGET_OPTS --no-check-certificate"
  fi
  wget $WGET_OPTS "$BINARY" "$DOWNLOAD_URL"
else
  echo "Error: curl or wget is required"
  exit 1
fi

chmod +x "$BINARY"
echo "Installed conduit to ${BINARY}"

# --- Join server ---
echo "Joining server..."
if [ -n "$DEV_INSECURE" ]; then
  "$BINARY" join "$CONDUIT_URL" "$TOKEN" --dev-insecure
else
  "$BINARY" join "$CONDUIT_URL" "$TOKEN"
fi

echo "Done. Agent installed and connected."
`
}

// handleListAvailableBinaries returns which OS/arch binaries are available.
// GET /api/v1/download/agent/platforms
// Public endpoint — used by the frontend to show available platforms.
func (s *Server) handleListAvailableBinaries(w http.ResponseWriter, r *http.Request) {
	binDir := s.cfg.Server.BinariesDir

	type platform struct {
		OS   string `json:"os"`
		Arch string `json:"arch"`
	}

	var platforms []platform

	if binDir != "" {
		for osName := range allowedOS {
			for arch := range allowedArch {
				filename := fmt.Sprintf("conduit-%s-%s", osName, arch)
				if osName == "windows" {
					filename += ".exe"
				}
				fullPath := filepath.Join(filepath.Clean(binDir), filename)
				if _, err := os.Stat(fullPath); err == nil {
					platforms = append(platforms, platform{OS: osName, Arch: arch})
				}
			}
		}
	}

	if platforms == nil {
		platforms = []platform{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"platforms":      platforms,
		"installScript":  s.serverBaseURL(r) + "/install.sh",
		"devMode":        s.cfg.Server.Mode == "dev",
	})
}
