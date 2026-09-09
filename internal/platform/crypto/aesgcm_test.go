package crypto_test

import (
	"crypto/rand"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	"github.com/stretchr/testify/require"
)

func key(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	_, err := rand.Read(k)
	require.NoError(t, err)
	return k
}

func TestSealOpenRoundTrip(t *testing.T) {
	c, err := crypto.NewCipher(key(t))
	require.NoError(t, err)

	token, err := c.Seal([]byte("gridbox-password"), []byte("company-1"))
	require.NoError(t, err)
	require.NotContains(t, token, "gridbox-password")

	got, err := c.Open(token, []byte("company-1"))
	require.NoError(t, err)
	require.Equal(t, "gridbox-password", string(got))
}

func TestSealProducesDifferentCiphertextEachTime(t *testing.T) {
	c, err := crypto.NewCipher(key(t))
	require.NoError(t, err)

	a, err := c.Seal([]byte("same"), nil)
	require.NoError(t, err)
	b, err := c.Seal([]byte("same"), nil)
	require.NoError(t, err)
	require.NotEqual(t, a, b, "a fresh nonce must be used for every seal")
}

func TestOpenRejectsWrongKey(t *testing.T) {
	c1, err := crypto.NewCipher(key(t))
	require.NoError(t, err)
	c2, err := crypto.NewCipher(key(t))
	require.NoError(t, err)

	token, err := c1.Seal([]byte("secret"), nil)
	require.NoError(t, err)
	_, err = c2.Open(token, nil)
	require.Error(t, err)
}

func TestOpenRejectsTamperedTokenAndWrongAAD(t *testing.T) {
	c, err := crypto.NewCipher(key(t))
	require.NoError(t, err)

	token, err := c.Seal([]byte("secret"), []byte("company-1"))
	require.NoError(t, err)

	tampered := []byte(token)
	tampered[len(tampered)-2] ^= 'A'
	_, err = c.Open(string(tampered), []byte("company-1"))
	require.Error(t, err)

	_, err = c.Open(token, []byte("company-2"))
	require.Error(t, err, "authenticated data must be bound to the ciphertext")
}

func TestNewCipherRejectsWrongKeyLength(t *testing.T) {
	_, err := crypto.NewCipher(make([]byte, 16))
	require.Error(t, err)
	require.Contains(t, err.Error(), "32")
}
