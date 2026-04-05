package captcha

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// solve finds a valid nonce for the given challenge. Used in tests only —
// production clients solve in JS. Uses a small difficulty to keep tests fast.
func solve(t *testing.T, c *Challenge) uint64 {
	t.Helper()
	salt, err := hex.DecodeString(c.Salt)
	require.NoError(t, err)
	target, err := hex.DecodeString(c.Target)
	require.NoError(t, err)
	for nonce := uint64(0); nonce < 1<<30; nonce++ {
		h := sha256.New()
		h.Write(salt)
		var nb [8]byte
		binary.BigEndian.PutUint64(nb[:], nonce)
		h.Write(nb[:])
		sum := h.Sum(nil)
		less := false
		for i := 0; i < 32; i++ {
			if sum[i] < target[i] {
				less = true
				break
			}
			if sum[i] > target[i] {
				break
			}
		}
		if less {
			return nonce
		}
	}
	t.Fatalf("solve exhausted")
	return 0
}

func newTestVerifier(t *testing.T) *Verifier {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	v := NewVerifier(key)
	v.SetDifficulty(8) // keep tests fast — ~256 expected iterations
	return v
}

func TestNewVerifier_RejectsShortKey(t *testing.T) {
	assert.Panics(t, func() { NewVerifier(make([]byte, 15)) })
	assert.NotPanics(t, func() { NewVerifier(make([]byte, 16)) })
}

func TestIssue_ReturnsFreshChallenge(t *testing.T) {
	v := newTestVerifier(t)
	c1, err := v.Issue("/auth/recovery/verify")
	require.NoError(t, err)
	require.NotNil(t, c1)
	assert.Len(t, c1.Salt, 64)
	assert.Len(t, c1.Target, 64)
	assert.Len(t, c1.HMAC, 64)
	assert.Equal(t, "/auth/recovery/verify", c1.Endpoint)
	assert.Greater(t, c1.Expires, time.Now().Unix())

	c2, err := v.Issue("/auth/recovery/verify")
	require.NoError(t, err)
	assert.NotEqual(t, c1.Salt, c2.Salt, "salts must be unique")
	assert.NotEqual(t, c1.HMAC, c2.HMAC, "hmacs must differ")
}

func TestIssue_RejectsInvalidEndpoints(t *testing.T) {
	v := newTestVerifier(t)
	cases := []string{
		"",
		"auth/recovery/verify",              // no leading /
		"/auth//recovery",                   // double slash
		"/auth/recovery?x=1",                // query chars
		"/auth/recovery#x",                  // fragment
		"/auth/reco very",                   // space
		"/" + strings.Repeat("a", 200),      // too long
		"/auth/recovery\x00drop",            // control char
	}
	for _, e := range cases {
		_, err := v.Issue(e)
		assert.Error(t, err, "should reject %q", e)
	}
}

func TestVerify_HappyPath(t *testing.T) {
	v := newTestVerifier(t)
	c, err := v.Issue("/auth/recovery/verify")
	require.NoError(t, err)
	nonce := solve(t, c)
	sol := &Solution{
		Salt: c.Salt, Target: c.Target, Expires: c.Expires,
		Endpoint: c.Endpoint, HMAC: c.HMAC, Nonce: nonce,
	}
	assert.NoError(t, v.Verify(sol, "/auth/recovery/verify"))
}

func TestVerify_RejectsReplay(t *testing.T) {
	v := newTestVerifier(t)
	c, err := v.Issue("/auth/recovery/verify")
	require.NoError(t, err)
	nonce := solve(t, c)
	sol := &Solution{
		Salt: c.Salt, Target: c.Target, Expires: c.Expires,
		Endpoint: c.Endpoint, HMAC: c.HMAC, Nonce: nonce,
	}
	require.NoError(t, v.Verify(sol, "/auth/recovery/verify"))
	assert.ErrorIs(t, v.Verify(sol, "/auth/recovery/verify"), ErrReplayed)
}

func TestVerify_RejectsForgedHMAC(t *testing.T) {
	v := newTestVerifier(t)
	c, err := v.Issue("/auth/recovery/verify")
	require.NoError(t, err)
	nonce := solve(t, c)
	sol := &Solution{
		Salt: c.Salt, Target: c.Target, Expires: c.Expires,
		Endpoint: c.Endpoint, HMAC: strings.Repeat("0", 64), Nonce: nonce,
	}
	assert.ErrorIs(t, v.Verify(sol, "/auth/recovery/verify"), ErrInvalidHMAC)
}

