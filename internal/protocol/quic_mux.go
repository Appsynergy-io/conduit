package protocol

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/quic-go/quic-go"
)

// Verify QUICMux implements FrameMux at compile time.
var _ FrameMux = (*QUICMux)(nil)

// QUICMux multiplexes CWP frames over a QUIC connection.
// Unlike WebSocket Mux which multiplexes by StreamID over a single connection,
// QUICMux uses a single control stream for all frames. QUIC's native stream
// multiplexing can be used at a higher layer if needed, but the CWP framing
// stays identical to WebSocket for protocol compatibility.
type QUICMux struct {
	conn   *quic.Conn
	stream *quic.Stream // control stream for all CWP frames
	logger *slog.Logger

	mu      sync.RWMutex
	streams map[uint32]chan *Frame
	global  chan *Frame

	writeMu sync.Mutex // serializes writes to the QUIC stream
	nextID  atomic.Uint32
	done    chan struct{}
	once    sync.Once
}

// NewQUICMux creates a multiplexer over the given QUIC connection and control stream.
// The control stream carries all CWP frames (identical framing to WebSocket).
func NewQUICMux(conn *quic.Conn, stream *quic.Stream, logger *slog.Logger) *QUICMux {
	return &QUICMux{
		conn:    conn,
		stream:  stream,
		logger:  logger,
		streams: make(map[uint32]chan *Frame),
		global:  make(chan *Frame, 64),
		done:    make(chan struct{}),
	}
}

// Global returns the channel for frames not routed to a specific stream.
func (m *QUICMux) Global() <-chan *Frame {
	return m.global
}

// Done returns a channel that is closed when the mux stops.
func (m *QUICMux) Done() <-chan struct{} {
	return m.done
}

// NextStreamID allocates a new unique stream ID.
func (m *QUICMux) NextStreamID() uint32 {
	return m.nextID.Add(1)
}

// OpenStream registers a handler for the given stream ID and returns
// a channel that will receive frames for that stream.
func (m *QUICMux) OpenStream(id uint32) <-chan *Frame {
	ch := make(chan *Frame, 64)
	m.mu.Lock()
	m.streams[id] = ch
	m.mu.Unlock()
	return ch
}

// CloseStream unregisters a stream handler.
func (m *QUICMux) CloseStream(id uint32) {
	m.mu.Lock()
	if ch, ok := m.streams[id]; ok {
		delete(m.streams, id)
		close(ch)
	}
	m.mu.Unlock()
}

// Send writes a frame to the QUIC control stream. Safe for concurrent use.
func (m *QUICMux) Send(ctx context.Context, f *Frame) error {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()

	if _, err := Encode(m.stream, f); err != nil {
		return fmt.Errorf("encoding frame to QUIC stream: %w", err)
	}
	return nil
}

// ReadLoop reads frames from the QUIC control stream and dispatches them
// to registered stream handlers or the global channel.
func (m *QUICMux) ReadLoop(ctx context.Context) error {
	defer m.close()

	for {
		frame, err := Decode(m.stream)
		if err != nil {
			return fmt.Errorf("reading QUIC stream: %w", err)
		}

		m.dispatch(frame)
	}
}

// dispatch routes a frame to the appropriate stream or global channel.
func (m *QUICMux) dispatch(f *Frame) {
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
func (m *QUICMux) close() {
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

// Close cleanly shuts down the multiplexer and QUIC connection.
func (m *QUICMux) Close() error {
	m.close()
	return m.conn.CloseWithError(0, "")
}

// StreamCount returns the number of open streams.
func (m *QUICMux) StreamCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.streams)
}
