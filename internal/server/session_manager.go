package server

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/protocol"
)

// LiveSession represents a shell session whose PTY is alive on the agent,
// independent of any browser connection. The session persists through browser
// disconnects and can be resumed from any platform.
type LiveSession struct {
	ID          string
	AgentID     string
	UserID      string
	TenantID    string
	StreamID    uint32
	Pinned      bool
	IdleTimeout time.Duration
	Ring        *RingBuffer
	Recorder    *asciicastRecorder

	mu          sync.Mutex
	browserConn *websocket.Conn // nil when detached
	browserDone chan struct{}    // closed when browser bridge exits
	detachedAt  *time.Time
	agentCh     <-chan *protocol.Frame
	connAgent   *ConnectedAgent
	cancel      context.CancelFunc // cancels the agent→ring goroutine on terminate
	exitDone    chan struct{}       // closed when onSessionExit completes
}

// IsDetached returns true if no browser is attached.
func (ls *LiveSession) IsDetached() bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.browserConn == nil
}

// DetachedSince returns how long the session has been detached, or 0 if attached.
func (ls *LiveSession) DetachedSince() time.Duration {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if ls.detachedAt == nil {
		return 0
	}
	return time.Since(*ls.detachedAt)
}

// SessionManager owns the lifecycle of all live shell sessions,
// decoupled from browser connections. It handles idle reaping,
// output buffering, and browser attach/detach.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*LiveSession // sessionID → LiveSession
	db       *db.DB
	logger   *slog.Logger
	eventBus *EventBus
	cancel   context.CancelFunc
}

// NewSessionManager creates a SessionManager and starts the idle reaper.
// Closes any orphaned sessions in the DB from previous server runs.
func NewSessionManager(database *db.DB, logger *slog.Logger, eventBus *EventBus) *SessionManager {
	ctx, cancel := context.WithCancel(context.Background())
	sm := &SessionManager{
		sessions: make(map[string]*LiveSession),
		db:       database,
		logger:   logger,
		eventBus: eventBus,
		cancel:   cancel,
	}

	// Clean up sessions left over from a previous server run.
	if n, err := database.CloseOrphanedShellSessions(context.Background()); err != nil {
		logger.Error("failed to close orphaned sessions", "error", err)
	} else if n > 0 {
		logger.Info("closed orphaned sessions from previous run", "count", n)
	}

	go sm.idleReaper(ctx)
	return sm
}

// Stop shuts down the session manager and idle reaper.
func (sm *SessionManager) Stop() {
	sm.cancel()
}

// Get returns a live session by ID, or nil if not found.
func (sm *SessionManager) Get(sessionID string) *LiveSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessions[sessionID]
}

// List returns all live sessions.
func (sm *SessionManager) List() []*LiveSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	out := make([]*LiveSession, 0, len(sm.sessions))
	for _, ls := range sm.sessions {
		out = append(out, ls)
	}
	return out
}

