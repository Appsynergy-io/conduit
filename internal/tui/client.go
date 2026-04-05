package tui

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// Client talks to the Conduit server API over HTTP and WebSocket.
type Client struct {
	ServerURL   string
	Token       string
	DevInsecure bool
	httpClient  *http.Client
}

// Agent is the API representation of a registered agent.
type Agent struct {
	ID          string  `json:"id"`
	Hostname    string  `json:"hostname"`
	DisplayName *string `json:"displayName"`
	OS          *string `json:"os"`
	Arch        *string `json:"arch"`
	IP          *string `json:"ip"`
	Status      string  `json:"status"`
	Transport   *string `json:"transport"`
	Version     *string `json:"version"`
}

// ShellSession is the API representation of a shell session.
type ShellSession struct {
	ID         string  `json:"id"`
	AgentID    string  `json:"agentId"`
	UserID     string  `json:"userId"`
	Status     string  `json:"status"`
	Pinned     int     `json:"pinned"`
	CreatedAt  string  `json:"createdAt"`
	DetachedAt *string `json:"detachedAt"`
}

// NewClient creates a new API client for the given server.
func NewClient(serverURL string, devInsecure bool) *Client {
	httpClient := &http.Client{Timeout: 30 * time.Second}
	if devInsecure {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionTLS12,
			},
		}
	}
	return &Client{
		ServerURL:   strings.TrimRight(serverURL, "/"),
		DevInsecure: devInsecure,
		httpClient:  httpClient,
	}
}

// SetToken sets the JWT token directly (for stored credential flows).
func (c *Client) SetToken(token string) {
	c.Token = token
}

// Login authenticates with email+password (dev mode) and stores the JWT.
func (c *Client) Login(email, password string) error {
	body, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})

	req, err := http.NewRequest("POST", c.ServerURL+"/api/v1/auth/password/login", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("parsing login response: %w", err)
	}

	c.Token = result.AccessToken
	return nil
}

// ListAgents fetches all agents from the server.
func (c *Client) ListAgents(ctx context.Context) ([]Agent, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.ServerURL+"/api/v1/agents", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching agents: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agent list failed (HTTP %d)", resp.StatusCode)
	}

	var result struct {
		Data []Agent `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing agents: %w", err)
	}

	return result.Data, nil
}

// ListShellSessions fetches active/detached shell sessions.
func (c *Client) ListShellSessions(ctx context.Context) ([]ShellSession, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.ServerURL+"/api/v1/shell/sessions", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching sessions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("session list failed (HTTP %d)", resp.StatusCode)
	}

	var result struct {
		Data []ShellSession `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing sessions: %w", err)
	}

	return result.Data, nil
}

// DialShell opens a WebSocket shell session to the given agent.
func (c *Client) DialShell(ctx context.Context, agentID string, cols, rows int) (*websocket.Conn, error) {
	wsURL := strings.Replace(c.ServerURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL = fmt.Sprintf("%s/api/v1/agents/%s/shell/new?cols=%d&rows=%d", wsURL, agentID, cols, rows)

	opts := &websocket.DialOptions{
		Subprotocols: []string{"conduit-shell-v1"},
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + c.Token},
		},
	}
	if c.DevInsecure {
		opts.HTTPClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
					MinVersion:         tls.VersionTLS12,
				},
			},
		}
	}

	conn, _, err := websocket.Dial(ctx, wsURL, opts)
	if err != nil {
		return nil, fmt.Errorf("connecting to shell: %w", err)
	}

	conn.SetReadLimit(1 << 20)
	return conn, nil
}

// DialShellResume opens a WebSocket to resume an existing shell session.
func (c *Client) DialShellResume(ctx context.Context, agentID, sessionID string, cols, rows int) (*websocket.Conn, error) {
	wsURL := strings.Replace(c.ServerURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL = fmt.Sprintf("%s/api/v1/agents/%s/shell/sessions/%s/ws?cols=%d&rows=%d", wsURL, agentID, sessionID, cols, rows)

	opts := &websocket.DialOptions{
		Subprotocols: []string{"conduit-shell-v1"},
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + c.Token},
		},
	}
	if c.DevInsecure {
		opts.HTTPClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
					MinVersion:         tls.VersionTLS12,
				},
			},
		}
	}

	conn, _, err := websocket.Dial(ctx, wsURL, opts)
	if err != nil {
		return nil, fmt.Errorf("resuming session: %w", err)
	}

	conn.SetReadLimit(1 << 20)
	return conn, nil
}

// ToggleSessionPin toggles the pinned state of a shell session.
// Returns the new pinned state.
func (c *Client) ToggleSessionPin(agentID, sessionID string) (bool, error) {
	// Get the current session state
	req, err := http.NewRequest("GET", c.ServerURL+"/api/v1/agents/"+agentID+"/shell/sessions/"+sessionID, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var session struct {
		Pinned int `json:"pinned"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return false, err
	}

	// Toggle
	newPinned := session.Pinned == 0
	body, _ := json.Marshal(map[string]bool{"pinned": newPinned})
	patchReq, err := http.NewRequest("PATCH", c.ServerURL+"/api/v1/agents/"+agentID+"/shell/sessions/"+sessionID, strings.NewReader(string(body)))
	if err != nil {
		return false, err
	}
	patchReq.Header.Set("Authorization", "Bearer "+c.Token)
	patchReq.Header.Set("Content-Type", "application/json")

	patchResp, err := c.httpClient.Do(patchReq)
	if err != nil {
		return false, err
	}
	defer patchResp.Body.Close()

	if patchResp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("pin toggle failed (HTTP %d)", patchResp.StatusCode)
	}

	return newPinned, nil
}

// DialEventStream opens a WebSocket connection to the EventBus for real-time updates.
// Returns the connection. Caller is responsible for reading messages and closing.
func (c *Client) DialEventStream(ctx context.Context, channels string) (*websocket.Conn, error) {
	wsURL := strings.Replace(c.ServerURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL = fmt.Sprintf("%s/api/v1/events/stream?channels=%s", wsURL, channels)

	opts := &websocket.DialOptions{
		Subprotocols: []string{"conduit-events-v1"},
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + c.Token},
		},
	}
	if c.DevInsecure {
		opts.HTTPClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
					MinVersion:         tls.VersionTLS12,
				},
			},
		}
	}

	conn, _, err := websocket.Dial(ctx, wsURL, opts)
	if err != nil {
		return nil, fmt.Errorf("connecting to event stream: %w", err)
	}

	return conn, nil
}

// TerminateSession closes a shell session.
func (c *Client) TerminateSession(agentID, sessionID string) error {
	req, err := http.NewRequest("DELETE", c.ServerURL+"/api/v1/agents/"+agentID+"/shell/sessions/"+sessionID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("terminate failed (HTTP %d)", resp.StatusCode)
	}
	return nil
}
