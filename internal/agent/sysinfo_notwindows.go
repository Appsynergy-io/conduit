//go:build !windows

package agent

import "github.com/appsynergy-io/conduit/internal/protocol"

// collectWindowsInfo is a no-op stub on non-Windows platforms.
// The runtime.GOOS switch in sysinfo.go never calls this, but the
// compiler requires it to exist.
func collectWindowsInfo(_ *protocol.AgentInfoPayload) {}
