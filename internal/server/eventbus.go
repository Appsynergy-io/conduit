package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/middleware"
)

// eventThrottleWindow is the minimum interval between published events
// for the same throttle key (e.g., same agent's connect/disconnect events).
// During rapid flapping, only the first event publishes immediately;
// the latest pending event publishes after the window expires.
const eventThrottleWindow = 5 * time.Second

// EventBus manages WebSocket connections for real-time event delivery.
// Clients subscribe to channels and receive JSON events pushed by the server.
type EventBus struct {
	mu      sync.RWMutex
	clients map[*eventClient]struct{}
	logger  *slog.Logger

	// Throttle state for rapid event suppression (e.g., agent flapping).
	throttleMu sync.Mutex
	throttle   map[string]*throttleEntry
}

// throttleEntry tracks per-key rate limiting state.
type throttleEntry struct {
	lastSent time.Time
	pending  *pendingEvent // latest suppressed event (published after cooldown)
	timer    *time.Timer
}

// pendingEvent is an event waiting to be published after a throttle window.
type pendingEvent struct {
	tenantID string
	event    Event
}

// eventClient represents a connected WebSocket client.
type eventClient struct {
	conn     *websocket.Conn
	tenantID string
	userID   string
	channels map[string]bool // subscribed channels
	send     chan []byte
	done     chan struct{}
}

