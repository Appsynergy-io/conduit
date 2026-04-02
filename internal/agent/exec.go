package agent

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
	"time"

	"github.com/appsynergy-io/conduit/internal/protocol"
)

const (
	// maxExecOutput is the per-field (stdout/stderr) output limit (1MB).
	maxExecOutput = 1 << 20

	// defaultExecTimeout is the default timeout for exec commands.
	defaultExecTimeout = 300 * time.Second
)

// startExec handles EXEC_START frames from the server.
// It runs the command, streams output via EXEC_DATA, and sends EXEC_EXIT on completion.
func (a *Agent) startExec(ctx context.Context, mux protocol.FrameMux, streamID uint32, payload *protocol.ExecStartPayload) {
	go a.runExec(ctx, mux, streamID, payload)
}

// runExec executes a command and sends results back to the server.
func (a *Agent) runExec(ctx context.Context, mux protocol.FrameMux, streamID uint32, payload *protocol.ExecStartPayload) {
	timeout := defaultExecTimeout
	if payload.Timeout > 0 {
		timeout = time.Duration(payload.Timeout) * time.Second
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Determine shell
	shell, flag := execShell()

	cmd := exec.CommandContext(execCtx, shell, flag, payload.Command)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{buf: &stdout, limit: maxExecOutput}
	cmd.Stderr = &limitedWriter{buf: &stderr, limit: maxExecOutput}

	a.logger.Info("exec start", "job_id", payload.JobID, "command", payload.Command)

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if execCtx.Err() == context.DeadlineExceeded {
			exitCode = -1 // timeout
		} else {
			exitCode = -1
		}
	}

	// Send combined output as EXEC_DATA
	output := stdout.Bytes()
	if stderr.Len() > 0 {
		output = append(output, stderr.Bytes()...)
	}
	if len(output) > 0 {
		dataFrame := &protocol.Frame{
			Type:     protocol.FrameExecData,
			StreamID: streamID,
			Payload:  output,
		}
		if sendErr := mux.Send(ctx, dataFrame); sendErr != nil {
			a.logger.Debug("failed to send EXEC_DATA", "error", sendErr)
		}
	}

	// Send EXEC_EXIT
	exitPayload := protocol.ExecExitPayload{
		JobID:    payload.JobID,
		ExitCode: exitCode,
	}
	exitFrame, err := protocol.NewFrame(protocol.FrameExecExit, streamID, exitPayload)
	if err != nil {
		a.logger.Error("failed to create EXEC_EXIT frame", "error", err)
		return
	}
	if sendErr := mux.Send(ctx, exitFrame); sendErr != nil {
		a.logger.Debug("failed to send EXEC_EXIT", "error", sendErr)
	}

	a.logger.Info("exec complete",
		"job_id", payload.JobID,
		"exit_code", exitCode,
		"duration", duration.String(),
	)
}

// execShell returns the shell and flag for exec commands on the current platform.
func execShell() (string, string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", "/C"
	}
	return "/bin/sh", "-c"
}

// limitedWriter wraps a bytes.Buffer and stops writing after the limit.
type limitedWriter struct {
	buf   *bytes.Buffer
	limit int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	remaining := w.limit - w.buf.Len()
	if remaining <= 0 {
		return len(p), nil // discard but report success
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	return w.buf.Write(p)
}
