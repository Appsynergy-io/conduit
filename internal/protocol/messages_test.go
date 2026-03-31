package protocol

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalUnmarshalPayload(t *testing.T) {
	original := HelloPayload{
		AgentID:  "agent-123",
		Hostname: "web-01",
		OS:       "linux",
		Arch:     "amd64",
		Version:  "0.1.0",
	}

	data, err := MarshalPayload(original)
	require.NoError(t, err)

	var decoded HelloPayload
	require.NoError(t, UnmarshalPayload(data, &decoded))

	assert.Equal(t, original, decoded)
}

func TestNewFrame(t *testing.T) {
	payload := ShellStartPayload{
		SessionID: "sess-1",
		Shell:     "/bin/bash",
		Cols:      80,
		Rows:      24,
	}

	f, err := NewFrame(FrameShellStart, 5, payload)
	require.NoError(t, err)

	assert.Equal(t, FrameShellStart, f.Type)
	assert.Equal(t, uint32(5), f.StreamID)
	assert.NotEmpty(t, f.Payload)

	var decoded ShellStartPayload
	require.NoError(t, UnmarshalPayload(f.Payload, &decoded))
	assert.Equal(t, payload, decoded)
}

func TestNewFrame_NilPayload(t *testing.T) {
	f, err := NewFrame(FramePing, 0, nil)
	require.NoError(t, err)
	assert.Equal(t, FramePing, f.Type)
	assert.Nil(t, f.Payload)
}

func TestAgentInfoPayload(t *testing.T) {
	info := AgentInfoPayload{
		CPUPercent: 45.5,
		MemTotal:   8589934592,
		MemUsed:    4294967296,
		DiskTotal:  107374182400,
		DiskUsed:   53687091200,
		Uptime:     86400,
		LoadAvg1:   1.5,
		LoadAvg5:   1.2,
		LoadAvg15:  0.9,
	}

	data, err := MarshalPayload(info)
	require.NoError(t, err)

	var decoded AgentInfoPayload
	require.NoError(t, UnmarshalPayload(data, &decoded))
	assert.Equal(t, info, decoded)
}

func TestExecPayloads(t *testing.T) {
	start := ExecStartPayload{
		JobID:   "job-1",
		Command: "uptime",
		Timeout: 30,
	}

	data, err := MarshalPayload(start)
	require.NoError(t, err)

	var decoded ExecStartPayload
	require.NoError(t, UnmarshalPayload(data, &decoded))
	assert.Equal(t, start, decoded)

	exit := ExecExitPayload{JobID: "job-1", ExitCode: 0}
	data, err = MarshalPayload(exit)
	require.NoError(t, err)

	var decodedExit ExecExitPayload
	require.NoError(t, UnmarshalPayload(data, &decodedExit))
	assert.Equal(t, exit, decodedExit)
}
