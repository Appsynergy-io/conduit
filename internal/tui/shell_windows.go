//go:build windows

package tui

import (
	"context"

	"github.com/coder/websocket"
)

// watchResize is a no-op on Windows (SIGWINCH not supported).
func watchResize(ctx context.Context, _ *websocket.Conn, _ int) {
	<-ctx.Done()
}
