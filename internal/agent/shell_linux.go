//go:build linux || darwin

package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"sync"

	"github.com/creack/pty"

	"github.com/appsynergy-io/conduit/internal/protocol"
)

// shellSession represents an active PTY shell session on the agent.
type shellSession struct {
	streamID  uint32
	sessionID string
	ptmx      *os.File
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	once      sync.Once
}

// Close terminates the shell session, killing the process and closing the PTY.
func (s *shellSession) Close() {
	s.once.Do(func() {
		s.cancel()
		if s.cmd != nil && s.cmd.Process != nil {
			s.cmd.Process.Kill()
		}
		if s.ptmx != nil {
			s.ptmx.Close()
		}
	})
}

// startShell launches a new PTY shell session and wires it to the CWP stream.
func (a *Agent) startShell(ctx context.Context, mux protocol.FrameMux, streamID uint32, payload *protocol.ShellStartPayload) {
	shell := defaultShell()
	if payload.Shell != "" {
		shell = payload.Shell
	}

	shellCtx, shellCancel := context.WithCancel(ctx)

	cmd := exec.CommandContext(shellCtx, shell)
	cmd.Env = buildShellEnv()

	// Start PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		a.logger.Error("failed to start PTY", "error", err, "shell", shell)
		shellCancel()
		a.sendShellExit(ctx, mux, streamID, payload.SessionID, -1)
		return
	}

	// Set initial terminal size
	if payload.Cols > 0 && payload.Rows > 0 {
		pty.Setsize(ptmx, &pty.Winsize{
			Cols: uint16(payload.Cols),
			Rows: uint16(payload.Rows),
		})
	}

	sess := &shellSession{
		streamID:  streamID,
		sessionID: payload.SessionID,
		ptmx:      ptmx,
		cmd:       cmd,
		cancel:    shellCancel,
	}

	a.shellMu.Lock()
	a.shells[streamID] = sess
	a.shellMu.Unlock()

	a.logger.Info("shell session started",
		"session_id", payload.SessionID,
		"stream_id", streamID,
		"shell", shell,
	)

	// Register the stream so mux routes SHELL_DATA frames to us
	streamCh := mux.OpenStream(streamID)

	// PTY → Server: read PTY output and send as SHELL_DATA
	go func() {
		defer func() {
			a.shellMu.Lock()
			delete(a.shells, streamID)
			a.shellMu.Unlock()
			mux.CloseStream(streamID)
			sess.Close()

			// Wait for exit code
			exitCode := 0
			if err := cmd.Wait(); err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				} else {
					exitCode = -1
				}
			}

			a.sendShellExit(ctx, mux, streamID, payload.SessionID, exitCode)
			a.logger.Info("shell session ended",
				"session_id", payload.SessionID,
				"exit_code", exitCode,
			)
		}()

		buf := make([]byte, 32*1024) // 32KB read buffer
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				f := &protocol.Frame{
					Type:     protocol.FrameShellData,
					StreamID: streamID,
					Payload:  make([]byte, n),
				}
				copy(f.Payload, buf[:n])
				if sendErr := mux.Send(ctx, f); sendErr != nil {
					a.logger.Debug("failed to send shell data", "error", sendErr)
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					a.logger.Debug("PTY read error", "error", err)
				}
				return
			}
		}
	}()

	// Server → PTY: read SHELL_DATA frames from stream and write to PTY
	go func() {
		for {
			select {
			case f, ok := <-streamCh:
				if !ok {
					return
				}
				switch f.Type {
				case protocol.FrameShellData:
					if _, err := ptmx.Write(f.Payload); err != nil {
						a.logger.Debug("PTY write error", "error", err)
						return
					}
				case protocol.FrameShellResize:
					var resize protocol.ShellResizePayload
					if err := protocol.UnmarshalPayload(f.Payload, &resize); err == nil {
						pty.Setsize(ptmx, &pty.Winsize{
							Cols: uint16(resize.Cols),
							Rows: uint16(resize.Rows),
						})
					}
				}
			case <-shellCtx.Done():
				return
			}
		}
	}()
}

// resizeShell resizes the PTY for an active shell session.
func (a *Agent) resizeShell(streamID uint32, payload *protocol.ShellResizePayload) {
	a.shellMu.Lock()
	sess, ok := a.shells[streamID]
	a.shellMu.Unlock()

	if !ok {
		a.logger.Debug("resize for unknown shell", "stream_id", streamID)
		return
	}

	if payload.Cols > 0 && payload.Rows > 0 {
		pty.Setsize(sess.ptmx, &pty.Winsize{
			Cols: uint16(payload.Cols),
			Rows: uint16(payload.Rows),
		})
	}
}

// sendShellExit sends a SHELL_EXIT frame to the server.
func (a *Agent) sendShellExit(ctx context.Context, mux protocol.FrameMux, streamID uint32, sessionID string, exitCode int) {
	payload := protocol.ShellExitPayload{
		SessionID: sessionID,
		ExitCode:  exitCode,
	}
	f, err := protocol.NewFrame(protocol.FrameShellExit, streamID, payload)
	if err != nil {
		a.logger.Debug("failed to create SHELL_EXIT frame", "error", err)
		return
	}
	if err := mux.Send(ctx, f); err != nil {
		a.logger.Debug("failed to send SHELL_EXIT", "error", err)
	}
}

// defaultShell returns the user's default shell or /bin/sh as fallback.
func defaultShell() string {
	// Try user's configured shell
	if u, err := user.Current(); err == nil && u.HomeDir != "" {
		// On Linux/macOS, check passwd for shell
		shell := lookupShell(u.Username)
		if shell != "" {
			return shell
		}
	}

	// Try SHELL environment variable
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}

	return "/bin/sh"
}

// lookupShell finds the user's configured shell from /etc/passwd.
func lookupShell(username string) string {
	u, err := user.Lookup(username)
	if err != nil {
		return ""
	}

	// user.User doesn't expose shell directly on all platforms.
	// Fall back to reading /etc/passwd.
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return ""
	}

	prefix := fmt.Sprintf("%s:", u.Username)
	for _, line := range splitLines(data) {
		if len(line) > 0 && hasPrefix(line, prefix) {
			fields := splitColon(line)
			if len(fields) >= 7 {
				return fields[6]
			}
		}
	}
	return ""
}

// splitLines splits byte data into lines without allocating strings.
func splitLines(data []byte) []string {
	var lines []string
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, string(data[start:i]))
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func splitColon(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// buildShellEnv constructs the environment for the shell process.
func buildShellEnv() []string {
	env := os.Environ()
	// Set TERM if not already present
	hasTerm := false
	for _, e := range env {
		if hasPrefix(e, "TERM=") {
			hasTerm = true
			break
		}
	}
	if !hasTerm {
		env = append(env, "TERM=xterm-256color")
	}
	return env
}
