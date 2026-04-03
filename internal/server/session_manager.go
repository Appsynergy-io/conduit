package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/protocol"
)

// Session client roles.
const (
	roleController = "controller"
	roleWatcher    = "watcher"
)

// SessionClient represents a single browser/CLI WebSocket connection to a session.
type SessionClient struct {
	ID       string          // unique client connection ID (UUID)
	UserID   string          // authenticated user
	Conn     *websocket.Conn // WebSocket connection
	Role     string          // "controller" or "watcher"
	JoinedAt time.Time
	Done     chan struct{} // closed when the client's read loop exits
}

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

	mu         sync.Mutex
	clients    map[string]*SessionClient // clientID → SessionClient
	controller *SessionClient            // the one client with write access (nil when detached)
	detachedAt *time.Time
	agentCh    <-chan *protocol.Frame
	connAgent  *ConnectedAgent
	cancel     context.CancelFunc // cancels the agent→ring goroutine on terminate
	exitDone   chan struct{}       // closed when onSessionExit completes
}

// IsDetached returns true if no clients are connected.
func (ls *LiveSession) IsDetached() bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return len(ls.clients) == 0
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

// ClientCount returns the number of connected clients.
func (ls *LiveSession) ClientCount() int {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return len(ls.clients)
}

// ActiveClients returns a snapshot of connected clients for API responses.
func (ls *LiveSession) ActiveClients() []SessionClientInfo {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	out := make([]SessionClientInfo, 0, len(ls.clients))
	for _, c := range ls.clients {
		out = append(out, SessionClientInfo{
			UserID:      c.UserID,
			Role:        c.Role,
			ConnectedAt: c.JoinedAt.UTC().Format(time.RFC3339),
		})
	}
	return out
}

