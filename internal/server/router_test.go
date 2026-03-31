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
