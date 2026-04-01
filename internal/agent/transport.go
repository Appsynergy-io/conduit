package agent

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/protocol"
)

const (
	handshakeTimeout = 10 * time.Second
)

// connect dials the server WebSocket endpoint and performs the CWP handshake.
// Returns an authenticated Mux and the ReadLoop error channel on success.
func (a *Agent) connect(ctx context.Context) (*protocol.Mux, <-chan error, error) {
	wsURL := a.buildWSURL()

	dialCtx, dialCancel := context.WithTimeout(ctx, handshakeTimeout)
	defer dialCancel()

	opts := &websocket.DialOptions{
		Subprotocols: []string{"conduit-cwp-v1"},
		HTTPHeader:   http.Header{},
	}

	// Configure TLS for dev-insecure mode
	if a.cfg.DevInsecure {
		opts.HTTPClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
					MinVersion:         tls.VersionTLS13,
				},
			},
		}
	}

	conn, _, err := websocket.Dial(dialCtx, wsURL, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("dialing %s: %w", wsURL, err)
	}

	// Set generous read limit for CWP frames
	conn.SetReadLimit(protocol.MaxPayloadSize + protocol.HeaderSize)

	mux := protocol.NewMux(conn, a.logger)

	// Perform CWP handshake
	readErr, err := a.handshake(ctx, mux)
	if err != nil {
		mux.Close()
		return nil, nil, fmt.Errorf("handshake: %w", err)
	}

	return mux, readErr, nil
}

// buildWSURL constructs the WebSocket URL from the server URL config.
func (a *Agent) buildWSURL() string {
	base := a.cfg.ServerURL

	// Normalize: strip trailing slash
	base = strings.TrimRight(base, "/")

	// Convert https:// to wss://
	if strings.HasPrefix(base, "https://") {
		base = "wss://" + base[8:]
	} else if strings.HasPrefix(base, "http://") {
		base = "ws://" + base[7:]
	} else if !strings.HasPrefix(base, "wss://") && !strings.HasPrefix(base, "ws://") {
		base = "wss://" + base
	}

	return base + "/agent/v1/connect"
}

// handshake performs the CWP HELLO + AUTH two-frame handshake.
// Returns a readErr channel from the ReadLoop that was started during handshake.
// The ReadLoop uses the parent ctx so it survives after handshake completes.
//
// Flow:
// 1. Agent sends HELLO (agentID, hostname, OS, arch, version)
// 2. Agent sends AUTH (HMAC-SHA256 signature of HELLO payload + nonce)
// 3. Server responds with AUTH_OK or AUTH_REJECT
func (a *Agent) handshake(ctx context.Context, mux *protocol.Mux) (<-chan error, error) {
	hsCtx, hsCancel := context.WithTimeout(ctx, handshakeTimeout)
	defer hsCancel()

	// Build HELLO payload
	hello := protocol.HelloPayload{
		AgentID:  a.cfg.AgentID,
		Hostname: Hostname(),
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		Version:  Version(),
	}

	helloFrame, err := protocol.NewFrame(protocol.FrameHello, 0, hello)
	if err != nil {
		return nil, fmt.Errorf("creating HELLO frame: %w", err)
	}

	// Send HELLO
	if err := mux.Send(hsCtx, helloFrame); err != nil {
		return nil, fmt.Errorf("sending HELLO: %w", err)
	}

	// Compute HMAC-SHA256 signature
	// Protocol: HMAC-SHA256(key=SHA256(agentKey), msg=helloPayload||nonce)
	nonce := uuid.NewString()

	agentKeyBytes, err := hex.DecodeString(a.cfg.AgentKey)
	if err != nil {
		return nil, fmt.Errorf("decoding agent key: %w", err)
	}
	keyHash := sha256.Sum256(agentKeyBytes)

	mac := hmac.New(sha256.New, keyHash[:])
	mac.Write(helloFrame.Payload)
	mac.Write([]byte(nonce))
	signature := hex.EncodeToString(mac.Sum(nil))

	// Build AUTH payload
	authPayload := protocol.AuthPayload{
		AgentID:   a.cfg.AgentID,
		Signature: signature,
		Nonce:     nonce,
	}

	authFrame, err := protocol.NewFrame(protocol.FrameAuth, 0, authPayload)
	if err != nil {
		return nil, fmt.Errorf("creating AUTH frame: %w", err)
	}

	// Send AUTH
	if err := mux.Send(hsCtx, authFrame); err != nil {
		return nil, fmt.Errorf("sending AUTH: %w", err)
	}

	// Start read loop to receive the response.
	// Use the parent ctx (not hsCtx) so the ReadLoop survives after handshake
	// completes — connectAndRun reuses it for the main loop.
	readErr := make(chan error, 1)
	go func() {
		readErr <- mux.ReadLoop(ctx)
	}()

	// Wait for AUTH_OK or AUTH_REJECT
	select {
	case f := <-mux.Global():
		switch f.Type {
		case protocol.FrameAuthOK:
			a.logger.Info("authenticated with server")
			return readErr, nil
		case protocol.FrameAuthReject:
			reason := string(f.Payload)
			return nil, fmt.Errorf("auth rejected: %s", reason)
		default:
			return nil, fmt.Errorf("unexpected frame during handshake: %s", f.Type.String())
		}

	case err := <-readErr:
		return nil, fmt.Errorf("connection lost during handshake: %w", err)

	case <-hsCtx.Done():
		return nil, fmt.Errorf("handshake timeout")
	}
}

// Version returns the agent binary version.
func Version() string {
	return "0.1.0"
}