// SessionClientInfo is a snapshot of a connected client for API responses.
type SessionClientInfo struct {
	UserID      string `json:"userId"`
	UserEmail   string `json:"userEmail,omitempty"`
	Role        string `json:"role"`
	ConnectedAt string `json:"connectedAt"`
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
// The session starts without a client attached — call AttachClient next.
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
		clients:     make(map[string]*SessionClient),
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
// to the ring buffer, recorder, and all connected clients.
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
				// Always buffer + record, regardless of client attachment
				ls.Ring.Write(f.Payload)
				if ls.Recorder != nil {
					ls.Recorder.WriteOutput(f.Payload)
				}

				// Broadcast to all connected clients
				ls.mu.Lock()
				clients := make([]*SessionClient, 0, len(ls.clients))
				for _, c := range ls.clients {
					clients = append(clients, c)
				}
				ls.mu.Unlock()

				for _, c := range clients {
					writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
					if err := c.Conn.Write(writeCtx, websocket.MessageBinary, f.Payload); err != nil {
						sm.logger.Debug("client write failed", "session_id", ls.ID, "client_id", c.ID, "error", err)
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

	// Send exit message and close all client connections
	exitMsg, _ := json.Marshal(map[string]interface{}{
		"type": "exit",
		"code": 0,
	})
	ls.mu.Lock()
	for _, c := range ls.clients {
		writeCtx, writeCancel := context.WithTimeout(ctx, 2*time.Second)
		_ = c.Conn.Write(writeCtx, websocket.MessageText, exitMsg)
		writeCancel()
		c.Conn.Close(websocket.StatusNormalClosure, "session ended")
	}
	ls.clients = make(map[string]*SessionClient)
	ls.controller = nil
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

// AttachClient connects a browser/CLI WebSocket to a live session.
// mode is "controller" or "watcher". Replays ring buffer, then bridges I/O.
// Returns when the client disconnects or the session ends.
func (sm *SessionManager) AttachClient(ctx context.Context, ls *LiveSession, conn *websocket.Conn, userID, mode string) error {
	client := &SessionClient{
		ID:       uuid.NewString(),
		UserID:   userID,
		Conn:     conn,
		Role:     mode,
		JoinedAt: time.Now(),
		Done:     make(chan struct{}),
	}

	ls.mu.Lock()
	// If connecting as controller, demote any existing controller to watcher
	if mode == roleController && ls.controller != nil {
		prevController := ls.controller
		prevController.Role = roleWatcher
		// Notify the demoted controller
		demoteMsg, _ := json.Marshal(map[string]interface{}{
			"type": "control_transferred",
			"to":   userID,
		})
		writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
		_ = prevController.Conn.Write(writeCtx, websocket.MessageText, demoteMsg)
		writeCancel()
	}

	ls.clients[client.ID] = client
	if mode == roleController {
		ls.controller = client
	}
	ls.detachedAt = nil
	watcherCount := sm.countWatchersLocked(ls)
	ls.mu.Unlock()

	// Update DB status
	if err := sm.db.UpdateShellSessionStatus(ctx, ls.ID, "active"); err != nil {
		sm.logger.Error("failed to update session status in DB", "session_id", ls.ID, "error", err)
	}

	// Replay ring buffer contents to bring client up to date
	replay := ls.Ring.Bytes()
	if len(replay) > 0 {
		writeCtx, writeCancel := context.WithTimeout(ctx, 10*time.Second)
		if err := conn.Write(writeCtx, websocket.MessageBinary, replay); err != nil {
			writeCancel()
			sm.detachClient(ctx, ls, client)
			return fmt.Errorf("replaying ring buffer: %w", err)
		}
		writeCancel()
	}

	sm.logger.Info("client attached", "session_id", ls.ID, "client_id", client.ID, "role", mode, "user_id", userID)

	// Notify other clients about the new connection
	if mode == roleWatcher {
		sm.broadcastJSON(ctx, ls, client.ID, map[string]interface{}{
			"type":         "watcher_joined",
			"userId":       userID,
			"watcherCount": watcherCount,
		})
		sm.eventBus.Publish(ls.TenantID, Event{
			Channel: "shell",
			Type:    "shell.session.watcher.joined",
			Data: map[string]string{
				"sessionId": ls.ID,
				"agentId":   ls.AgentID,
				"userId":    userID,
			},
		})
	} else {
		sm.eventBus.Publish(ls.TenantID, Event{
			Channel: "shell",
			Type:    "shell.session.attached",
			Data: map[string]string{
				"sessionId": ls.ID,
				"agentId":   ls.AgentID,
				"userId":    userID,
			},
		})
	}

	// Client → Agent: read from client, forward input if controller
	go func() {
		defer func() {
			select {
			case <-client.Done:
			default:
				close(client.Done)
			}
		}()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}

			// Check for JSON control messages
			if len(data) > 0 && data[0] == '{' {
				if sm.handleClientJSON(ctx, ls, client, data) {
					continue
				}
			}

			// Only forward input from the controller
			ls.mu.Lock()
			isController := ls.controller == client
			ls.mu.Unlock()
			if !isController {
				continue // silently discard watcher input
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

	// Wait for client disconnect or session end
	select {
	case <-client.Done:
	case <-ctx.Done():
	}

	// Client disconnected — detach this client (session stays alive if others remain)
	sm.detachClient(ctx, ls, client)
	return nil
}

// handleClientJSON processes JSON control messages from a connected client.
// Returns true if the message was handled (caller should skip normal processing).
func (sm *SessionManager) handleClientJSON(ctx context.Context, ls *LiveSession, client *SessionClient, data []byte) bool {
	var msg struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return false
	}

	switch msg.Type {
	case "take_control":
		sm.TransferControl(ctx, ls, client)
		return true
	case "resize":
		// Resize is only allowed from the controller
		ls.mu.Lock()
		isController := ls.controller == client
		ls.mu.Unlock()
		if !isController {
			return true // discard resize from watchers
		}
		return false // let normal resize parsing handle it
	}
	return false
}

// TransferControl promotes a client to controller, demoting the current controller.
func (sm *SessionManager) TransferControl(ctx context.Context, ls *LiveSession, newController *SessionClient) {
	ls.mu.Lock()
	prevController := ls.controller

	// Already the controller — nothing to do
	if prevController == newController {
		ls.mu.Unlock()
		return
	}

	// Demote previous controller to watcher
	var prevUserID string
	if prevController != nil {
		prevController.Role = roleWatcher
		prevUserID = prevController.UserID
	}

	// Promote new controller
	newController.Role = roleController
	ls.controller = newController
	watcherCount := sm.countWatchersLocked(ls)
	ls.mu.Unlock()

	// Notify the demoted controller
	if prevController != nil {
		demoteMsg, _ := json.Marshal(map[string]interface{}{
			"type": "control_transferred",
			"to":   newController.UserID,
		})
		writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
		_ = prevController.Conn.Write(writeCtx, websocket.MessageText, demoteMsg)
		writeCancel()
	}

	// Notify the new controller
	grantMsg, _ := json.Marshal(map[string]string{
		"type": "control_granted",
	})
	writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
	_ = newController.Conn.Write(writeCtx, websocket.MessageText, grantMsg)
	writeCancel()

	// Broadcast to all other clients
	sm.broadcastJSON(ctx, ls, "", map[string]interface{}{
		"type":         "controller_changed",
		"userId":       newController.UserID,
		"watcherCount": watcherCount,
	})

	sm.logger.Info("control transferred",
		"session_id", ls.ID,
		"from_user_id", prevUserID,
		"to_user_id", newController.UserID,
	)

	sm.eventBus.Publish(ls.TenantID, Event{
		Channel: "shell",
		Type:    "shell.session.control.transferred",
		Data: map[string]string{
			"sessionId":  ls.ID,
			"agentId":    ls.AgentID,
			"fromUserId": prevUserID,
			"toUserId":   newController.UserID,
		},
	})
}

// detachClient removes a single client from the session.
// If the controller disconnects and watchers remain, auto-promotes the first watcher.
// Sets detachedAt only when all clients are gone.
func (sm *SessionManager) detachClient(ctx context.Context, ls *LiveSession, client *SessionClient) {
	ls.mu.Lock()
	// Already removed (e.g., session exit closed all clients)
	if _, exists := ls.clients[client.ID]; !exists {
		ls.mu.Unlock()
		return
	}

	delete(ls.clients, client.ID)
	wasController := ls.controller == client
	if wasController {
		ls.controller = nil
	}

	// Auto-promote a watcher if the controller left and watchers remain
	var promoted *SessionClient
	if wasController && len(ls.clients) > 0 {
		for _, c := range ls.clients {
			c.Role = roleController
			ls.controller = c
			promoted = c
			break
		}
	}

	allGone := len(ls.clients) == 0
	if allGone {
		now := time.Now()
		ls.detachedAt = &now
	}
	watcherCount := sm.countWatchersLocked(ls)
	ls.mu.Unlock()

	// Notify promoted watcher
	if promoted != nil {
		grantMsg, _ := json.Marshal(map[string]string{
			"type": "control_granted",
		})
		writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
		_ = promoted.Conn.Write(writeCtx, websocket.MessageText, grantMsg)
		writeCancel()

		sm.logger.Info("watcher auto-promoted to controller",
			"session_id", ls.ID,
			"user_id", promoted.UserID,
		)
	}

	// Notify remaining clients about the departure
	if client.Role == roleWatcher || wasController {
		eventType := "watcher_left"
		if wasController {
			eventType = "controller_changed"
		}
		sm.broadcastJSON(ctx, ls, "", map[string]interface{}{
			"type":         eventType,
			"userId":       client.UserID,
			"watcherCount": watcherCount,
		})
	}

	if allGone {
		if err := sm.db.DetachShellSession(ctx, ls.ID); err != nil {
			sm.logger.Error("failed to detach shell session in DB", "session_id", ls.ID, "error", err)
		}

		sm.logger.Info("all clients detached", "session_id", ls.ID, "pinned", ls.Pinned)

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
	} else {
		sm.eventBus.Publish(ls.TenantID, Event{
			Channel: "shell",
			Type:    "shell.session.watcher.left",
			Data: map[string]string{
				"sessionId": ls.ID,
				"agentId":   ls.AgentID,
				"userId":    client.UserID,
			},
		})
	}
}

// GetClientByUserID returns a connected client by user ID, or nil if not found.
func (ls *LiveSession) GetClientByUserID(userID string) *SessionClient {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	for _, c := range ls.clients {
		if c.UserID == userID {
			return c
		}
	}
	return nil
}

// broadcastJSON sends a JSON message to all connected clients, optionally excluding one.
func (sm *SessionManager) broadcastJSON(ctx context.Context, ls *LiveSession, excludeClientID string, msg interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	ls.mu.Lock()
	clients := make([]*SessionClient, 0, len(ls.clients))
	for _, c := range ls.clients {
		if c.ID != excludeClientID {
			clients = append(clients, c)
		}
	}
	ls.mu.Unlock()

	for _, c := range clients {
		writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
		_ = c.Conn.Write(writeCtx, websocket.MessageText, data)
		writeCancel()
	}
}

// countWatchersLocked returns the number of watcher clients. Must be called with ls.mu held.
func (sm *SessionManager) countWatchersLocked(ls *LiveSession) int {
	count := 0
	for _, c := range ls.clients {
		if c.Role == roleWatcher {
			count++
		}
	}
	return count
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
