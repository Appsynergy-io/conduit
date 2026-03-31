// Package protocol implements the Conduit Wire Protocol (CWP).
//
// CWP is a binary frame format used for all agent-server communication.
// The same framing is used over both QUIC streams and WebSocket messages.
//
// Frame layout:
//
//	┌──────────┬──────────┬───────────┬──────────────────┐
//	│ Type (1B)│ StreamID │ Length    │ Payload          │
//	│          │  (4B)    │  (4B)    │ (variable)       │
//	└──────────┴──────────┴───────────┴──────────────────┘
package protocol

// FrameType identifies the kind of CWP frame.
type FrameType uint8

const (
	// Control frames
	FrameHello       FrameType = 0x01 // Agent → Server: announces identity
	FrameAuth        FrameType = 0x02 // Both: HMAC auth challenge/response
	FrameAuthOK      FrameType = 0x03 // Server → Agent: auth accepted
	FrameAuthReject  FrameType = 0x04 // Server → Agent: auth rejected
	FramePing        FrameType = 0xF0 // Both: keepalive
	FramePong        FrameType = 0xF1 // Both: keepalive response

	// Shell frames
	FrameShellData   FrameType = 0x10 // Both: PTY stdin/stdout bytes
	FrameShellResize FrameType = 0x11 // Server → Agent: terminal resize
	FrameShellStart  FrameType = 0x12 // Server → Agent: request new shell
	FrameShellExit   FrameType = 0x13 // Agent → Server: shell exited

	// File frames
	FrameFileList    FrameType = 0x20 // Both: directory listing req/resp
	FrameFileRead    FrameType = 0x21 // Both: file content req/resp
	FrameFileWrite   FrameType = 0x22 // Both: write file req/resp
	FrameFileStat    FrameType = 0x23 // Both: file metadata req/resp
	FrameFileDelete  FrameType = 0x24 // Both: delete file/dir req/resp
	FrameFileRename  FrameType = 0x25 // Both: rename/move req/resp
	FrameFileMkdir   FrameType = 0x26 // Both: create directory req/resp

	// Agent info
	FrameAgentInfo   FrameType = 0x30 // Agent → Server: system metrics

	// Exec frames
	FrameExecStart   FrameType = 0x40 // Server → Agent: start command
	FrameExecData    FrameType = 0x41 // Agent → Server: stdout/stderr
	FrameExecExit    FrameType = 0x42 // Agent → Server: process exited
)

// HeaderSize is the fixed size of a CWP frame header in bytes.
const HeaderSize = 9 // 1 (type) + 4 (stream ID) + 4 (length)

// MaxPayloadSize is the maximum allowed payload size (16 MB).
const MaxPayloadSize = 16 << 20

// Frame is a single CWP protocol frame.
type Frame struct {
	Type     FrameType
	StreamID uint32
	Payload  []byte
}

// String returns the human-readable name of a frame type.
func (ft FrameType) String() string {
	switch ft {
	case FrameHello:
		return "HELLO"
	case FrameAuth:
		return "AUTH"
	case FrameAuthOK:
		return "AUTH_OK"
	case FrameAuthReject:
		return "AUTH_REJECT"
	case FramePing:
		return "PING"
	case FramePong:
		return "PONG"
	case FrameShellData:
		return "SHELL_DATA"
	case FrameShellResize:
		return "SHELL_RESIZE"
	case FrameShellStart:
		return "SHELL_START"
	case FrameShellExit:
		return "SHELL_EXIT"
	case FrameFileList:
		return "FILE_LIST"
	case FrameFileRead:
		return "FILE_READ"
	case FrameFileWrite:
		return "FILE_WRITE"
	case FrameFileStat:
		return "FILE_STAT"
	case FrameFileDelete:
		return "FILE_DELETE"
	case FrameFileRename:
		return "FILE_RENAME"
	case FrameFileMkdir:
		return "FILE_MKDIR"
	case FrameAgentInfo:
		return "AGENT_INFO"
	case FrameExecStart:
		return "EXEC_START"
	case FrameExecData:
		return "EXEC_DATA"
	case FrameExecExit:
		return "EXEC_EXIT"
	default:
		return "UNKNOWN"
	}
}
