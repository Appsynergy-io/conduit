package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/coder/websocket"
	"golang.org/x/term"
)

// ShellResult holds information about the shell session after it ends.
type ShellResult struct {
	SessionID string
	Detached  bool // true if user detached with ~., false if session exited
}

// RunShell connects to an agent shell via WebSocket and bridges stdin/stdout.
// If resumeSessionID is non-empty, it resumes an existing session instead of creating one.
// It takes over the terminal in raw mode. Returns when the session ends or user detaches.
func RunShell(client *Client, agentID, resumeSessionID string) (*ShellResult, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return nil, fmt.Errorf("not a terminal")
	}

	cols, rows, err := term.GetSize(fd)
	if err != nil {
		cols, rows = 80, 24
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var conn *websocket.Conn
	if resumeSessionID != "" {
		conn, err = client.DialShellResume(ctx, agentID, resumeSessionID, cols, rows)
	} else {
		conn, err = client.DialShell(ctx, agentID, cols, rows)
	}
	if err != nil {
		return nil, err
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// For new sessions, read the initial session message from the server.
	// The server sends {"type":"session","sessionId":"..."} as the first message.
	sessionID := resumeSessionID
	if sessionID == "" {
		readCtx, readCancel := context.WithTimeout(ctx, 5*1e9) // 5s
		_, initData, readErr := conn.Read(readCtx)
		readCancel()
		if readErr == nil {
			var msg struct {
				Type      string `json:"type"`
				SessionID string `json:"sessionId"`
			}
			if json.Unmarshal(initData, &msg) == nil && msg.Type == "session" {
				sessionID = msg.SessionID
			}
		}
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("entering raw mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	// Show subtle connect hint
	fmt.Fprintf(os.Stdout, "\r\n\x1b[90m── Conduit ── Ctrl+] for commands ──\x1b[0m\r\n\r\n")

	done := make(chan struct{}, 1)
	detached := false

	// stdin → WebSocket (with Ctrl+] command prefix)
	go func() {
		buf := make([]byte, 4096)
		const ctrlRightBracket = 0x1D // Ctrl+]
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				cancel()
				return
			}

			out := make([]byte, 0, n)
			for i := 0; i < n; i++ {
				b := buf[i]
				if b == ctrlRightBracket {
					// Show command bar and wait for action
					fmt.Fprintf(os.Stdout, "\r\n\x1b[7m Conduit \x1b[0m \x1b[97md\x1b[90m detach  \x1b[97mp\x1b[90m pin  \x1b[97mEsc\x1b[90m cancel\x1b[0m ")

					// Read next byte for command
					cmdBuf := make([]byte, 1)
					if _, err := os.Stdin.Read(cmdBuf); err != nil {
						cancel()
						return
					}

					switch cmdBuf[0] {
					case 'd':
						detached = true
						fmt.Fprintf(os.Stdout, "\r\n\x1b[90mDetached.\x1b[0m\r\n")
						cancel()
						return
					case 'p':
						fmt.Fprintf(os.Stdout, "\r\n")
						go togglePin(client, agentID, sessionID)
					default:
						// Esc or any other key — cancel, clear the bar
						fmt.Fprintf(os.Stdout, "\r\x1b[2K")
					}
					continue
				}
				out = append(out, b)
			}

			if len(out) > 0 {
				if err := conn.Write(ctx, websocket.MessageBinary, out); err != nil {
					cancel()
					return
				}
			}
		}
	}()

	// WebSocket → stdout
	go func() {
		defer func() {
			select {
			case done <- struct{}{}:
			default:
			}
		}()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			os.Stdout.Write(data)
		}
	}()

	// Terminal resize watcher (platform-specific)
	go watchResize(ctx, conn, fd)

	// Wait for shell to exit or user detach
	select {
	case <-done:
	case <-ctx.Done():
	}

	return &ShellResult{SessionID: sessionID, Detached: detached}, nil
}

// togglePin sends a PATCH to toggle the pinned state of a session.
func togglePin(client *Client, agentID, sessionID string) {
	if sessionID == "" {
		return
	}
	pinned, err := client.ToggleSessionPin(agentID, sessionID)
	if err != nil {
		fmt.Fprintf(os.Stdout, "\r\n[pin error: %v]\r\n", err)
		return
	}
	if pinned {
		fmt.Fprintf(os.Stdout, "\r\n[session pinned]\r\n")
	} else {
		fmt.Fprintf(os.Stdout, "\r\n[session unpinned]\r\n")
	}
}

// sendResize sends a JSON resize message over the WebSocket.
func sendResize(ctx context.Context, conn *websocket.Conn, cols, rows int) {
	msg, _ := json.Marshal(map[string]interface{}{
		"type": "resize",
		"cols": cols,
		"rows": rows,
	})
	conn.Write(ctx, websocket.MessageText, msg)
}
