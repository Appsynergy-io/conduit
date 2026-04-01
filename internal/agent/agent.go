package agent

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"sync"
	"time"

	"github.com/appsynergy-io/conduit/internal/protocol"
)

const (
	// Reconnect backoff parameters
	backoffMin    = 1 * time.Second
	backoffMax    = 30 * time.Second
	backoffFactor = 2.0
	backoffJitter = 0.3 // +/- 30%

	// Heartbeat interval
	pingInterval = 15 * time.Second

	// Agent info reporting interval
	infoInterval = 60 * time.Second
)

// Agent is the client-side daemon that connects to a Conduit server,
// authenticates via CWP, and handles shell/exec/file operations.
type Agent struct {
	cfg    *Config
	logger *slog.Logger

	mux    *protocol.Mux
	muxMu  sync.RWMutex
	shells map[uint32]*shellSession // streamID → shell session
	shellMu sync.Mutex
}

// New creates a new Agent with the given config.
func New(cfg *Config, logger *slog.Logger) *Agent {
	return &Agent{
		cfg:    cfg,
		logger: logger,
		shells: make(map[uint32]*shellSession),
	}
}

// Run connects to the server and enters the main agent loop.
// It reconnects automatically on disconnection with exponential backoff.
// Blocks until ctx is canceled.
func (a *Agent) Run(ctx context.Context) error {
	backoff := backoffMin

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		a.logger.Info("connecting to server", "url", a.cfg.ServerURL)

		err := a.connectAndRun(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			a.logger.Warn("disconnected from server", "error", err)
		}

		// Clean up any active shell sessions
		a.cleanupShells()

		// Exponential backoff with jitter
		jittered := addJitter(backoff, backoffJitter)
		a.logger.Info("reconnecting", "backoff", jittered.Round(time.Millisecond))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(jittered):
		}

		backoff = time.Duration(float64(backoff) * backoffFactor)
		if backoff > backoffMax {
			backoff = backoffMax
		}
	}
}

// connectAndRun establishes a connection, performs the handshake, and runs
// the main loop. Returns when the connection is lost or ctx is canceled.
func (a *Agent) connectAndRun(ctx context.Context) error {
	mux, err := a.connect(ctx)
	if err != nil {
		return fmt.Errorf("connecting: %w", err)
	}
	defer mux.Close()

	a.muxMu.Lock()
	a.mux = mux
	a.muxMu.Unlock()

	defer func() {
		a.muxMu.Lock()
		a.mux = nil
		a.muxMu.Unlock()
	}()

	a.logger.Info("connected to server", "agent_id", a.cfg.AgentID)

	// Start read loop
	readErr := make(chan error, 1)
	go func() {
		readErr <- mux.ReadLoop(ctx)
	}()

	// Main loop: handle frames, send pings and agent info
	return a.runLoop(ctx, mux, readErr)
}

// runLoop is the main agent event loop. It handles incoming frames,
// sends periodic pings, and reports system info.
func (a *Agent) runLoop(ctx context.Context, mux *protocol.Mux, readErr <-chan error) error {
	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()

	infoTicker := time.NewTicker(infoInterval)
	defer infoTicker.Stop()

	// Send initial agent info
	a.sendAgentInfo(ctx, mux)

	for {
		select {
		case f, ok := <-mux.Global():
			if !ok {
				return fmt.Errorf("global channel closed")
			}
			a.handleFrame(ctx, mux, f)

		case <-pingTicker.C:
			pong := &protocol.Frame{Type: protocol.FramePong}
			// We send PONG proactively as keepalive; server sends PINGs
			// Actually, respond to server PINGs inline in handleFrame.
			// Here we just do keepalive by sending our own PING.
			ping := &protocol.Frame{Type: protocol.FramePing}
			_ = pong // unused, PINGs are sent proactively
			if err := mux.Send(ctx, ping); err != nil {
				return fmt.Errorf("sending ping: %w", err)
			}

		case <-infoTicker.C:
			a.sendAgentInfo(ctx, mux)

		case err := <-readErr:
			return fmt.Errorf("read loop: %w", err)

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// handleFrame processes a single frame from the server.
func (a *Agent) handleFrame(ctx context.Context, mux *protocol.Mux, f *protocol.Frame) {
	switch f.Type {
	case protocol.FramePing:
		// Respond with PONG
		pong := &protocol.Frame{Type: protocol.FramePong, StreamID: f.StreamID}
		if err := mux.Send(ctx, pong); err != nil {
			a.logger.Debug("failed to send PONG", "error", err)
		}

	case protocol.FrameShellStart:
		var payload protocol.ShellStartPayload
		if err := protocol.UnmarshalPayload(f.Payload, &payload); err != nil {
			a.logger.Warn("invalid SHELL_START payload", "error", err)
			return
		}
		a.startShell(ctx, mux, f.StreamID, &payload)

	case protocol.FrameShellResize:
		var payload protocol.ShellResizePayload
		if err := protocol.UnmarshalPayload(f.Payload, &payload); err != nil {
			a.logger.Warn("invalid SHELL_RESIZE payload", "error", err)
			return
		}
		a.resizeShell(f.StreamID, &payload)

	case protocol.FrameExecStart:
		var payload protocol.ExecStartPayload
		if err := protocol.UnmarshalPayload(f.Payload, &payload); err != nil {
			a.logger.Warn("invalid EXEC_START payload", "error", err)
			return
		}
		a.startExec(ctx, mux, f.StreamID, &payload)

	case protocol.FrameFileList, protocol.FrameFileRead, protocol.FrameFileWrite,
		protocol.FrameFileStat, protocol.FrameFileDelete, protocol.FrameFileRename, protocol.FrameFileMkdir:
		a.handleFileFrame(ctx, mux, f)

	default:
		a.logger.Debug("unhandled frame",
			"type", f.Type.String(),
			"stream_id", f.StreamID,
		)
	}
}

// sendAgentInfo collects system metrics and sends an AGENT_INFO frame.
func (a *Agent) sendAgentInfo(ctx context.Context, mux *protocol.Mux) {
	info := collectSysInfo()

	f, err := protocol.NewFrame(protocol.FrameAgentInfo, 0, info)
	if err != nil {
		a.logger.Debug("failed to create AGENT_INFO frame", "error", err)
		return
	}
	if err := mux.Send(ctx, f); err != nil {
		a.logger.Debug("failed to send AGENT_INFO", "error", err)
	}
}

// cleanupShells closes all active shell sessions.
func (a *Agent) cleanupShells() {
	a.shellMu.Lock()
	defer a.shellMu.Unlock()

	for id, sess := range a.shells {
		sess.Close()
		delete(a.shells, id)
	}
}

// Hostname returns the system hostname for HELLO payload.
func Hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}

// addJitter adds random jitter to a duration.
// jitterFrac is the fraction of the duration to vary (e.g., 0.3 = +/- 30%).
func addJitter(d time.Duration, jitterFrac float64) time.Duration {
	if jitterFrac <= 0 {
		return d
	}
	jitterRange := int64(float64(d) * jitterFrac * 2)
	if jitterRange <= 0 {
		return d
	}
	n, err := rand.Int(rand.Reader, big.NewInt(jitterRange))
	if err != nil {
		return d
	}
	offset := n.Int64() - jitterRange/2
	result := d + time.Duration(offset)
	if result < 0 {
		return d
	}
	return result
}
