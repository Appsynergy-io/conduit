//go:build windows

package agent

import (
	"github.com/appsynergy-io/conduit/internal/protocol"
)

// collectDiskUsage is a stub on Windows.
func collectDiskUsage(_ string, _ *protocol.AgentInfoPayload) {
	// Windows disk usage collection requires syscall.GetDiskFreeSpaceEx
	// which will be implemented when Windows agent support is added.
}
