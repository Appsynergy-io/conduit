package protocol

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"log/slog"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestTLS creates a self-signed TLS config for QUIC testing.
func generateTestTLS(t *testing.T) *tls.Config {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(1 * time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)

	return &tls.Config{
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{certDER},
			PrivateKey:  key,
		}},
		NextProtos:         []string{"conduit-cwp-v1"},
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
	}
}

func TestQUICMux_SendReceive(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	tlsCfg := generateTestTLS(t)

	// Start QUIC server
	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	require.NoError(t, err)

	tr := &quic.Transport{Conn: udpConn}
	defer tr.Close()

	ln, err := tr.Listen(tlsCfg, &quic.Config{})
	require.NoError(t, err)
	defer ln.Close()

	serverAddr := udpConn.LocalAddr().String()

	// Server: accept connection, read a frame, send a frame back
	serverDone := make(chan *Frame, 1)
	go func() {
		conn, err := ln.Accept(context.Background())
		if err != nil {
			return
		}
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			return
		}
		mux := NewQUICMux(conn, stream, logger)
		readErr := make(chan error, 1)
		go func() { readErr <- mux.ReadLoop(context.Background()) }()

		// Read from global channel
		f := <-mux.Global()
		serverDone <- f

		// Send a response
		resp := &Frame{Type: FrameAuthOK, StreamID: 0}
		mux.Send(context.Background(), resp)
	}()

	// Client: connect, send a frame, read a frame
	clientTLS := &tls.Config{
		NextProtos:         []string{"conduit-cwp-v1"},
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
	}

	conn, err := quic.DialAddr(context.Background(), serverAddr, clientTLS, &quic.Config{})
	require.NoError(t, err)
	defer conn.CloseWithError(0, "")

	stream, err := conn.OpenStreamSync(context.Background())
	require.NoError(t, err)

	clientMux := NewQUICMux(conn, stream, logger)
	clientReadErr := make(chan error, 1)
	go func() { clientReadErr <- clientMux.ReadLoop(context.Background()) }()

	// Send HELLO frame
	hello := &Frame{Type: FrameHello, StreamID: 0, Payload: []byte(`{"agentId":"test"}`)}
	err = clientMux.Send(context.Background(), hello)
	require.NoError(t, err)

	// Server should have received it
	received := <-serverDone
	assert.Equal(t, FrameHello, received.Type)
	assert.Equal(t, []byte(`{"agentId":"test"}`), received.Payload)

	// Client should receive AUTH_OK
	resp := <-clientMux.Global()
	assert.Equal(t, FrameAuthOK, resp.Type)
}

func TestQUICMux_StreamRouting(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	tlsCfg := generateTestTLS(t)

	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	require.NoError(t, err)
	tr := &quic.Transport{Conn: udpConn}
	defer tr.Close()

	ln, err := tr.Listen(tlsCfg, &quic.Config{})
	require.NoError(t, err)
	defer ln.Close()

	// Server: echo frames back
	go func() {
		conn, _ := ln.Accept(context.Background())
		stream, _ := conn.AcceptStream(context.Background())
		mux := NewQUICMux(conn, stream, logger)
		go mux.ReadLoop(context.Background())

		for f := range mux.Global() {
			mux.Send(context.Background(), f)
		}
	}()

	clientTLS := &tls.Config{
		NextProtos:         []string{"conduit-cwp-v1"},
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
	}

	conn, err := quic.DialAddr(context.Background(), udpConn.LocalAddr().String(), clientTLS, &quic.Config{})
	require.NoError(t, err)
	defer conn.CloseWithError(0, "")

	stream, err := conn.OpenStreamSync(context.Background())
	require.NoError(t, err)

	mux := NewQUICMux(conn, stream, logger)
	go mux.ReadLoop(context.Background())

	// Open a stream and verify routing
	streamID := mux.NextStreamID()
	ch := mux.OpenStream(streamID)

	// Send a frame with the stream ID
	f := &Frame{Type: FrameShellData, StreamID: streamID, Payload: []byte("hello")}
	err = mux.Send(context.Background(), f)
	require.NoError(t, err)

	// Should arrive on the registered stream channel
	select {
	case received := <-ch:
		assert.Equal(t, FrameShellData, received.Type)
		assert.Equal(t, streamID, received.StreamID)
		assert.Equal(t, []byte("hello"), received.Payload)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for routed frame")
	}

	// Close stream and verify cleanup
	mux.CloseStream(streamID)
	assert.Equal(t, 0, mux.StreamCount())
}

func TestQUICMux_ImplementsFrameMux(t *testing.T) {
	// Compile-time check is via var _ FrameMux = (*QUICMux)(nil)
	// but let's also verify at runtime
	var _ FrameMux = &QUICMux{}
}