// CreateSession allocates a CWP stream on the agent, sends SHELL_START,
// creates the DB record, and starts the agent→ring goroutine.
// The session starts without a browser attached — call AttachBrowser next.
func (sm *SessionManager) CreateSession(
	ctx context.Context,
	connAgent *ConnectedAgent,
	agent *db.Agent,
	userID, tenantID string,
	cols, rows int,
	pinned bool,
	idleTimeout int,
	recording bool,
) (*LiveSession, error) {
	sessionID := uuid.NewString()

	// Allocate CWP stream
	streamID := connAgent.Mux.NextStreamID()
	agentCh := connAgent.Mux.OpenStream(streamID)

	// Create DB record
	pinnedInt := 0
	if pinned {
		pinnedInt = 1
	}
	rec := 0
	if recording {
		rec = 1
	}
	shellSession := &db.ShellSession{
		ID:          sessionID,
		TenantID:    tenantID,
		AgentID:     connAgent.AgentID,
		UserID:      userID,
		Status:      "active",
		Cols:        &cols,
		Rows:        &rows,
		Recording:   rec,
		Pinned:      pinnedInt,
		IdleTimeout: idleTimeout,
	}
	if err := sm.db.CreateShellSession(ctx, shellSession); err != nil {
		connAgent.Mux.CloseStream(streamID)
		return nil, fmt.Errorf("creating shell session: %w", err)
	}

	// Send SHELL_START to agent
	startPayload := protocol.ShellStartPayload{
		SessionID: sessionID,
		Cols:      cols,
		Rows:      rows,
	}
	startFrame, err := protocol.NewFrame(protocol.FrameShellStart, streamID, startPayload)
	if err != nil {
		connAgent.Mux.CloseStream(streamID)
		return nil, fmt.Errorf("creating SHELL_START frame: %w", err)
	}
	if err := connAgent.Mux.Send(ctx, startFrame); err != nil {
		connAgent.Mux.CloseStream(streamID)
		return nil, fmt.Errorf("sending SHELL_START: %w", err)
	}

	// Build LiveSession
	sessionCtx, sessionCancel := context.WithCancel(context.Background())
	var recorder *asciicastRecorder
	if recording {
		recorder = newAsciicastRecorder(cols, rows)
	}

	ls := &LiveSession{
		ID:          sessionID,
		AgentID:     connAgent.AgentID,
		UserID:      userID,
		TenantID:    tenantID,
		StreamID:    streamID,
		Pinned:      pinned,
		IdleTimeout: time.Duration(idleTimeout) * time.Second,
		Ring:        NewRingBuffer(DefaultRingSize),
		Recorder:    recorder,
		agentCh:     agentCh,
		connAgent:   connAgent,
		cancel:      sessionCancel,
		exitDone:    make(chan struct{}),
	}

	sm.mu.Lock()
	sm.sessions[sessionID] = ls
	sm.mu.Unlock()

	// Start agent → ring/recorder/browser goroutine
	go sm.agentReadLoop(sessionCtx, ls)

	return ls, nil
}

// agentReadLoop reads frames from the agent CWP stream and routes output
// to the ring buffer, recorder, and browser (if attached).
// Runs until the session is terminated or the agent stream closes.
func (sm *SessionManager) agentReadLoop(ctx context.Context, ls *LiveSession) {
	defer sm.onSessionExit(ls)

	for {
		select {
		case f, ok := <-ls.agentCh:
			if !ok {
				return // stream closed (agent disconnected or session terminated)
			}
			switch f.Type {
			case protocol.FrameShellData:
				// Always buffer + record, regardless of browser attachment
				ls.Ring.Write(f.Payload)
				if ls.Recorder != nil {
					ls.Recorder.WriteOutput(f.Payload)
				}

				// Forward to browser if attached
				ls.mu.Lock()
				conn := ls.browserConn
				ls.mu.Unlock()
				if conn != nil {
					writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
					if err := conn.Write(writeCtx, websocket.MessageBinary, f.Payload); err != nil {
						// Browser write failed — will be detected by browser read loop
						sm.logger.Debug("browser write failed", "session_id", ls.ID, "error", err)
					}
					writeCancel()
				}

			case protocol.FrameShellExit:
				sm.logger.Info("shell exited on agent", "session_id", ls.ID)
				return
			}

		case <-ctx.Done():
			return
		}
	}
}

// onSessionExit is called when the agent stream closes (PTY exited).
// Saves the recording, closes the DB session, publishes events.
func (sm *SessionManager) onSessionExit(ls *LiveSession) {
	defer close(ls.exitDone)

	ctx := context.Background()

	// Close the browser connection if still attached
	ls.mu.Lock()
	if ls.browserConn != nil {
		ls.browserConn.Close(websocket.StatusNormalClosure, "session ended")
		ls.browserConn = nil
	}
	ls.mu.Unlock()

	// Save recording
	if ls.Recorder != nil && ls.Recorder.buf.Len() > 0 {
		rec := &db.ShellRecording{
			ID:        uuid.NewString(),
			TenantID:  ls.TenantID,
			SessionID: ls.ID,
			AgentID:   ls.AgentID,
			UserID:    ls.UserID,
			Duration:  ls.Recorder.Duration(),
			SizeBytes: ls.Recorder.buf.Len(),
			Format:    "asciicast-v2",
			Data:      ls.Recorder.Bytes(),
		}
		if err := sm.db.CreateShellRecording(ctx, rec); err != nil {
			sm.logger.Error("failed to save recording", "session_id", ls.ID, "error", err)
		}
	}

	// Update DB
	if err := sm.db.CloseShellSession(ctx, ls.ID); err != nil {
		sm.logger.Error("failed to close shell session in DB", "session_id", ls.ID, "error", err)
	}

	// Remove from manager
	sm.mu.Lock()
	delete(sm.sessions, ls.ID)
	sm.mu.Unlock()

	sm.logger.Info("session closed", "session_id", ls.ID, "agent_id", ls.AgentID)

	sm.eventBus.Publish(ls.TenantID, Event{
		Channel: "shell",
		Type:    "shell.end",
		Data: map[string]string{
			"sessionId": ls.ID,
			"agentId":   ls.AgentID,
		},
	})
}

