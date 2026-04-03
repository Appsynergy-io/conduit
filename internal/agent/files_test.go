//go:build linux || darwin

package agent

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/appsynergy-io/conduit/internal/protocol"
)

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/var/log", "/var/log"},
		{"/var/log/../etc/passwd", "/var/etc/passwd"},
		{"../etc/passwd", "/etc/passwd"},
		{"/var/log/./syslog", "/var/log/syslog"},
		{"relative/path", "/relative/path"},
		{"/", "/"},
		{".", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, sanitizePath(tt.input))
		})
	}
}

// setupTestMux creates a mock server and returns a connected mux for testing file ops.
func setupTestMux(t *testing.T) (*Agent, *protocol.Mux, chan *protocol.Frame) {
	t.Helper()
	received := make(chan *protocol.Frame, 64)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{"conduit-cwp-v1"},
		})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		ctx := r.Context()
		conn.SetReadLimit(protocol.MaxPayloadSize + protocol.HeaderSize)

		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			frame, err := protocol.DecodeBytes(data)
			if err != nil {
				continue
			}
			received <- frame
		}
	}))
	t.Cleanup(srv.Close)

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	a := &Agent{
		cfg: &Config{
			ServerURL: srv.URL,
			AgentID:   "test-agent",
			AgentKey:  "unused",
			TenantID:  "test-tenant",
		},
		logger: logger,
		shells: make(map[uint32]*shellSession),
	}

	ctx := context.Background()
	wsURL := a.buildWSURL()
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		Subprotocols: []string{"conduit-cwp-v1"},
	})
	require.NoError(t, err)

	conn.SetReadLimit(protocol.MaxPayloadSize + protocol.HeaderSize)
	mux := protocol.NewMux(conn, logger)
	go mux.ReadLoop(ctx)

	t.Cleanup(func() { mux.Close() })

	return a, mux, received
}

func TestHandleFileList(t *testing.T) {
	a, mux, received := setupTestMux(t)
	ctx := context.Background()

	// Create a test directory with files
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("hello"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".hidden"), []byte("secret"), 0644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0755))

	// Send FILE_LIST request
	req := protocol.FileListRequest{Path: dir, ShowHidden: false}
	f, err := protocol.NewFrame(protocol.FrameFileList, 1, req)
	require.NoError(t, err)

	a.handleFileList(ctx, mux, f)

	// Read the response
	select {
	case resp := <-received:
		assert.Equal(t, protocol.FrameFileList, resp.Type)
		var listResp protocol.FileListResponse
		require.NoError(t, protocol.UnmarshalPayload(resp.Payload, &listResp))
		assert.Equal(t, dir, listResp.Path)
		assert.Empty(t, listResp.Error)

		// Should have file1.txt and subdir (hidden file excluded)
		names := make(map[string]string)
		for _, e := range listResp.Entries {
			names[e.Name] = e.Type
		}
		assert.Equal(t, "file", names["file1.txt"])
		assert.Equal(t, "directory", names["subdir"])
		assert.NotContains(t, names, ".hidden")

	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for FILE_LIST response")
	}
}

func TestHandleFileList_ShowHidden(t *testing.T) {
	a, mux, received := setupTestMux(t)
	ctx := context.Background()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".hidden"), []byte("hi"), 0644))

	req := protocol.FileListRequest{Path: dir, ShowHidden: true}
	f, _ := protocol.NewFrame(protocol.FrameFileList, 1, req)
	a.handleFileList(ctx, mux, f)

	select {
	case resp := <-received:
		var listResp protocol.FileListResponse
		require.NoError(t, protocol.UnmarshalPayload(resp.Payload, &listResp))
		assert.Len(t, listResp.Entries, 1)
		assert.Equal(t, ".hidden", listResp.Entries[0].Name)
		assert.True(t, listResp.Entries[0].IsHidden)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}

