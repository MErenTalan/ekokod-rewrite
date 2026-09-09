// Package crypto seals integration credentials at rest with AES-256-GCM.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// Cipher seals and opens values with a single AES-256-GCM key.
type Cipher struct{ aead cipher.AEAD }

// NewCipher builds a Cipher from a 32-byte key.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption key must be exactly 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("build aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("build gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Seal encrypts plaintext, binding aad to the ciphertext, and returns
// base64(nonce || ciphertext).
func (c *Cipher) Seal(plaintext, aad []byte) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, plaintext, aad)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Open reverses Seal. It fails if the key, the ciphertext or aad differ.
func (c *Cipher) Open(token string, aad []byte) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("decode token: %w", err)
	}
	if len(raw) < c.aead.NonceSize() {
		return nil, errors.New("token is too short to contain a nonce")
	}
	nonce, ciphertext := raw[:c.aead.NonceSize()], raw[c.aead.NonceSize():]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, errors.New("decryption failed: wrong key, tampered ciphertext or wrong associated data")
	}
	return plaintext, nil
}