// AttachBrowser connects a browser WebSocket to a live session.
// Replays the ring buffer, then bridges live I/O.
// Returns when the browser disconnects or the session ends.
func (sm *SessionManager) AttachBrowser(ctx context.Context, ls *LiveSession, browserConn *websocket.Conn) error {
	ls.mu.Lock()
	if ls.browserConn != nil {
		ls.mu.Unlock()
		return fmt.Errorf("session already has an active browser attachment")
	}
	ls.browserConn = browserConn
	ls.detachedAt = nil
	ls.browserDone = make(chan struct{})
	ls.mu.Unlock()

	// Update DB status
	if err := sm.db.UpdateShellSessionStatus(ctx, ls.ID, "active"); err != nil {
		sm.logger.Error("failed to update session status in DB", "session_id", ls.ID, "error", err)
	}

	// Replay ring buffer contents to bring browser up to date
	replay := ls.Ring.Bytes()
	if len(replay) > 0 {
		writeCtx, writeCancel := context.WithTimeout(ctx, 10*time.Second)
		if err := browserConn.Write(writeCtx, websocket.MessageBinary, replay); err != nil {
			writeCancel()
			sm.detachBrowser(ctx, ls)
			return fmt.Errorf("replaying ring buffer: %w", err)
		}
		writeCancel()
	}

	sm.logger.Info("browser attached", "session_id", ls.ID)

	sm.eventBus.Publish(ls.TenantID, Event{
		Channel: "shell",
		Type:    "shell.session.attached",
		Data: map[string]string{
			"sessionId": ls.ID,
			"agentId":   ls.AgentID,
			"userId":    ls.UserID,
		},
	})

	// Browser → Agent: read from browser, send as SHELL_DATA/SHELL_RESIZE
	go func() {
		defer func() {
			ls.mu.Lock()
			if ls.browserDone != nil {
				select {
				case <-ls.browserDone:
				default:
					close(ls.browserDone)
				}
			}
			ls.mu.Unlock()
		}()
		for {
			_, data, err := browserConn.Read(ctx)
			if err != nil {
				return
			}

			// Check for resize command
			if f := parseBrowserResize(data, ls.StreamID); f != nil {
				if err := ls.connAgent.Mux.Send(ctx, f); err != nil {
					return
				}
				continue
			}

			// Regular terminal data
			f := &protocol.Frame{
				Type:     protocol.FrameShellData,
				StreamID: ls.StreamID,
				Payload:  data,
			}
			if err := ls.connAgent.Mux.Send(ctx, f); err != nil {
				return
			}
		}
	}()

	// Wait for browser disconnect or session end
	ls.mu.Lock()
	done := ls.browserDone
	ls.mu.Unlock()

	select {
	case <-done:
	case <-ctx.Done():
	}

	// Browser disconnected — detach (session stays alive)
	sm.detachBrowser(ctx, ls)
	return nil
}

// detachBrowser disconnects the browser from the session without killing the PTY.
func (sm *SessionManager) detachBrowser(ctx context.Context, ls *LiveSession) {
	ls.mu.Lock()
	if ls.browserConn == nil {
		ls.mu.Unlock()
		return
	}
	ls.browserConn = nil
	now := time.Now()
	ls.detachedAt = &now
	ls.mu.Unlock()

	if err := sm.db.DetachShellSession(ctx, ls.ID); err != nil {
		sm.logger.Error("failed to detach shell session in DB", "session_id", ls.ID, "error", err)
	}

	sm.logger.Info("browser detached", "session_id", ls.ID, "pinned", ls.Pinned)

	sm.eventBus.Publish(ls.TenantID, Event{
		Channel: "shell",
		Type:    "shell.session.detached",
		Data: map[string]interface{}{
			"sessionId": ls.ID,
			"agentId":   ls.AgentID,
			"userId":    ls.UserID,
			"pinned":    ls.Pinned,
		},
	})
}

