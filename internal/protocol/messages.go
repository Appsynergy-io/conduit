package protocol

import (
	"encoding/json"
	"fmt"
)

// HelloPayload is sent by the agent in the HELLO frame to announce itself.
type HelloPayload struct {
	AgentID  string `json:"agentId"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Version  string `json:"version"`
}

// AuthPayload is sent by the agent in the AUTH frame.
type AuthPayload struct {
	AgentID   string `json:"agentId"`
	Signature string `json:"signature"` // HMAC-SHA256 of HELLO payload using agent key
	Nonce     string `json:"nonce"`     // Unique per-connection nonce
}

// ShellStartPayload requests a new shell session on the agent.
type ShellStartPayload struct {
	SessionID string `json:"sessionId"`
	Shell     string `json:"shell,omitempty"` // e.g. "/bin/bash", empty = default
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
}

// ShellResizePayload requests a terminal resize.
type ShellResizePayload struct {
	SessionID string `json:"sessionId"`
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
}

// ShellExitPayload reports that a shell session has ended.
type ShellExitPayload struct {
	SessionID string `json:"sessionId"`
	ExitCode  int    `json:"exitCode"`
}

// ExecStartPayload requests command execution on the agent.
type ExecStartPayload struct {
	JobID   string `json:"jobId"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"` // seconds, 0 = default
}

// ExecExitPayload reports that an exec command has finished.
type ExecExitPayload struct {
	JobID    string `json:"jobId"`
	ExitCode int    `json:"exitCode"`
}

// AgentInfoPayload carries system metrics from the agent.
type AgentInfoPayload struct {
	CPUPercent  float64 `json:"cpuPercent"`
	MemTotal    uint64  `json:"memTotal"`
	MemUsed     uint64  `json:"memUsed"`
	DiskTotal   uint64  `json:"diskTotal"`
	DiskUsed    uint64  `json:"diskUsed"`
	Uptime      int64   `json:"uptime"` // seconds
	LoadAvg1    float64 `json:"loadAvg1,omitempty"`
	LoadAvg5    float64 `json:"loadAvg5,omitempty"`
	LoadAvg15   float64 `json:"loadAvg15,omitempty"`
}

// MarshalPayload encodes a structured payload as JSON bytes for a Frame.
func MarshalPayload(v interface{}) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshaling payload: %w", err)
	}
	return b, nil
}

// UnmarshalPayload decodes a Frame's JSON payload into a structured type.
func UnmarshalPayload(data []byte, v interface{}) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("unmarshaling payload: %w", err)
	}
	return nil
}

// NewFrame is a convenience constructor for creating a Frame with a JSON payload.
func NewFrame(ft FrameType, streamID uint32, payload interface{}) (*Frame, error) {
	f := &Frame{
		Type:     ft,
		StreamID: streamID,
	}
	if payload != nil {
		b, err := MarshalPayload(payload)
		if err != nil {
			return nil, err
		}
		f.Payload = b
	}
	return f, nil
}
