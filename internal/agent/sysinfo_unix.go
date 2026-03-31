//go:build linux || darwin

package agent

import (
	"syscall"

	"github.com/appsynergy-io/conduit/internal/protocol"
)

// collectDiskUsage reads filesystem usage via statfs.
func collectDiskUsage(path string, info *protocol.AgentInfoPayload) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return
	}

	info.DiskTotal = stat.Blocks * uint64(stat.Bsize)
	info.DiskUsed = (stat.Blocks - stat.Bfree) * uint64(stat.Bsize)
}
