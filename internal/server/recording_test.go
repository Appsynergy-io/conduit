package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// asciicastRecorder unit tests (PR #34 — session recording format)
// ---------------------------------------------------------------------------

func TestAsciicastRecorder_Header(t *testing.T) {
	rec := newAsciicastRecorder(120, 40)
	data := rec.Bytes()

	// First line should be valid JSON header
	lines := strings.SplitN(string(data), "\n", 2)
	require.NotEmpty(t, lines)

	var header asciicastHeader
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &header))
	assert.Equal(t, 2, header.Version)
	assert.Equal(t, 120, header.Width)
	assert.Equal(t, 40, header.Height)
	assert.NotZero(t, header.Timestamp)
}

func TestAsciicastRecorder_WriteOutput(t *testing.T) {
	rec := newAsciicastRecorder(80, 24)

	rec.WriteOutput([]byte("hello"))
	rec.WriteOutput([]byte("world"))

	data := string(rec.Bytes())
	lines := strings.Split(strings.TrimSpace(data), "\n")
	// header + 2 events
	assert.Len(t, lines, 3)

	// Parse first event
	var event []interface{}
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &event))
	assert.Len(t, event, 3)
	// [elapsed, "o", "hello"]
	assert.IsType(t, float64(0), event[0])
	assert.Equal(t, "o", event[1])
	assert.Equal(t, "hello", event[2])
}

func TestAsciicastRecorder_ElapsedTime(t *testing.T) {
	rec := newAsciicastRecorder(80, 24)
	rec.WriteOutput([]byte("first"))
	time.Sleep(10 * time.Millisecond)
	rec.WriteOutput([]byte("second"))

	data := string(rec.Bytes())
	lines := strings.Split(strings.TrimSpace(data), "\n")
	require.Len(t, lines, 3)

	var event1, event2 []interface{}
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &event1))
	require.NoError(t, json.Unmarshal([]byte(lines[2]), &event2))

	t1 := event1[0].(float64)
	t2 := event2[0].(float64)
	assert.Greater(t, t2, t1, "second event should have a later timestamp")
}

func TestAsciicastRecorder_SpecialCharacters(t *testing.T) {
	rec := newAsciicastRecorder(80, 24)
	rec.WriteOutput([]byte("line1\nline2\ttab\"quote"))

	data := string(rec.Bytes())
	lines := strings.Split(strings.TrimSpace(data), "\n")
	require.Len(t, lines, 2) // header + 1 event

	var event []interface{}
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &event))
	// The string should contain the special chars
	assert.Contains(t, event[2].(string), "\n")
	assert.Contains(t, event[2].(string), "\t")
	assert.Contains(t, event[2].(string), "\"")
}

func TestAsciicastRecorder_Duration(t *testing.T) {
	rec := newAsciicastRecorder(80, 24)
	// Duration should be >= 0
	dur := rec.Duration()
	assert.GreaterOrEqual(t, dur, 0)
}

func TestAsciicastRecorder_EmptyOutput(t *testing.T) {
	rec := newAsciicastRecorder(80, 24)
	rec.WriteOutput([]byte{})

	data := string(rec.Bytes())
	lines := strings.Split(strings.TrimSpace(data), "\n")
	assert.Len(t, lines, 2) // header + 1 event for empty data

	var event []interface{}
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &event))
	assert.Equal(t, "", event[2])
}

func TestAsciicastRecorder_ConcurrentWrites(t *testing.T) {
	rec := newAsciicastRecorder(80, 24)
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			rec.WriteOutput([]byte("concurrent"))
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	data := string(rec.Bytes())
	lines := strings.Split(strings.TrimSpace(data), "\n")
	// header + 10 events
	assert.Len(t, lines, 11)
}
