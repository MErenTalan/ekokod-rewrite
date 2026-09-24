package legacy_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/migrate/legacy"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

// Vectors produced by the legacy src/utils/encryption.ts algorithm (aes-256-cbc,
// key = sha256(secret), "enc:" + iv hex + ":" + cipher hex) with fixed IVs.
const (
	vecPrimary  = "enc:000102030405060708090a0b0c0d0e0f:11fb7bd0f91070054d26d0696247d982" // "Şifre!2024", legacy-secret-key
	vecFallback = "enc:0f0e0d0c0b0a09080706050403020100:7d850d982a3af45da11199a113390c32" // "osos-pass", old-rotated-key
	vecEmpty    = "enc:1111111111111111111111111111111a:465eaf35e2ce63ffbaf05be050bb68b5" // "", legacy-secret-key
)

func TestLegacyDecryptMatchesTheLegacyAlgorithm(t *testing.T) {
	keys := legacy.Keys{Primary: "legacy-secret-key", Fallback: "old-rotated-key"}
	got, err := keys.Decrypt(vecPrimary)
	require.NoError(t, err)
	require.Equal(t, "Şifre!2024", got)
	got, err = keys.Decrypt(vecFallback)
	require.NoError(t, err)
	require.Equal(t, "osos-pass", got, "the rotation fallback key is tried second")
	got, err = keys.Decrypt(vecEmpty)
	require.NoError(t, err)
	require.Empty(t, got)

	_, err = legacy.Keys{Primary: "wrong"}.Decrypt(vecPrimary)
	require.ErrorIs(t, err, legacy.ErrDecrypt)
	for _, bad := range []string{"enc:zz:yy", "enc:0001:", "enc:000102030405060708090a0b0c0d0e0f:11fb", "garbage"} {
		_, err := keys.Decrypt(bad)
		require.ErrorIs(t, err, legacy.ErrDecrypt, bad)
	}
}

func TestResealBindsTheSecretToItsCredentialRow(t *testing.T) {
	cipher, err := crypto.NewCipher(make([]byte, 32))
	require.NoError(t, err)
	r := legacy.Resealer{Keys: legacy.Keys{Primary: "legacy-secret-key"}, Cipher: cipher}
	company, def := uuid.New(), uuid.New()

	token, plain, err := r.Reseal(company, def, vecPrimary)
	require.NoError(t, err)
	require.False(t, plain)
	opened, err := cipher.Open(token, postgres.IntegrationCredentialAAD(company, def))
	require.NoError(t, err)
	require.Equal(t, "Şifre!2024", string(opened))
	_, err = cipher.Open(token, postgres.IntegrationCredentialAAD(company, uuid.New()))
	require.Error(t, err, "a token copied to another row does not open")
	require.NotContains(t, token, "Şifre")

	token, plain, err = r.Reseal(company, def, "pre-encryption-era")
	require.NoError(t, err)
	require.True(t, plain, "an unprefixed legacy value is plaintext and is reported as such")
	opened, err = cipher.Open(token, postgres.IntegrationCredentialAAD(company, def))
	require.NoError(t, err)
	require.Equal(t, "pre-encryption-era", string(opened))

	_, _, err = legacy.Resealer{Keys: legacy.Keys{Primary: "wrong"}, Cipher: cipher}.Reseal(company, def, vecPrimary)
	require.ErrorIs(t, err, legacy.ErrDecrypt)
}
