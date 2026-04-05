package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/appsynergy-io/conduit/internal/protocol"
)

func TestParseBrowserResize_Valid(t *testing.T) {
	data := []byte(`{"type":"resize","cols":120,"rows":40}`)
	f := parseBrowserResize(data, 5)
	require.NotNil(t, f)
	assert.Equal(t, protocol.FrameShellResize, f.Type)
	assert.Equal(t, uint32(5), f.StreamID)

	var payload protocol.ShellResizePayload
	require.NoError(t, protocol.UnmarshalPayload(f.Payload, &payload))
	assert.Equal(t, 120, payload.Cols)
	assert.Equal(t, 40, payload.Rows)
}

func TestParseBrowserResize_TerminalData(t *testing.T) {
	// Regular terminal data should not be treated as resize
	data := []byte("ls -la\r\n")
	f := parseBrowserResize(data, 5)
	assert.Nil(t, f)
}

func TestParseBrowserResize_OtherJSON(t *testing.T) {
	// JSON but not a resize message
	data := []byte(`{"type":"other","value":42}`)
	f := parseBrowserResize(data, 5)
	assert.Nil(t, f)
}

func TestParseBrowserResize_InvalidCols(t *testing.T) {
	data := []byte(`{"type":"resize","cols":0,"rows":40}`)
	f := parseBrowserResize(data, 5)
	assert.Nil(t, f)
}

func TestParseBrowserResize_Empty(t *testing.T) {
	f := parseBrowserResize(nil, 5)
	assert.Nil(t, f)

	f = parseBrowserResize([]byte{}, 5)
	assert.Nil(t, f)
}

func TestParseBrowserResize_InvalidJSON(t *testing.T) {
	data := []byte(`{broken json`)
	f := parseBrowserResize(data, 5)
	assert.Nil(t, f)
}

func TestParseResizeDims(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		wantCols int
		wantRows int
		wantOK   bool
	}{
		{"valid", []byte(`{"type":"resize","cols":120,"rows":40}`), 120, 40, true},
		{"not resize", []byte(`{"type":"other"}`), 0, 0, false},
		{"terminal data", []byte("ls -la"), 0, 0, false},
		{"empty", []byte{}, 0, 0, false},
		{"invalid json", []byte(`{broken`), 0, 0, false},
		{"zero cols", []byte(`{"type":"resize","cols":0,"rows":40}`), 0, 0, false},
		{"zero rows", []byte(`{"type":"resize","cols":80,"rows":0}`), 0, 0, false},
		{"over bound cols", []byte(`{"type":"resize","cols":501,"rows":40}`), 0, 0, false},
		{"over bound rows", []byte(`{"type":"resize","cols":80,"rows":501}`), 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cols, rows, ok := parseResizeDims(tt.data)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantCols, cols)
			assert.Equal(t, tt.wantRows, rows)
		})
	}
}

func TestLiveSession_SizeTracking(t *testing.T) {
	ls := &LiveSession{cols: 80, rows: 24}
	cols, rows := ls.Size()
	assert.Equal(t, 80, cols)
	assert.Equal(t, 24, rows)

	ls.SetSize(120, 40)
	cols, rows = ls.Size()
	assert.Equal(t, 120, cols)
	assert.Equal(t, 40, rows)
}
