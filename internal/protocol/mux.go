package protocol

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/coder/websocket"
)

// FrameMux is the interface for multiplexed CWP frame transport.
// Both WebSocket (Mux) and QUIC (QUICMux) implement this.
type FrameMux interface {
	// Global returns the channel for frames not routed to a specific stream.
	Global() <-chan *Frame
	// Done returns a channel that is closed when the mux stops.
	Done() <-chan struct{}
	// NextStreamID allocates a new unique stream ID.
	NextStreamID() uint32
	// OpenStream registers a handler for the given stream ID.
	OpenStream(id uint32) <-chan *Frame
	// CloseStream unregisters a stream handler.
	CloseStream(id uint32)
	// Send writes a frame to the connection. Safe for concurrent use.
	Send(ctx context.Context, f *Frame) error
	// ReadLoop reads frames and dispatches them. Blocks until error or close.
	ReadLoop(ctx context.Context) error
	// Close cleanly shuts down the multiplexer and underlying connection.
	Close() error
	// StreamCount returns the number of open streams.
	StreamCount() int
}

// Verify Mux implements FrameMux at compile time.
var _ FrameMux = (*Mux)(nil)

// Mux multiplexes CWP frames over a single WebSocket connection.
// Each logical operation (shell session, file transfer, exec job) gets its own
// StreamID. The Mux routes incoming frames to registered stream handlers and
// serializes outgoing frames from multiple goroutines onto the single connection.
type Mux struct {
	conn   *websocket.Conn
	logger *slog.Logger

	mu      sync.RWMutex
	streams map[uint32]chan *Frame // registered stream handlers
	global  chan *Frame            // frames with no registered stream (e.g. HELLO, AUTH, PING)

	nextID atomic.Uint32
	done   chan struct{}
	once   sync.Once
}

// NewMux creates a multiplexer over the given WebSocket connection.
// The global channel receives frames for unregistered stream IDs.
func NewMux(conn *websocket.Conn, logger *slog.Logger) *Mux {
	return &Mux{
		conn:    conn,
		logger:  logger,
		streams: make(map[uint32]chan *Frame),
		global:  make(chan *Frame, 64),
		done:    make(chan struct{}),
	}
}

// Global returns the channel for frames not routed to a specific stream.
// Control frames (HELLO, AUTH, PING/PONG, AGENT_INFO) typically arrive here.
func (m *Mux) Global() <-chan *Frame {
	return m.global
}

// Done returns a channel that is closed when the mux stops.
func (m *Mux) Done() <-chan struct{} {
	return m.done
}

// NextStreamID allocates a new unique stream ID.
func (m *Mux) NextStreamID() uint32 {
	return m.nextID.Add(1)
}

// OpenStream registers a handler for the given stream ID and returns
// a channel that will receive frames for that stream.
func (m *Mux) OpenStream(id uint32) <-chan *Frame {
	ch := make(chan *Frame, 64)
	m.mu.Lock()
	m.streams[id] = ch
	m.mu.Unlock()
	return ch
}

// CloseStream unregisters a stream handler.
func (m *Mux) CloseStream(id uint32) {
	m.mu.Lock()
	if ch, ok := m.streams[id]; ok {
		delete(m.streams, id)
		close(ch)
	}
	m.mu.Unlock()
}

// Send writes a frame to the WebSocket connection. Safe for concurrent use.
func (m *Mux) Send(ctx context.Context, f *Frame) error {
	data, err := EncodeBytes(f)
	if err != nil {
		return fmt.Errorf("encoding frame: %w", err)
	}
	return m.conn.Write(ctx, websocket.MessageBinary, data)
}

// ReadLoop reads frames from the WebSocket and dispatches them to registered
// stream handlers or the global channel. It blocks until the connection is
// closed or an error occurs, then closes the Done channel.
func (m *Mux) ReadLoop(ctx context.Context) error {
	defer m.close()

	for {
		_, data, err := m.conn.Read(ctx)
		if err != nil {
			return fmt.Errorf("reading websocket message: %w", err)
		}

		frame, err := DecodeBytes(data)
		if err != nil {
			m.logger.Warn("dropping malformed frame", "error", err)
			continue
		}

		m.dispatch(frame)
	}
}

// dispatch routes a frame to the appropriate stream or global channel.
func (m *Mux) dispatch(f *Frame) {
	m.mu.RLock()
	ch, ok := m.streams[f.StreamID]
	m.mu.RUnlock()

	if ok {
		select {
		case ch <- f:
		default:
			m.logger.Warn("dropping frame for slow stream",
				"stream_id", f.StreamID,
				"type", f.Type.String(),
			)
		}
		return
	}

	// No registered stream — send to global
	select {
	case m.global <- f:
	default:
		m.logger.Warn("dropping frame for full global channel",
			"stream_id", f.StreamID,
			"type", f.Type.String(),
		)
	}
}

// close shuts down the mux and all stream channels.
func (m *Mux) close() {
	m.once.Do(func() {
		close(m.done)

		m.mu.Lock()
		for id, ch := range m.streams {
			delete(m.streams, id)
			close(ch)
		}
		m.mu.Unlock()
	})
}

// Close cleanly shuts down the multiplexer and WebSocket connection.
func (m *Mux) Close() error {
	m.close()
	return m.conn.Close(websocket.StatusNormalClosure, "")
}

// StreamCount returns the number of open streams.
func (m *Mux) StreamCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.streams)
}