// Event is a server-sent event pushed to subscribed clients.
type Event struct {
	Channel   string      `json:"channel"`
	Type      string      `json:"type"`
	Timestamp string      `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// Valid EventBus channels.
var validChannels = map[string]bool{
	"agents":  true,
	"shell":   true,
	"files":   true,
	"auth":    true,
	"audit":   true,
	"metrics": true,
	"exec":    true,
	"system":  true,
}

// NewEventBus creates a new EventBus instance.
func NewEventBus(logger *slog.Logger) *EventBus {
	return &EventBus{
		clients:  make(map[*eventClient]struct{}),
		logger:   logger,
		throttle: make(map[string]*throttleEntry),
	}
}

// Publish sends an event to all clients subscribed to the event's channel
// and belonging to the specified tenant. Events matching a throttle key
// (e.g., rapid agent connect/disconnect) are rate-limited to prevent
// flooding browsers during flapping.
func (eb *EventBus) Publish(tenantID string, evt Event) {
	if evt.Timestamp == "" {
		evt.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	// Throttle rapid state-change events (e.g., agent connect/disconnect flapping)
	if key := eventThrottleKey(evt); key != "" {
		if eb.shouldThrottle(key, tenantID, evt) {
			return
		}
	}

	eb.publishToClients(tenantID, evt)
}

// publishToClients delivers an event to all matching WebSocket clients.
func (eb *EventBus) publishToClients(tenantID string, evt Event) {
	data, err := json.Marshal(evt)
	if err != nil {
		eb.logger.Error("failed to marshal event", "error", err)
		return
	}

	eb.mu.RLock()
	defer eb.mu.RUnlock()

	for client := range eb.clients {
		if client.tenantID != tenantID {
			continue
		}
		if !client.channels[evt.Channel] {
			continue
		}
		select {
		case client.send <- data:
		default:
			// Client buffer full — skip to avoid blocking publisher
			eb.logger.Warn("dropping event for slow client",
				"user_id", client.userID,
				"channel", evt.Channel,
			)
		}
	}
}

// eventThrottleKey returns a throttle key for events that should be rate-limited,
// or "" if the event should not be throttled. Agent connection state events are
// keyed by agent ID so rapid flapping doesn't flood the EventBus.
func eventThrottleKey(evt Event) string {
	switch evt.Type {
	case "agent.connected", "agent.disconnected":
		if m, ok := evt.Data.(map[string]string); ok {
			if id := m["agentId"]; id != "" {
				return "agent:" + id
			}
		}
	}
	return ""
}

// shouldThrottle returns true if the event should be suppressed. The first event
// for a key publishes immediately. Subsequent events within the throttle window
// are stored as pending; the latest pending event publishes when the window expires.
func (eb *EventBus) shouldThrottle(key, tenantID string, evt Event) bool {
	eb.throttleMu.Lock()
	defer eb.throttleMu.Unlock()

	entry, exists := eb.throttle[key]
	if !exists || time.Since(entry.lastSent) >= eventThrottleWindow {
		// First event or cooldown expired — publish immediately
		if exists && entry.timer != nil {
			entry.timer.Stop()
		}
		eb.throttle[key] = &throttleEntry{lastSent: time.Now()}
		return false
	}

	// Within cooldown — suppress and store as pending (last-write-wins)
	entry.pending = &pendingEvent{tenantID: tenantID, event: evt}

	if entry.timer == nil {
		remaining := eventThrottleWindow - time.Since(entry.lastSent)
		entry.timer = time.AfterFunc(remaining, func() {
			eb.flushPending(key)
		})
	}

	eb.logger.Debug("throttling rapid event",
		"type", evt.Type,
		"key", key,
	)
	return true
}

// flushPending publishes the latest pending event for a throttle key and
// resets the entry so the next event can publish immediately.
func (eb *EventBus) flushPending(key string) {
	eb.throttleMu.Lock()
	entry, ok := eb.throttle[key]
	if !ok {
		eb.throttleMu.Unlock()
		return
	}

	pending := entry.pending
	if pending == nil {
		// No pending event — clean up the entry entirely
		delete(eb.throttle, key)
		eb.throttleMu.Unlock()
		return
	}

	// Reset for next cycle
	entry.pending = nil
	entry.lastSent = time.Now()
	entry.timer = nil
	eb.throttleMu.Unlock()

	eb.publishToClients(pending.tenantID, pending.event)
}

// StopThrottleTimers cancels all pending throttle timers (for graceful shutdown).
func (eb *EventBus) StopThrottleTimers() {
	eb.throttleMu.Lock()
	defer eb.throttleMu.Unlock()
	for key, entry := range eb.throttle {
		if entry.timer != nil {
			entry.timer.Stop()
		}
		delete(eb.throttle, key)
	}
}

// ClientCount returns the number of connected clients.
func (eb *EventBus) ClientCount() int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	return len(eb.clients)
}

// PublishTestEvent exposes publishEvent for integration tests.
func (s *Server) PublishTestEvent(tenantID string, event Event) {
	s.eventBus.Publish(tenantID, event)
}

// handleEventStream handles WebSocket upgrade and event streaming.
// GET /api/v1/events/stream?channels=agents,shell,auth
func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	if s.eventBus == nil {
		apierror.Write(w, r, http.StatusServiceUnavailable, "Service Unavailable",
			"EventBus is not configured.", nil)
		return
	}

	// Authenticate via Authorization header or httpOnly cookie (NIST IA-2, OWASP A07)
	tokenStr := middleware.ExtractToken(r)
	if tokenStr == "" {
		apierror.Unauthorized(w, r, "Authentication required.", nil)
		return
	}

	claims, err := s.jwtMgr.ValidateToken(tokenStr)
	if err != nil {
		apierror.Unauthorized(w, r, "Invalid or expired token.", nil)
		return
	}

	// Parse requested channels
	channelStr := r.URL.Query().Get("channels")
	channels := make(map[string]bool)
	if channelStr != "" {
		for _, ch := range strings.Split(channelStr, ",") {
			ch = strings.TrimSpace(ch)
			if validChannels[ch] {
				channels[ch] = true
			}
		}
	}
	// Default: subscribe to all channels
	if len(channels) == 0 {
		for ch := range validChannels {
			channels[ch] = true
		}
	}

	// Upgrade to WebSocket
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: []string{"conduit-events-v1"},
	})
	if err != nil {
		s.logger.Error("websocket upgrade failed", "error", err)
		return
	}

	client := &eventClient{
		conn:     conn,
		tenantID: claims.TenantID,
		userID:   claims.Subject,
		channels: channels,
		send:     make(chan []byte, 256),
		done:     make(chan struct{}),
	}

	s.eventBus.mu.Lock()
	s.eventBus.clients[client] = struct{}{}
	s.eventBus.mu.Unlock()

	s.logger.Info("eventbus client connected",
		"user_id", claims.Subject,
		"channels", channelStr,
	)

	// Run write and read pumps
	go s.eventWritePump(client)
	go s.eventReadPump(client)
}

// eventWritePump sends events to the client.
func (s *Server) eventWritePump(client *eventClient) {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		s.removeEventClient(client)
	}()

	for {
		select {
		case msg, ok := <-client.send:
			if !ok {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := client.conn.Write(ctx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				s.logger.Debug("eventbus write failed", "error", err)
				return
			}
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := client.conn.Ping(ctx)
			cancel()
			if err != nil {
				return
			}
		case <-client.done:
			return
		}
	}
}

// clientMessage is a message sent from the browser to the EventBus WebSocket.
type clientMessage struct {
	Type     string   `json:"type"`
	Channels []string `json:"channels"`
}

// eventReadPump reads from the client and handles subscribe/unsubscribe/ping.
func (s *Server) eventReadPump(client *eventClient) {
	defer func() {
		close(client.done)
		s.removeEventClient(client)
	}()

	for {
		_, data, err := client.conn.Read(context.Background())
		if err != nil {
			return
		}

		var msg clientMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			s.sendClientError(client, "invalid message format")
			continue
		}

		switch msg.Type {
		case "subscribe":
			s.handleClientSubscribe(client, msg.Channels)
		case "unsubscribe":
			s.handleClientUnsubscribe(client, msg.Channels)
		case "ping":
			s.sendClientPong(client)
		default:
			s.sendClientError(client, "unknown message type")
		}
	}
}

// handleClientSubscribe adds channels to a client's subscriptions.
func (s *Server) handleClientSubscribe(client *eventClient, channels []string) {
	s.eventBus.mu.Lock()
	defer s.eventBus.mu.Unlock()

	for _, ch := range channels {
		ch = strings.TrimSpace(ch)
		if validChannels[ch] {
			client.channels[ch] = true
		}
	}
}

// handleClientUnsubscribe removes channels from a client's subscriptions.
func (s *Server) handleClientUnsubscribe(client *eventClient, channels []string) {
	s.eventBus.mu.Lock()
	defer s.eventBus.mu.Unlock()

	for _, ch := range channels {
		ch = strings.TrimSpace(ch)
		delete(client.channels, ch)
	}
}

// sendClientPong sends a pong response to the client.
func (s *Server) sendClientPong(client *eventClient) {
	msg, _ := json.Marshal(map[string]string{"type": "pong"})
	select {
	case client.send <- msg:
	default:
	}
}

// sendClientError sends an error message to the client.
func (s *Server) sendClientError(client *eventClient, message string) {
	msg, _ := json.Marshal(map[string]string{"type": "error", "message": message})
	select {
	case client.send <- msg:
	default:
	}
}

// removeEventClient unregisters a client and closes its connection.
func (s *Server) removeEventClient(client *eventClient) {
	s.eventBus.mu.Lock()
	if _, ok := s.eventBus.clients[client]; ok {
		delete(s.eventBus.clients, client)
		close(client.send)
	}
	s.eventBus.mu.Unlock()

	client.conn.Close(websocket.StatusNormalClosure, "")
	s.logger.Info("eventbus client disconnected", "user_id", client.userID)
}
