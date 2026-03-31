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

// FileListRequest is sent by the server to request a directory listing.
type FileListRequest struct {
	Path       string `json:"path"`
	ShowHidden bool   `json:"showHidden,omitempty"`
}

// FileEntry represents a single file or directory in a listing.
type FileEntry struct {
	Name       string `json:"name"`
	Type       string `json:"type"` // "file", "directory", "symlink"
	Size       int64  `json:"size"`
	Permissions string `json:"permissions,omitempty"` // e.g. "-rw-r--r--"
	Owner      string `json:"owner,omitempty"`
	Group      string `json:"group,omitempty"`
	ModifiedAt string `json:"modifiedAt"`
	IsHidden   bool   `json:"isHidden,omitempty"`
}

// FileListResponse is the response to a FILE_LIST request.
type FileListResponse struct {
	Path    string      `json:"path"`
	Entries []FileEntry `json:"entries"`
	Error   string      `json:"error,omitempty"`
}

// FileReadRequest is sent by the server to read a file.
type FileReadRequest struct {
	Path     string `json:"path"`
	MaxBytes int64  `json:"maxBytes,omitempty"` // 0 = entire file
	Offset   int64  `json:"offset,omitempty"`
}

// FileReadResponse is the metadata response for a FILE_READ.
// Actual file content follows as raw payload bytes in subsequent frames on the same stream,
// or is included inline for small files.
type FileReadResponse struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	MimeType  string `json:"mimeType,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	Content   string `json:"content,omitempty"` // base64 for binary, raw UTF-8 for text
	Error     string `json:"error,omitempty"`
}

// FileWriteRequest is sent by the server to write a file.
type FileWriteRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"` // base64-encoded file content
	Mode    string `json:"mode,omitempty"` // e.g. "0644", defaults to 0644
}

// FileWriteResponse is the response to a FILE_WRITE.
type FileWriteResponse struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum,omitempty"` // SHA-256
	Error    string `json:"error,omitempty"`
}

// FileStatRequest requests metadata about a file or directory.
type FileStatRequest struct {
	Path string `json:"path"`
}

// FileStatResponse is the response to a FILE_STAT request.
type FileStatResponse struct {
	Path        string `json:"path"`
	Type        string `json:"type"` // "file", "directory", "symlink"
	Size        int64  `json:"size"`
	Permissions string `json:"permissions,omitempty"`
	Owner       string `json:"owner,omitempty"`
	Group       string `json:"group,omitempty"`
	ModifiedAt  string `json:"modifiedAt"`
	Error       string `json:"error,omitempty"`
}

// FileDeleteRequest is sent by the server to delete a file or directory.
type FileDeleteRequest struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive,omitempty"`
}

// FileDeleteResponse is the response to a FILE_DELETE.
type FileDeleteResponse struct {
	Path  string `json:"path"`
	Error string `json:"error,omitempty"`
}

// FileRenameRequest is sent by the server to rename/move a file.
type FileRenameRequest struct {
	OldPath string `json:"oldPath"`
	NewPath string `json:"newPath"`
}

// FileRenameResponse is the response to a rename operation.
type FileRenameResponse struct {
	OldPath string `json:"oldPath"`
	NewPath string `json:"newPath"`
	Error   string `json:"error,omitempty"`
}

// FileMkdirRequest is sent by the server to create a directory.
type FileMkdirRequest struct {
	Path    string `json:"path"`
	Parents bool   `json:"parents,omitempty"` // mkdir -p
}

// FileMkdirResponse is the response to a mkdir operation.
type FileMkdirResponse struct {
	Path  string `json:"path"`
	Error string `json:"error,omitempty"`
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
