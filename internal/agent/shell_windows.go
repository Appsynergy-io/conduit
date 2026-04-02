//go:build windows

package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/appsynergy-io/conduit/internal/protocol"
)

// shellSession represents an active shell session on the agent (Windows stub).
type shellSession struct {
	streamID  uint32
	sessionID string
	cancel    context.CancelFunc
	once      sync.Once
}

// Close terminates the shell session.
func (s *shellSession) Close() {
	s.once.Do(func() {
		s.cancel()
	})
}

// startShell is not yet implemented on Windows (ConPTY support planned).
func (a *Agent) startShell(ctx context.Context, mux protocol.FrameMux, streamID uint32, payload *protocol.ShellStartPayload) {
	a.logger.Warn("shell sessions not yet supported on Windows")
	a.sendShellExit(ctx, mux, streamID, payload.SessionID, -1)
}

// resizeShell is not yet implemented on Windows.
func (a *Agent) resizeShell(_ uint32, _ *protocol.ShellResizePayload) {}

// sendShellExit sends a SHELL_EXIT frame to the server.
func (a *Agent) sendShellExit(ctx context.Context, mux protocol.FrameMux, streamID uint32, sessionID string, exitCode int) {
	payload := protocol.ShellExitPayload{
		SessionID: sessionID,
		ExitCode:  exitCode,
	}
	f, err := protocol.NewFrame(protocol.FrameShellExit, streamID, payload)
	if err != nil {
		return
	}
	_ = mux.Send(ctx, f)
}

// defaultShell returns the default shell on Windows.
func defaultShell() string {
	return fmt.Sprintf("%s\\System32\\cmd.exe", "C:\\Windows")
}
