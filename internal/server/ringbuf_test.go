package server

import (
	"bytes"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// RingBuffer unit tests
// ---------------------------------------------------------------------------

func TestNewRingBuffer_DefaultSize(t *testing.T) {
	r := NewRingBuffer(0)
	assert.Equal(t, DefaultRingSize, r.size)
	assert.Equal(t, 0, r.Len())
}

func TestNewRingBuffer_NegativeSize(t *testing.T) {
	r := NewRingBuffer(-10)
	assert.Equal(t, DefaultRingSize, r.size)
}

func TestNewRingBuffer_CustomSize(t *testing.T) {
	r := NewRingBuffer(512)
	assert.Equal(t, 512, r.size)
	assert.Equal(t, 0, r.Len())
}

func TestRingBuffer_EmptyBytes(t *testing.T) {
	r := NewRingBuffer(64)
	b := r.Bytes()
	assert.Empty(t, b)
	assert.Equal(t, 0, r.Len())
}

func TestRingBuffer_SingleWrite(t *testing.T) {
	r := NewRingBuffer(64)
	r.Write([]byte("hello"))
	assert.Equal(t, 5, r.Len())
	assert.Equal(t, []byte("hello"), r.Bytes())
}

func TestRingBuffer_MultipleWrites(t *testing.T) {
	r := NewRingBuffer(64)
	r.Write([]byte("hello "))
	r.Write([]byte("world"))
	assert.Equal(t, 11, r.Len())
	assert.Equal(t, []byte("hello world"), r.Bytes())
}

func TestRingBuffer_ExactFit(t *testing.T) {
	r := NewRingBuffer(5)
	r.Write([]byte("abcde"))
	assert.Equal(t, 5, r.Len())
	assert.Equal(t, []byte("abcde"), r.Bytes())
}

func TestRingBuffer_Wraparound(t *testing.T) {
	r := NewRingBuffer(8)
	r.Write([]byte("12345678")) // fill completely
	assert.Equal(t, 8, r.Len())
	assert.Equal(t, []byte("12345678"), r.Bytes())

	// Write more — should overwrite oldest
	r.Write([]byte("AB"))
	assert.Equal(t, 8, r.Len())
	// Buffer should be: "345678AB" (oldest first)
	assert.Equal(t, []byte("345678AB"), r.Bytes())
}

func TestRingBuffer_LargeWriteOverflow(t *testing.T) {
	r := NewRingBuffer(4)
	// Write more than buffer size
	r.Write([]byte("abcdefgh"))
	assert.Equal(t, 4, r.Len())
	// Only last 4 bytes should remain
	assert.Equal(t, []byte("efgh"), r.Bytes())
}

func TestRingBuffer_MultiWrapAround(t *testing.T) {
	r := NewRingBuffer(4)
	r.Write([]byte("AB"))   // [A,B,_,_] w=2
	r.Write([]byte("CD"))   // [A,B,C,D] w=0, full=true
	r.Write([]byte("EF"))   // [E,F,C,D] w=2, full=true
	assert.Equal(t, 4, r.Len())
	assert.Equal(t, []byte("CDEF"), r.Bytes())
}

func TestRingBuffer_BytesReturnsCopy(t *testing.T) {
	r := NewRingBuffer(16)
	r.Write([]byte("test"))
	b := r.Bytes()
	// Mutating the returned slice should not affect the buffer
	b[0] = 'X'
	assert.Equal(t, []byte("test"), r.Bytes())
}

func TestRingBuffer_Reset(t *testing.T) {
	r := NewRingBuffer(16)
	r.Write([]byte("some data"))
	assert.Equal(t, 9, r.Len())

	r.Reset()
	assert.Equal(t, 0, r.Len())
	assert.Empty(t, r.Bytes())
}

func TestRingBuffer_ResetAfterWrap(t *testing.T) {
	r := NewRingBuffer(4)
	r.Write([]byte("abcdef"))
	assert.Equal(t, 4, r.Len())

	r.Reset()
	assert.Equal(t, 0, r.Len())
	assert.Empty(t, r.Bytes())

	// Can write again after reset
	r.Write([]byte("XY"))
	assert.Equal(t, 2, r.Len())
	assert.Equal(t, []byte("XY"), r.Bytes())
}

func TestRingBuffer_ConcurrentWrites(t *testing.T) {
	r := NewRingBuffer(1024)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Write([]byte("data"))
		}()
	}
	wg.Wait()

	// Buffer should not panic or corrupt — just verify it's valid
	b := r.Bytes()
	assert.True(t, len(b) > 0)
	assert.LessOrEqual(t, len(b), 1024)
}

func TestRingBuffer_ConcurrentReadWrite(t *testing.T) {
	r := NewRingBuffer(256)
	var wg sync.WaitGroup

	// Concurrent writers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Write([]byte("hello"))
		}()
	}

	// Concurrent readers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.Bytes()
			_ = r.Len()
		}()
	}

	wg.Wait()
	// No race condition = success
}

func TestRingBuffer_WriteEmptySlice(t *testing.T) {
	r := NewRingBuffer(16)
	r.Write([]byte{})
	assert.Equal(t, 0, r.Len())
	assert.Empty(t, r.Bytes())
}

func TestRingBuffer_WriteSingleByte(t *testing.T) {
	r := NewRingBuffer(4)
	r.Write([]byte{0x41})
	r.Write([]byte{0x42})
	r.Write([]byte{0x43})
	r.Write([]byte{0x44})
	assert.Equal(t, 4, r.Len())
	assert.Equal(t, []byte("ABCD"), r.Bytes())

	r.Write([]byte{0x45})
	assert.Equal(t, 4, r.Len())
	assert.Equal(t, []byte("BCDE"), r.Bytes())
}

func TestRingBuffer_BinaryData(t *testing.T) {
	r := NewRingBuffer(8)
	data := []byte{0x00, 0xFF, 0x01, 0xFE, 0x02, 0xFD}
	r.Write(data)
	assert.Equal(t, 6, r.Len())
	assert.True(t, bytes.Equal(data, r.Bytes()))
}

func TestRingBuffer_DefaultRingSizeConstant(t *testing.T) {
	require.Equal(t, 256*1024, DefaultRingSize, "DefaultRingSize should be 256KB")
}
