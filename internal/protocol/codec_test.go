package protocol

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		frame Frame
	}{
		{
			name:  "empty payload",
			frame: Frame{Type: FramePing, StreamID: 0, Payload: nil},
		},
		{
			name:  "small payload",
			frame: Frame{Type: FrameShellData, StreamID: 42, Payload: []byte("hello world")},
		},
		{
			name:  "large stream ID",
			frame: Frame{Type: FrameAuth, StreamID: 0xFFFFFFFF, Payload: []byte(`{"agentId":"abc"}`)},
		},
		{
			name:  "hello frame",
			frame: Frame{Type: FrameHello, StreamID: 1, Payload: []byte(`{"agentId":"a1","hostname":"web-01","os":"linux","arch":"amd64","version":"0.1.0"}`)},
		},
		{
			name:  "pong frame",
			frame: Frame{Type: FramePong, StreamID: 0, Payload: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			n, err := Encode(&buf, &tt.frame)
			require.NoError(t, err)
			assert.Equal(t, HeaderSize+len(tt.frame.Payload), n)

			decoded, err := Decode(&buf)
			require.NoError(t, err)
			assert.Equal(t, tt.frame.Type, decoded.Type)
			assert.Equal(t, tt.frame.StreamID, decoded.StreamID)
			assert.Equal(t, tt.frame.Payload, decoded.Payload)
		})
	}
}

func TestEncodeBytesDecodeBytesRoundTrip(t *testing.T) {
	original := Frame{
		Type:     FrameExecData,
		StreamID: 99,
		Payload:  []byte("some exec output"),
	}

	data, err := EncodeBytes(&original)
	require.NoError(t, err)
	assert.Len(t, data, HeaderSize+len(original.Payload))

	decoded, err := DecodeBytes(data)
	require.NoError(t, err)
	assert.Equal(t, original.Type, decoded.Type)
	assert.Equal(t, original.StreamID, decoded.StreamID)
	assert.Equal(t, original.Payload, decoded.Payload)
}

func TestDecodeBytes_TooShort(t *testing.T) {
	_, err := DecodeBytes([]byte{0x01, 0x02})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too short")
}

func TestDecodeBytes_PayloadTooLarge(t *testing.T) {
	// Craft a header claiming a payload larger than MaxPayloadSize
	data := make([]byte, HeaderSize)
	data[0] = byte(FrameShellData)
	// Set payload length to MaxPayloadSize + 1
	data[5] = 0x01
	data[6] = 0x00
	data[7] = 0x00
	data[8] = 0x01

	_, err := DecodeBytes(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")
}

func TestDecodeBytes_TruncatedPayload(t *testing.T) {
	// Header says 10 bytes payload but only 5 provided
	data := make([]byte, HeaderSize+5)
	data[0] = byte(FrameShellData)
	data[5] = 0x00
	data[6] = 0x00
	data[7] = 0x00
	data[8] = 0x0A // 10 bytes

	_, err := DecodeBytes(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too short")
}

func TestEncode_PayloadTooLarge(t *testing.T) {
	f := &Frame{
		Type:    FrameShellData,
		Payload: make([]byte, MaxPayloadSize+1),
	}
	var buf bytes.Buffer
	_, err := Encode(&buf, f)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")
}

func TestDecode_PayloadTooLarge(t *testing.T) {
	// Write a valid header with an oversized payload length
	var buf bytes.Buffer
	header := make([]byte, HeaderSize)
	header[0] = byte(FrameShellData)
	header[5] = 0x01
	header[6] = 0x00
	header[7] = 0x00
	header[8] = 0x01
	buf.Write(header)

	_, err := Decode(&buf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")
}

func TestMultipleFrames(t *testing.T) {
	frames := []Frame{
		{Type: FrameHello, StreamID: 1, Payload: []byte("hello")},
		{Type: FrameAuth, StreamID: 1, Payload: []byte("auth")},
		{Type: FrameShellData, StreamID: 2, Payload: []byte("data")},
		{Type: FramePing, StreamID: 0, Payload: nil},
	}

	var buf bytes.Buffer
	for _, f := range frames {
		_, err := Encode(&buf, &f)
		require.NoError(t, err)
	}

	for _, expected := range frames {
		decoded, err := Decode(&buf)
		require.NoError(t, err)
		assert.Equal(t, expected.Type, decoded.Type)
		assert.Equal(t, expected.StreamID, decoded.StreamID)
		assert.Equal(t, expected.Payload, decoded.Payload)
	}
}

func TestFrameTypeString(t *testing.T) {
	assert.Equal(t, "HELLO", FrameHello.String())
	assert.Equal(t, "SHELL_DATA", FrameShellData.String())
	assert.Equal(t, "PING", FramePing.String())
	assert.Equal(t, "EXEC_EXIT", FrameExecExit.String())
	assert.Equal(t, "UNKNOWN", FrameType(0xFF).String())
}
