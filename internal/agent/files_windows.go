//go:build windows

package agent

import (
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/appsynergy-io/conduit/internal/protocol"
)

// sanitizePath cleans and validates an absolute path.
// On Windows, ensures the path is absolute using the current volume if needed.
// Prevents path traversal attacks (OWASP A03, V12).
func sanitizePath(path string) string {
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		// On Windows, prepend the system drive (e.g. C:\)
		sysDrive := os.Getenv("SystemDrive")
		if sysDrive == "" {
			sysDrive = "C:"
		}
		cleaned = filepath.Join(sysDrive+`\`, cleaned)
	}
	return cleaned
}

// setOwnerGroup extracts owner and group information from file info.
// On Windows, syscall.Stat_t is not available. We use os/user.Current as
// a best-effort fallback — Windows file ACLs are richer than Unix uid/gid.
func setOwnerGroup(_ fs.FileInfo, fe *protocol.FileEntry) {
	if u, err := user.Current(); err == nil {
		fe.Owner = u.Username
	}
}

// isProtectedPath returns true if the path is a critical system directory
// that must not be deleted, overwritten, or renamed.
func isProtectedPath(path string) bool {
	cleaned := strings.ToLower(filepath.Clean(path))

	// Protect drive roots (C:\, D:\, etc.)
	if len(cleaned) == 3 && cleaned[1] == ':' && cleaned[2] == '\\' {
		return true
	}

	sysDrive := strings.ToLower(os.Getenv("SystemDrive"))
	if sysDrive == "" {
		sysDrive = "c:"
	}

	protected := []string{
		sysDrive + `\windows`,
		sysDrive + `\windows\system32`,
		sysDrive + `\program files`,
		sysDrive + `\program files (x86)`,
		sysDrive + `\programdata`,
		sysDrive + `\users`,
	}

	for _, p := range protected {
		if cleaned == p {
			return true
		}
	}
	return false
}
