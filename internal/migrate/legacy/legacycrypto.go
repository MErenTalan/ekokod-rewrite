package legacy

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

// ErrDecrypt is a legacy secret that no supplied key opens (R410).
var ErrDecrypt = errors.New("decrypt_failed")

const encPrefix = "enc:"

// Keys are the legacy ENCRYPTION_SECRET_KEY and its optional rotation fallback.
// They are needed at transform time only and are never stored (08 §5).
type Keys struct {
	Primary  string
	Fallback string
}

// Decrypt opens a legacy "enc:<iv hex>:<cipher hex>" value: AES-256-CBC, key =
// SHA-256(secret), PKCS#7 — exactly src/utils/encryption.ts.
func (k Keys) Decrypt(value string) (string, error) {
	ivHex, ctHex, ok := strings.Cut(strings.TrimPrefix(value, encPrefix), ":")
	if !ok || ivHex == "" || ctHex == "" {
		return "", fmt.Errorf("%w: not iv:cipher", ErrDecrypt)
	}
	iv, err1 := hex.DecodeString(ivHex)
	ct, err2 := hex.DecodeString(ctHex)
	if err1 != nil || err2 != nil || len(iv) != aes.BlockSize || len(ct) == 0 || len(ct)%aes.BlockSize != 0 {
		return "", fmt.Errorf("%w: malformed", ErrDecrypt)
	}
	for _, secret := range []string{k.Primary, k.Fallback} {
		if secret == "" {
			continue
		}
		if plain, ok := openCBC(secret, iv, ct); ok {
			return plain, nil
		}
	}
	return "", fmt.Errorf("%w: no key opens it", ErrDecrypt)
}

func openCBC(secret string, iv, ct []byte) (string, bool) {
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", false
	}
	out := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, ct)
	pad := int(out[len(out)-1])
	if pad == 0 || pad > aes.BlockSize || !bytes.Equal(out[len(out)-pad:], bytes.Repeat([]byte{byte(pad)}, pad)) {
		return "", false // a wrong key almost always breaks the padding
	}
	return string(out[:len(out)-pad]), true
}

// Resealer turns legacy secrets into the new system's sealed tokens without
// plaintext ever reaching disk (Q-J3).
type Resealer struct {
	Keys   Keys
	Cipher *crypto.Cipher
}

// Reseal returns the new token for one credential row; plain reports a value
// stored before the legacy system encrypted secrets.
func (r Resealer) Reseal(companyID, definitionID uuid.UUID, value string) (token string, plain bool, err error) {
	secret := value
	if strings.HasPrefix(value, encPrefix) {
		if secret, err = r.Keys.Decrypt(value); err != nil {
			return "", false, err
		}
	} else {
		plain = true
	}
	token, err = r.Cipher.Seal([]byte(secret), postgres.IntegrationCredentialAAD(companyID, definitionID))
	return token, plain, err
}
