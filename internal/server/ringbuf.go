package server

import "sync"

// DefaultRingSize is the default ring buffer capacity per session (256KB).
const DefaultRingSize = 256 * 1024

// RingBuffer is a fixed-size circular byte buffer for buffering terminal
// output during detached sessions. When a browser reattaches, the ring
// contents are replayed to bring the terminal up to date.
type RingBuffer struct {
	mu   sync.Mutex
	buf  []byte
	size int
	w    int  // next write position
	full bool // true once the buffer has wrapped
}

// NewRingBuffer creates a ring buffer with the given capacity.
func NewRingBuffer(size int) *RingBuffer {
	if size <= 0 {
		size = DefaultRingSize
	}
	return &RingBuffer{
		buf:  make([]byte, size),
		size: size,
	}
}

// Write appends data to the ring buffer, overwriting oldest data if full.
func (r *RingBuffer) Write(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for len(p) > 0 {
		n := copy(r.buf[r.w:], p)
		r.w += n
		if r.w >= r.size {
			r.w = 0
			r.full = true
		}
		p = p[n:]
	}
}

// Bytes returns the buffered data in order (oldest first).
// Returns a copy — safe to use after the lock is released.
func (r *RingBuffer) Bytes() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.full {
		out := make([]byte, r.w)
		copy(out, r.buf[:r.w])
		return out
	}

	// Buffer has wrapped: data is [w..size) + [0..w)
	out := make([]byte, r.size)
	n := copy(out, r.buf[r.w:])
	copy(out[n:], r.buf[:r.w])
	return out
}

// Len returns the number of bytes currently stored.
func (r *RingBuffer) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.full {
		return r.size
	}
	return r.w
}

// Reset clears the buffer.
func (r *RingBuffer) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.w = 0
	r.full = false
}
