// Package auth holds the pure authentication primitives: password hashing and
// policy, access and refresh tokens, device fingerprints, role sets and the
// permission table. It performs no I/O and knows nothing about storage.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// legacyPrefix marks a migrated legacy bcrypt hash of the bare password (R145).
const legacyPrefix = "legacy$"

// dummyHashes holds one throwaway hash per bcrypt cost, so an unknown e-mail
// costs the same bcrypt work as a wrong password at the configured cost (R147).
var dummyHashes sync.Map // int → []byte

// Hasher hashes passwords as bcrypt over an HMAC of the password (R145), so
// bcrypt's 72-byte truncation never drops password bytes.
type Hasher struct {
	Pepper []byte
	Cost   int
}

func (h Hasher) prehash(password string) []byte {
	mac := hmac.New(sha256.New, h.Pepper)
	mac.Write([]byte(password))
	return []byte(base64.StdEncoding.EncodeToString(mac.Sum(nil)))
}

// Hash returns the stored form of password.
func (h Hasher) Hash(password string) (string, error) {
	out, err := bcrypt.GenerateFromPassword(h.prehash(password), h.Cost)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Verify reports whether password matches stored, and whether stored should
// be replaced by a fresh Hash (a legacy hash that matched).
func (h Hasher) Verify(stored, password string) (ok, needsRehash bool) {
	if rest, isLegacy := strings.CutPrefix(stored, legacyPrefix); isLegacy {
		if bcrypt.CompareHashAndPassword([]byte(rest), []byte(password)) == nil {
			return true, true
		}
		return false, false
	}
	return bcrypt.CompareHashAndPassword([]byte(stored), h.prehash(password)) == nil, false
}

// DummyVerify spends one bcrypt comparison and discards the result.
func (h Hasher) DummyVerify(password string) {
	v, ok := dummyHashes.Load(h.Cost)
	if !ok {
		generated, err := bcrypt.GenerateFromPassword([]byte("ekokod-dummy-password"), h.Cost)
		if err != nil {
			return
		}
		v, _ = dummyHashes.LoadOrStore(h.Cost, generated)
	}
	_ = bcrypt.CompareHashAndPassword(v.([]byte), h.prehash(password))
}