func TestHandleFileList_NotFound(t *testing.T) {
	a, mux, received := setupTestMux(t)
	ctx := context.Background()

	req := protocol.FileListRequest{Path: "/nonexistent/dir/12345"}
	f, _ := protocol.NewFrame(protocol.FrameFileList, 1, req)
	a.handleFileList(ctx, mux, f)

	select {
	case resp := <-received:
		var listResp protocol.FileListResponse
		require.NoError(t, protocol.UnmarshalPayload(resp.Payload, &listResp))
		assert.NotEmpty(t, listResp.Error)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}

func TestHandleFileRead(t *testing.T) {
	a, mux, received := setupTestMux(t)
	ctx := context.Background()

	dir := t.TempDir()
	content := "Hello, file content!"
	path := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	req := protocol.FileReadRequest{Path: path}
	f, _ := protocol.NewFrame(protocol.FrameFileRead, 1, req)
	a.handleFileRead(ctx, mux, f)

	select {
	case resp := <-received:
		var readResp protocol.FileReadResponse
		require.NoError(t, protocol.UnmarshalPayload(resp.Payload, &readResp))
		assert.Empty(t, readResp.Error)
		assert.Equal(t, int64(len(content)), readResp.Size)
		assert.False(t, readResp.Truncated)

		decoded, err := base64.StdEncoding.DecodeString(readResp.Content)
		require.NoError(t, err)
		assert.Equal(t, content, string(decoded))
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}

func TestHandleFileWrite(t *testing.T) {
	a, mux, received := setupTestMux(t)
	ctx := context.Background()

	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")
	content := "Written via CWP"

	req := protocol.FileWriteRequest{
		Path:    path,
		Content: base64.StdEncoding.EncodeToString([]byte(content)),
	}
	f, _ := protocol.NewFrame(protocol.FrameFileWrite, 1, req)
	a.handleFileWrite(ctx, mux, f)

	select {
	case resp := <-received:
		var writeResp protocol.FileWriteResponse
		require.NoError(t, protocol.UnmarshalPayload(resp.Payload, &writeResp))
		assert.Empty(t, writeResp.Error)
		assert.Equal(t, int64(len(content)), writeResp.Size)
		assert.NotEmpty(t, writeResp.Checksum)

		// Verify file was actually written
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, content, string(data))
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}

func TestHandleFileStat(t *testing.T) {
	a, mux, received := setupTestMux(t)
	ctx := context.Background()

	dir := t.TempDir()
	path := filepath.Join(dir, "stat.txt")
	require.NoError(t, os.WriteFile(path, []byte("stat test"), 0644))

	req := protocol.FileStatRequest{Path: path}
	f, _ := protocol.NewFrame(protocol.FrameFileStat, 1, req)
	a.handleFileStat(ctx, mux, f)

	select {
	case resp := <-received:
		var statResp protocol.FileStatResponse
		require.NoError(t, protocol.UnmarshalPayload(resp.Payload, &statResp))
		assert.Empty(t, statResp.Error)
		assert.Equal(t, "file", statResp.Type)
		assert.Equal(t, int64(9), statResp.Size)
		assert.NotEmpty(t, statResp.Permissions)
		assert.NotEmpty(t, statResp.ModifiedAt)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}

func TestHandleFileDelete(t *testing.T) {
	a, mux, received := setupTestMux(t)
	ctx := context.Background()

	dir := t.TempDir()
	path := filepath.Join(dir, "delete_me.txt")
	require.NoError(t, os.WriteFile(path, []byte("bye"), 0644))

	req := protocol.FileDeleteRequest{Path: path}
	f, _ := protocol.NewFrame(protocol.FrameFileDelete, 1, req)
	a.handleFileDelete(ctx, mux, f)

	select {
	case resp := <-received:
		var delResp protocol.FileDeleteResponse
		require.NoError(t, protocol.UnmarshalPayload(resp.Payload, &delResp))
		assert.Empty(t, delResp.Error)

		_, err := os.Stat(path)
		assert.True(t, os.IsNotExist(err))
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}

func TestHandleFileDelete_SystemDir(t *testing.T) {
	a, mux, received := setupTestMux(t)
	ctx := context.Background()

	// Try to delete root — should be refused
	req := protocol.FileDeleteRequest{Path: "/"}
	f, _ := protocol.NewFrame(protocol.FrameFileDelete, 1, req)
	a.handleFileDelete(ctx, mux, f)

	select {
	case resp := <-received:
		var delResp protocol.FileDeleteResponse
		require.NoError(t, protocol.UnmarshalPayload(resp.Payload, &delResp))
		assert.Contains(t, delResp.Error, "refusing")
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}

func TestHandleFileRename(t *testing.T) {
	a, mux, received := setupTestMux(t)
	ctx := context.Background()

	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.txt")
	newPath := filepath.Join(dir, "new.txt")
	require.NoError(t, os.WriteFile(oldPath, []byte("rename me"), 0644))

	req := protocol.FileRenameRequest{OldPath: oldPath, NewPath: newPath}
	f, _ := protocol.NewFrame(protocol.FrameFileRename, 1, req)
	a.handleFileRename(ctx, mux, f)

	select {
	case resp := <-received:
		var renameResp protocol.FileRenameResponse
		require.NoError(t, protocol.UnmarshalPayload(resp.Payload, &renameResp))
		assert.Empty(t, renameResp.Error)

		_, err := os.Stat(oldPath)
		assert.True(t, os.IsNotExist(err))
		data, err := os.ReadFile(newPath)
		require.NoError(t, err)
		assert.Equal(t, "rename me", string(data))
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}

func TestHandleFileMkdir(t *testing.T) {
	a, mux, received := setupTestMux(t)
	ctx := context.Background()

	dir := t.TempDir()
	newDir := filepath.Join(dir, "a", "b", "c")

	req := protocol.FileMkdirRequest{Path: newDir, Parents: true}
	f, _ := protocol.NewFrame(protocol.FrameFileMkdir, 1, req)
	a.handleFileMkdir(ctx, mux, f)

	select {
	case resp := <-received:
		var mkdirResp protocol.FileMkdirResponse
		require.NoError(t, protocol.UnmarshalPayload(resp.Payload, &mkdirResp))
		assert.Empty(t, mkdirResp.Error)

		info, err := os.Stat(newDir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}
