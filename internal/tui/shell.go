package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/coder/websocket"
	"golang.org/x/term"
)

// RunShell connects to an agent shell via WebSocket and bridges stdin/stdout.
// It takes over the terminal in raw mode. Returns when the session ends.
func RunShell(client *Client, agentID string) error {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return fmt.Errorf("not a terminal")
	}

	cols, rows, err := term.GetSize(fd)
	if err != nil {
		cols, rows = 80, 24
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, err := client.DialShell(ctx, agentID, cols, rows)
	if err != nil {
		return err
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("entering raw mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	done := make(chan struct{}, 1)

	// stdin → WebSocket
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				cancel()
				return
			}
			if err := conn.Write(ctx, websocket.MessageBinary, buf[:n]); err != nil {
				cancel()
				return
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

	// Wait for shell to exit (WebSocket close from server)
	select {
	case <-done:
	case <-ctx.Done():
	}

	return nil
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
