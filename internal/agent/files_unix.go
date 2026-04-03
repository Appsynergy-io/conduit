//go:build linux || darwin

package agent

import (
	"io/fs"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/appsynergy-io/conduit/internal/protocol"
)

// sanitizePath cleans and validates an absolute path.
// Prevents path traversal attacks (OWASP A03, V12).
func sanitizePath(path string) string {
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		cleaned = filepath.Join("/", cleaned)
	}
	return cleaned
}

// setOwnerGroup extracts owner and group information from file info.
func setOwnerGroup(info fs.FileInfo, fe *protocol.FileEntry) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}

	if u, err := user.LookupId(strconv.Itoa(int(stat.Uid))); err == nil {
		fe.Owner = u.Username
	} else {
		fe.Owner = strconv.Itoa(int(stat.Uid))
	}

	if g, err := user.LookupGroupId(strconv.Itoa(int(stat.Gid))); err == nil {
		fe.Group = g.Name
	} else {
		fe.Group = strconv.Itoa(int(stat.Gid))
	}
}

// isProtectedPath returns true if the path is a critical system directory
// that must not be deleted, overwritten, or renamed.
func isProtectedPath(path string) bool {
	switch path {
	case "/", "/root", "/home", "/etc", "/var", "/usr",
		"/bin", "/sbin", "/lib", "/lib64", "/boot", "/dev",
		"/proc", "/sys", "/tmp", "/run", "/opt", "/srv":
		return true
	}
	return false
}
