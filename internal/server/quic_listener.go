package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"

	"github.com/appsynergy-io/conduit/internal/protocol"
)

// alpnDemuxListener wraps a quic.Listener and filters connections by ALPN.
// Connections matching the target ALPN are passed through; others are rejected.
type alpnDemuxListener struct {
	ch   chan *quic.Conn
	addr net.Addr
	done chan struct{}
}

func (l *alpnDemuxListener) Accept(_ context.Context) (*quic.Conn, error) {
	select {
	case conn := <-l.ch:
		return conn, nil
	case <-l.done:
		return nil, http.ErrServerClosed
	}
}

func (l *alpnDemuxListener) Addr() net.Addr { return l.addr }

func (l *alpnDemuxListener) Close() error {
	select {
	case <-l.done:
	default:
		close(l.done)
	}
	return nil
}

// startQUICServices starts a single shared UDP listener and demultiplexes
// incoming QUIC connections by ALPN:
//   - "conduit-cwp-v1" → agent CWP handler
//   - "h3" → HTTP/3 server for browsers
//
// This avoids the port conflict that would occur with separate UDP listeners.
func (s *Server) startQUICServices(ctx context.Context) error {
	addr := s.cfg.Server.QUICAddr
	if addr == "" {
		return nil
	}

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("resolving QUIC address %s: %w", addr, err)
	}

	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("listening UDP on %s: %w", addr, err)
	}

	transport := &quic.Transport{Conn: udpConn}
	s.quicTransport = transport

	// Single TLS config with both ALPNs
	quicTLS := s.tlsConfig.Clone()
	quicTLS.NextProtos = []string{"conduit-cwp-v1", "h3"}

	ln, err := transport.ListenEarly(quicTLS, &quic.Config{
		Allow0RTT:       false,
		KeepAlivePeriod: 15 * time.Second,
		MaxIdleTimeout:  60 * time.Second,
	})
	if err != nil {
		udpConn.Close()
		return fmt.Errorf("creating QUIC listener: %w", err)
	}

	s.quicEarlyListener = ln

	// Create filtered listener for HTTP/3
	h3Listener := &alpnDemuxListener{
		ch:   make(chan *quic.Conn, 32),
		addr: udpConn.LocalAddr(),
		done: make(chan struct{}),
	}
	s.http3Listener = h3Listener

	// Start HTTP/3 server on the filtered listener
	s.http3Srv = &http3.Server{
		Handler: s.router,
	}
	go func() {
		if serveErr := s.http3Srv.ServeListener(h3Listener); serveErr != nil && serveErr != http.ErrServerClosed {
			s.logger.ErrorContext(ctx, "HTTP/3 server error", "error", serveErr)
		}
	}()

	// Accept loop: demux connections by ALPN
	go s.demuxQUICConnections(ctx, ln, h3Listener)

	s.logger.InfoContext(ctx, "QUIC services started",
		"addr", addr,
		"protocols", "conduit-cwp-v1, h3",
	)
	return nil
}

// demuxQUICConnections accepts QUIC connections and routes them by ALPN.
func (s *Server) demuxQUICConnections(ctx context.Context, ln *quic.EarlyListener, h3Listener *alpnDemuxListener) {
	for {
		conn, err := ln.Accept(ctx)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, quic.ErrServerClosed) {
				return
			}
			s.logger.Error("QUIC accept error", "error", err)
			continue
		}

		alpn := conn.ConnectionState().TLS.NegotiatedProtocol

		switch alpn {
		case "conduit-cwp-v1":
			go s.handleAgentQUIC(conn)
		case "h3":
			select {
			case h3Listener.ch <- conn:
			default:
				s.logger.Warn("HTTP/3 listener backlog full, dropping connection",
					"remote_addr", conn.RemoteAddr(),
				)
				conn.CloseWithError(0, "server busy")
			}
		default:
			s.logger.Warn("unknown QUIC ALPN, closing connection",
				"alpn", alpn,
				"remote_addr", conn.RemoteAddr(),
			)
			conn.CloseWithError(1, "unsupported protocol")
		}
	}
}

// handleAgentQUIC handles an inbound agent QUIC connection.
// The agent opens a control stream and performs the CWP HELLO+AUTH handshake.
func (s *Server) handleAgentQUIC(qconn *quic.Conn) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Accept the control stream opened by the agent.
	stream, err := qconn.AcceptStream(ctx)
	if err != nil {
		s.logger.Warn("QUIC agent failed to open control stream", "error", err, "remote_addr", qconn.RemoteAddr())
		qconn.CloseWithError(1, "no control stream")
		return
	}

	mux := protocol.NewQUICMux(qconn, stream, s.logger)

	readErr := make(chan error, 1)
	go func() {
		readErr <- mux.ReadLoop(ctx)
	}()

	agent, hello, err := s.authenticateAgent(ctx, mux, readErr, qconn.RemoteAddr().String())
	if err != nil {
		s.logger.Warn("agent QUIC auth failed", "error", err, "remote_addr", qconn.RemoteAddr())
		mux.Close()
		return
	}

	s.onAgentAuthenticated(ctx, cancel, mux, readErr, agent, hello, "quic", qconn.RemoteAddr().String())
}

// stopQUICServices shuts down all QUIC services.
func (s *Server) stopQUICServices() {
	if s.http3Srv != nil {
		s.http3Srv.Close()
	}
	if s.http3Listener != nil {
		s.http3Listener.Close()
	}
	if s.quicEarlyListener != nil {
		s.quicEarlyListener.Close()
	}
	if s.quicTransport != nil {
		s.quicTransport.Close()
	}
}