// TerminateSession kills a session: closes the CWP stream, saves recording,
// updates DB. Blocks until cleanup completes or 5s timeout.
func (sm *SessionManager) TerminateSession(ctx context.Context, ls *LiveSession) {
	ls.cancel() // stops agentReadLoop → triggers onSessionExit
	ls.connAgent.Mux.CloseStream(ls.StreamID)

	// Wait for onSessionExit to finish so DB is updated before caller returns.
	select {
	case <-ls.exitDone:
	case <-time.After(5 * time.Second):
		sm.logger.Warn("session exit timed out", "session_id", ls.ID)
	}
}

// UpdateSession modifies session settings (pin, idle timeout).
func (sm *SessionManager) UpdateSession(ctx context.Context, ls *LiveSession, pinned *bool, idleTimeout *int) error {
	ls.mu.Lock()
	if pinned != nil {
		ls.Pinned = *pinned
	}
	if idleTimeout != nil {
		ls.IdleTimeout = time.Duration(*idleTimeout) * time.Second
	}
	ls.mu.Unlock()

	pinnedInt := 0
	if ls.Pinned {
		pinnedInt = 1
	}
	return sm.db.UpdateShellSessionSettings(ctx, ls.ID, pinnedInt, int(ls.IdleTimeout.Seconds()))
}

// idleReaper runs every 30 seconds, closing detached non-pinned sessions
// whose idle timeout has expired. NIST AC-12 compliance.
func (sm *SessionManager) idleReaper(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sm.reapIdleSessions(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (sm *SessionManager) reapIdleSessions(ctx context.Context) {
	sm.mu.RLock()
	var toReap []*LiveSession
	for _, ls := range sm.sessions {
		ls.mu.Lock()
		detached := ls.detachedAt != nil
		pinned := ls.Pinned
		idleTimeout := ls.IdleTimeout
		var detachedDuration time.Duration
		if ls.detachedAt != nil {
			detachedDuration = time.Since(*ls.detachedAt)
		}
		ls.mu.Unlock()

		if detached && !pinned && detachedDuration > idleTimeout {
			toReap = append(toReap, ls)
		}
	}
	sm.mu.RUnlock()

	for _, ls := range toReap {
		// Recheck pinned under lock — user may have pinned between scan and reap.
		ls.mu.Lock()
		nowPinned := ls.Pinned
		ls.mu.Unlock()
		if nowPinned {
			continue
		}

		sm.logger.Info("reaping idle session",
			"session_id", ls.ID,
			"agent_id", ls.AgentID,
			"detached_duration", ls.DetachedSince().String(),
		)

		sm.db.InsertAuditLog(ctx, &db.AuditEntry{
			ID:        uuid.NewString(),
			TenantID:  ls.TenantID,
			EventType: "shell.session.idle_timeout",
			UserID:    &ls.UserID,
			AgentID:   &ls.AgentID,
			Details:   strPtr(fmt.Sprintf(`{"session_id":"%s","detached_seconds":%d}`, ls.ID, int(ls.DetachedSince().Seconds()))),
			Outcome:   "success",
		})

		sm.eventBus.Publish(ls.TenantID, Event{
			Channel: "shell",
			Type:    "shell.session.idle_timeout",
			Data: map[string]interface{}{
				"sessionId":        ls.ID,
				"agentId":          ls.AgentID,
				"userId":           ls.UserID,
				"detachedDuration": int(ls.DetachedSince().Seconds()),
			},
		})

		sm.TerminateSession(ctx, ls)
	}
}

// CleanupAgentSessions terminates all live sessions for an agent that disconnected.
func (sm *SessionManager) CleanupAgentSessions(ctx context.Context, agentID string) {
	sm.mu.RLock()
	var toClean []*LiveSession
	for _, ls := range sm.sessions {
		if ls.AgentID == agentID {
			toClean = append(toClean, ls)
		}
	}
	sm.mu.RUnlock()

	for _, ls := range toClean {
		sm.logger.Info("cleaning up session for disconnected agent",
			"session_id", ls.ID,
			"agent_id", agentID,
		)
		sm.TerminateSession(ctx, ls)
	}
}
