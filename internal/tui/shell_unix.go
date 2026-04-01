//go:build !windows

package tui

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/coder/websocket"
	"golang.org/x/term"
)

// watchResize monitors terminal size changes and sends resize messages.
func watchResize(ctx context.Context, conn *websocket.Conn, fd int) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	for {
		select {
		case <-sigCh:
			cols, rows, err := term.GetSize(fd)
			if err != nil {
				continue
			}
			sendResize(ctx, conn, cols, rows)
		case <-ctx.Done():
			return
		}
	}
}