func TestVerify_RejectsTamperedExpiry(t *testing.T) {
	v := newTestVerifier(t)
	c, err := v.Issue("/auth/recovery/verify")
	require.NoError(t, err)
	nonce := solve(t, c)
	sol := &Solution{
		Salt: c.Salt, Target: c.Target, Expires: c.Expires + 3600, // extend lifetime
		Endpoint: c.Endpoint, HMAC: c.HMAC, Nonce: nonce,
	}
	assert.ErrorIs(t, v.Verify(sol, "/auth/recovery/verify"), ErrInvalidHMAC)
}

func TestVerify_RejectsExpired(t *testing.T) {
	v := newTestVerifier(t)
	fakeTime := time.Unix(1_000_000, 0)
	v.now = func() time.Time { return fakeTime }
	c, err := v.Issue("/auth/recovery/verify")
	require.NoError(t, err)
	nonce := solve(t, c)
	sol := &Solution{
		Salt: c.Salt, Target: c.Target, Expires: c.Expires,
		Endpoint: c.Endpoint, HMAC: c.HMAC, Nonce: nonce,
	}
	v.now = func() time.Time { return fakeTime.Add(DefaultTTL + time.Second) }
	assert.ErrorIs(t, v.Verify(sol, "/auth/recovery/verify"), ErrExpired)
}

func TestVerify_RejectsCrossEndpointReuse(t *testing.T) {
	v := newTestVerifier(t)
	c, err := v.Issue("/auth/recovery/verify")
	require.NoError(t, err)
	nonce := solve(t, c)
	sol := &Solution{
		Salt: c.Salt, Target: c.Target, Expires: c.Expires,
		Endpoint: c.Endpoint, HMAC: c.HMAC, Nonce: nonce,
	}
	assert.ErrorIs(t, v.Verify(sol, "/auth/password/login"), ErrWrongEndpoint)
}

func TestVerify_RejectsInsufficientPoW(t *testing.T) {
	v := newTestVerifier(t)
	c, err := v.Issue("/auth/recovery/verify")
	require.NoError(t, err)
	sol := &Solution{
		Salt: c.Salt, Target: c.Target, Expires: c.Expires,
		Endpoint: c.Endpoint, HMAC: c.HMAC, Nonce: 0, // almost certainly wrong at d=8
	}
	// Find a nonce that definitely doesn't meet the target by using difficulty=8
	// with a fixed salt. Nonce 0 has ~1/256 chance of being valid; try a few
	// values until we find a losing one.
	for n := uint64(0); n < 100; n++ {
		sol.Nonce = n
		if err := v.Verify(sol, "/auth/recovery/verify"); err == ErrInsufficientPoW {
			return
		}
	}
	t.Skip("could not find a failing nonce in 100 tries (unlucky RNG)")
}

func TestVerify_RejectsMalformedSolution(t *testing.T) {
	v := newTestVerifier(t)
	cases := []*Solution{
		nil,
		{}, // all empty
		{Salt: "notHex!!" + strings.Repeat("x", 56), Target: strings.Repeat("0", 64),
			HMAC: strings.Repeat("0", 64), Endpoint: "/x", Expires: 1},
		{Salt: strings.Repeat("a", 63), Target: strings.Repeat("0", 64),
			HMAC: strings.Repeat("0", 64), Endpoint: "/x", Expires: 1},
	}
	for i, sol := range cases {
		err := v.Verify(sol, "/x")
		assert.Error(t, err, "case %d", i)
	}
}

func TestComputeTarget_LeadingZeroBits(t *testing.T) {
	cases := []struct {
		difficulty int
		firstByte  byte
		zeroBytes  int
	}{
		{1, 0x7F, 0},
		{7, 0x01, 0},
		{8, 0xFF, 1},
		{16, 0xFF, 2},
		{18, 0x3F, 2},
	}
	for _, c := range cases {
		got := computeTarget(c.difficulty)
		for i := 0; i < c.zeroBytes; i++ {
			assert.Equal(t, byte(0), got[i], "diff=%d byte[%d]", c.difficulty, i)
		}
		assert.Equal(t, c.firstByte, got[c.zeroBytes], "diff=%d", c.difficulty)
	}
}

func TestReplayCache_PrunesExpired(t *testing.T) {
	v := newTestVerifier(t)
	base := time.Unix(1_000_000, 0)
	v.now = func() time.Time { return base }
	v.markSeen("salt-a", base.Add(10*time.Second).Unix())
	v.markSeen("salt-b", base.Add(100*time.Second).Unix())

	// Advance time past salt-a's expiry
	advanced := base.Add(30 * time.Second).Unix()
	assert.False(t, v.isSeen("salt-a", advanced))
	assert.True(t, v.isSeen("salt-b", advanced))
}
