package credentials

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"strings"
	"time"

	"github.com/google/uuid"
)

// stateTTL is R22's 10-minute expiry.
const stateTTL = 10 * time.Minute

// stateNonceLen is the nonce's byte length: large enough that a forged
// state cannot guess an in-flight nonce, and it doubles as the single-use
// Consume key.
const stateNonceLen = 16

// statePayloadLen is the fixed wire length of a state token's payload half:
// companyID(16) + credentialID(16) + expiry unix seconds, big-endian
// (8) + nonce(16).
const statePayloadLen = 16 + 16 + 8 + stateNonceLen

// stateClaims is what a verified state token asserts.
type stateClaims struct {
	CompanyID    uuid.UUID
	CredentialID uuid.UUID
	Expiry       time.Time
	Nonce        [stateNonceLen]byte
}

// stateNonceKey is the lock.Consumer key a state's nonce single-use-consumes
// under.
func stateNonceKey(nonce [stateNonceLen]byte) string {
	return "isolar:state:" + base64.RawURLEncoding.EncodeToString(nonce[:])
}

// signState builds R22's state token:
// base64url(companyID||credentialID||expiry unix||nonce) + "." +
// base64url(HMAC-SHA256(key, payload)).
func signState(key []byte, companyID, credentialID uuid.UUID, now time.Time) (string, error) {
	var nonce [stateNonceLen]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}

	payload := make([]byte, 0, statePayloadLen)
	payload = append(payload, companyID[:]...)
	payload = append(payload, credentialID[:]...)
	var expBuf [8]byte
	binary.BigEndian.PutUint64(expBuf[:], uint64(now.Add(stateTTL).Unix()))
	payload = append(payload, expBuf[:]...)
	payload = append(payload, nonce[:]...)

	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	sig := mac.Sum(nil)

	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// verifyState checks the HMAC (via hmac.Equal, never a string/byte-slice
// == comparison — that would be a timing side channel) and, INDEPENDENTLY,
// the expiry: a mutation that skips the expiry check must not be masked by
// the HMAC check alone, which is exactly what Step 5(c)'s mutation proof
// exercises.
func verifyState(key []byte, token string, now time.Time) (stateClaims, error) {
	encPayload, encSig, ok := strings.Cut(token, ".")
	if !ok {
		return stateClaims{}, ErrInvalidState
	}
	payload, err := base64.RawURLEncoding.DecodeString(encPayload)
	if err != nil || len(payload) != statePayloadLen {
		return stateClaims{}, ErrInvalidState
	}
	sig, err := base64.RawURLEncoding.DecodeString(encSig)
	if err != nil {
		return stateClaims{}, ErrInvalidState
	}

	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	expected := mac.Sum(nil)
	if !hmac.Equal(expected, sig) {
		return stateClaims{}, ErrInvalidState
	}

	var claims stateClaims
	copy(claims.CompanyID[:], payload[0:16])
	copy(claims.CredentialID[:], payload[16:32])
	expUnix := int64(binary.BigEndian.Uint64(payload[32:40]))
	claims.Expiry = time.Unix(expUnix, 0).UTC()
	copy(claims.Nonce[:], payload[40:statePayloadLen])

	if now.After(claims.Expiry) {
		return stateClaims{}, ErrInvalidState
	}
	return claims, nil
}
