package auth_test

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
)

var tokenKey = []byte("jwt-signing-key-0123456789abcdef-0123")

func testTokens(now time.Time) auth.Tokens {
	return auth.Tokens{Key: tokenKey, TTL: 15 * time.Minute, Now: func() time.Time { return now }}
}

func TestTokensIssueParse(t *testing.T) {
	now := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	tk := testTokens(now)
	user, session := uuid.New(), uuid.New()

	token, exp, err := tk.Issue(user, session, "mobile")
	require.NoError(t, err)
	require.True(t, exp.Equal(now.Add(15*time.Minute)))

	claims, err := tk.Parse(token)
	require.NoError(t, err)
	require.Equal(t, user, claims.UserID)
	require.Equal(t, session, claims.SessionID)
	require.Equal(t, "mobile", claims.Audience)

	_, err = testTokens(now.Add(16 * time.Minute)).Parse(token)
	require.ErrorIs(t, err, auth.ErrTokenExpired)

	tampered := []byte(token)
	last := strings.LastIndexByte(token, '.') + 5
	if tampered[last] == 'A' {
		tampered[last] = 'B'
	} else {
		tampered[last] = 'A'
	}
	_, err = tk.Parse(string(tampered))
	require.ErrorIs(t, err, auth.ErrTokenInvalid)

	_, err = tk.Parse("garbage")
	require.ErrorIs(t, err, auth.ErrTokenInvalid)
}

func TestTokensRejectForeignShapes(t *testing.T) {
	now := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	tk := testTokens(now)
	claims := jwt.MapClaims{
		"iss": "ekokod", "sub": uuid.NewString(), "sid": uuid.NewString(), "aud": "web",
		"typ": "access", "iat": now.Unix(), "exp": now.Add(time.Minute).Unix(),
	}
	sign := func(m jwt.SigningMethod, key any, c jwt.MapClaims) string {
		s, err := jwt.NewWithClaims(m, c).SignedString(key)
		require.NoError(t, err)
		return s
	}

	_, err := tk.Parse(sign(jwt.SigningMethodHS256, tokenKey, claims))
	require.NoError(t, err, "control: a well-formed token parses")

	none := sign(jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, claims)
	_, err = tk.Parse(none)
	require.ErrorIs(t, err, auth.ErrTokenInvalid)

	_, err = tk.Parse(sign(jwt.SigningMethodHS384, tokenKey, claims))
	require.ErrorIs(t, err, auth.ErrTokenInvalid)

	_, err = tk.Parse(sign(jwt.SigningMethodHS256, []byte("other-key-0123456789abcdef-0123456789"), claims))
	require.ErrorIs(t, err, auth.ErrTokenInvalid)

	refresh := jwt.MapClaims{}
	for k, v := range claims {
		refresh[k] = v
	}
	refresh["typ"] = "refresh"
	_, err = tk.Parse(sign(jwt.SigningMethodHS256, tokenKey, refresh))
	require.ErrorIs(t, err, auth.ErrTokenInvalid)

	noExp := jwt.MapClaims{}
	for k, v := range claims {
		if k != "exp" {
			noExp[k] = v
		}
	}
	_, err = tk.Parse(sign(jwt.SigningMethodHS256, tokenKey, noExp))
	require.ErrorIs(t, err, auth.ErrTokenInvalid)

	badAud := jwt.MapClaims{}
	for k, v := range claims {
		badAud[k] = v
	}
	badAud["aud"] = "admin"
	_, err = tk.Parse(sign(jwt.SigningMethodHS256, tokenKey, badAud))
	require.ErrorIs(t, err, auth.ErrTokenInvalid)
}

func TestRefreshTokenShape(t *testing.T) {
	plain, hash, err := auth.NewRefreshToken()
	require.NoError(t, err)
	require.Len(t, plain, 43)
	require.NotContains(t, plain, "=")
	require.Equal(t, hash, auth.HashRefreshToken(plain))
	require.Len(t, hash, 64)
	_, err = hex.DecodeString(hash)
	require.NoError(t, err)

	plain2, _, err := auth.NewRefreshToken()
	require.NoError(t, err)
	require.NotEqual(t, plain, plain2)
}

func TestFingerprintIsKeyedHMAC(t *testing.T) {
	a := auth.Fingerprint([]byte("secret-one"), "Mozilla/5.0 X")
	require.Len(t, a, 64)
	require.Equal(t, a, auth.Fingerprint([]byte("secret-one"), "Mozilla/5.0 X"))
	require.NotEqual(t, a, auth.Fingerprint([]byte("secret-two"), "Mozilla/5.0 X"))
	require.NotEqual(t, a, auth.Fingerprint([]byte("secret-one"), "Mozilla/5.0 Y"))
}
