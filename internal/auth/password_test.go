package auth_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
)

func testHasher() auth.Hasher {
	return auth.Hasher{Pepper: []byte("pepper-0123456789abcdef-0123456789"), Cost: bcrypt.MinCost}
}

func TestHasherRoundTripAndPepper(t *testing.T) {
	h := testHasher()
	stored, err := h.Hash("Ekokod!2026x")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(stored, "$2a$"), stored)

	ok, rehash := h.Verify(stored, "Ekokod!2026x")
	require.True(t, ok)
	require.False(t, rehash)

	ok, _ = h.Verify(stored, "Ekokod!2026y")
	require.False(t, ok)

	other := auth.Hasher{Pepper: []byte("another-pepper-0123456789abcdef-01"), Cost: bcrypt.MinCost}
	ok, _ = other.Verify(stored, "Ekokod!2026x")
	require.False(t, ok, "the pepper must take part in the hash")
}

func TestHasherLegacyPrefixNeedsRehash(t *testing.T) {
	h := testHasher()
	legacy, err := bcrypt.GenerateFromPassword([]byte("Eski!Parola99"), bcrypt.MinCost)
	require.NoError(t, err)
	stored := "legacy$" + string(legacy)

	ok, rehash := h.Verify(stored, "Eski!Parola99")
	require.True(t, ok)
	require.True(t, rehash)

	ok, rehash = h.Verify(stored, "Yanlis!Parola99")
	require.False(t, ok)
	require.False(t, rehash)
}

func TestHasherHandlesPasswordsLongerThan72Bytes(t *testing.T) {
	h := testHasher()
	a := strings.Repeat("a", 90) + "X" + strings.Repeat("b", 9)
	b := strings.Repeat("a", 90) + "Y" + strings.Repeat("b", 9)
	stored, err := h.Hash(a)
	require.NoError(t, err)
	ok, _ := h.Verify(stored, b)
	require.False(t, ok, "bytes beyond bcrypt's 72-byte limit must still count (R145)")
}

func TestHasherRejectsGarbageHash(t *testing.T) {
	ok, rehash := testHasher().Verify("not-a-hash", "whatever")
	require.False(t, ok)
	require.False(t, rehash)
}

func TestDummyVerifyCostsLikeARealCompare(t *testing.T) {
	h := auth.Hasher{Pepper: []byte("p"), Cost: 10}
	stored, err := h.Hash("Ekokod!2026x")
	require.NoError(t, err)
	h.DummyVerify("warm-up") // first call generates the per-cost hash

	start := time.Now()
	h.Verify(stored, "wrong")
	real := time.Since(start)
	start = time.Now()
	h.DummyVerify("wrong")
	dummy := time.Since(start)
	require.Greater(t, dummy, real/3, "dummy compare must do bcrypt work at the configured cost")
}
