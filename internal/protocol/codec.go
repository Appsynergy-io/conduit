package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Encode serializes a Frame into its binary wire format and writes it to w.
// Returns the total number of bytes written (header + payload).
func Encode(w io.Writer, f *Frame) (int, error) {
	if len(f.Payload) > MaxPayloadSize {
		return 0, fmt.Errorf("payload size %d exceeds maximum %d", len(f.Payload), MaxPayloadSize)
	}

	var header [HeaderSize]byte
	header[0] = byte(f.Type)
	binary.BigEndian.PutUint32(header[1:5], f.StreamID)
	binary.BigEndian.PutUint32(header[5:9], uint32(len(f.Payload)))

	n, err := w.Write(header[:])
	if err != nil {
		return n, fmt.Errorf("writing frame header: %w", err)
	}

	if len(f.Payload) > 0 {
		pn, err := w.Write(f.Payload)
		n += pn
		if err != nil {
			return n, fmt.Errorf("writing frame payload: %w", err)
		}
	}

	return n, nil
}

// Decode reads a single Frame from r.
// It reads the fixed-size header first, then the variable-length payload.
func Decode(r io.Reader) (*Frame, error) {
	var header [HeaderSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, fmt.Errorf("reading frame header: %w", err)
	}

	f := &Frame{
		Type:     FrameType(header[0]),
		StreamID: binary.BigEndian.Uint32(header[1:5]),
	}

	payloadLen := binary.BigEndian.Uint32(header[5:9])
	if payloadLen > MaxPayloadSize {
		return nil, fmt.Errorf("payload size %d exceeds maximum %d", payloadLen, MaxPayloadSize)
	}

	if payloadLen > 0 {
		f.Payload = make([]byte, payloadLen)
		if _, err := io.ReadFull(r, f.Payload); err != nil {
			return nil, fmt.Errorf("reading frame payload: %w", err)
		}
	}

	return f, nil
}

// EncodeBytes serializes a Frame into a byte slice.
// Useful for WebSocket messages where the entire frame is a single message.
func EncodeBytes(f *Frame) ([]byte, error) {
	if len(f.Payload) > MaxPayloadSize {
		return nil, fmt.Errorf("payload size %d exceeds maximum %d", len(f.Payload), MaxPayloadSize)
	}

	buf := make([]byte, HeaderSize+len(f.Payload))
	buf[0] = byte(f.Type)
	binary.BigEndian.PutUint32(buf[1:5], f.StreamID)
	binary.BigEndian.PutUint32(buf[5:9], uint32(len(f.Payload)))
	copy(buf[HeaderSize:], f.Payload)
	return buf, nil
}

// DecodeBytes deserializes a Frame from a byte slice.
// Useful for WebSocket messages where the entire frame is a single message.
func DecodeBytes(data []byte) (*Frame, error) {
	if len(data) < HeaderSize {
		return nil, fmt.Errorf("data too short: %d bytes, need at least %d", len(data), HeaderSize)
	}

	f := &Frame{
		Type:     FrameType(data[0]),
		StreamID: binary.BigEndian.Uint32(data[1:5]),
	}

	payloadLen := binary.BigEndian.Uint32(data[5:9])
	if payloadLen > MaxPayloadSize {
		return nil, fmt.Errorf("payload size %d exceeds maximum %d", payloadLen, MaxPayloadSize)
	}

	expected := HeaderSize + int(payloadLen)
	if len(data) < expected {
		return nil, fmt.Errorf("data too short: %d bytes, need %d", len(data), expected)
	}

	if payloadLen > 0 {
		f.Payload = make([]byte, payloadLen)
		copy(f.Payload, data[HeaderSize:expected])
	}

	return f, nil
}
