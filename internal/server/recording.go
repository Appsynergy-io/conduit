package server

import (
	"bytes"
	"encoding/json"
	"sync"
	"time"
)

// asciicastRecorder captures shell output in asciicast v2 format.
// See: https://docs.asciinema.org/manual/asciicast/v2/
type asciicastRecorder struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	start time.Time
}

// asciicastHeader is the asciicast v2 header object (first line of the file).
type asciicastHeader struct {
	Version   int   `json:"version"`
	Width     int   `json:"width"`
	Height    int   `json:"height"`
	Timestamp int64 `json:"timestamp"`
}

// newAsciicastRecorder creates a recorder and writes the asciicast v2 header line.
func newAsciicastRecorder(cols, rows int) *asciicastRecorder {
	r := &asciicastRecorder{
		start: time.Now(),
	}

	header := asciicastHeader{
		Version:   2,
		Width:     cols,
		Height:    rows,
		Timestamp: r.start.Unix(),
	}
	headerBytes, _ := json.Marshal(header)
	r.buf.Write(headerBytes)
	r.buf.WriteByte('\n')

	return r
}

// WriteOutput appends an output event line in asciicast v2 format.
// Each event is a JSON array: [elapsed_seconds, "o", data_string]
func (r *asciicastRecorder) WriteOutput(data []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	elapsed := time.Since(r.start).Seconds()

	// Marshal the data as a JSON string to get proper escaping
	dataJSON, _ := json.Marshal(string(data))

	// Build the event line: [elapsed, "o", "data"]
	var line bytes.Buffer
	line.WriteByte('[')
	// Format elapsed as float with 6 decimal places
	elapsedBytes, _ := json.Marshal(elapsed)
	line.Write(elapsedBytes)
	line.WriteString(`, "o", `)
	line.Write(dataJSON)
	line.WriteByte(']')
	line.WriteByte('\n')

	r.buf.Write(line.Bytes())
}

// Bytes returns the recorded data.
func (r *asciicastRecorder) Bytes() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.Bytes()
}

// Duration returns elapsed seconds since start.
func (r *asciicastRecorder) Duration() int {
	return int(time.Since(r.start).Seconds())
}
