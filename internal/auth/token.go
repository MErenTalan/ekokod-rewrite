package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Token audiences (R140).
const (
	AudienceWeb    = "web"
	AudienceMobile = "mobile"
)

const (
	issuer          = "ekokod"
	accessTokenType = "access"
)

// Parse's two failure modes; the API maps them to token_expired and token_invalid.
var (
	ErrTokenExpired = errors.New("auth: token expired")
	ErrTokenInvalid = errors.New("auth: token invalid")
)

// Claims is what an access token proves.
type Claims struct {
	UserID, SessionID   uuid.UUID
	Audience            string
	IssuedAt, ExpiresAt time.Time
}

// Tokens issues and parses HS256 access tokens.
type Tokens struct {
	Key []byte
	TTL time.Duration
	Now func() time.Time
}

type accessClaims struct {
	SessionID string `json:"sid"`
	Type      string `json:"typ"`
	jwt.RegisteredClaims
}

// Issue signs an access token for one session.
func (t Tokens) Issue(userID, sessionID uuid.UUID, audience string) (string, time.Time, error) {
	now := t.Now().Truncate(time.Second)
	exp := now.Add(t.TTL)
	c := accessClaims{
		SessionID: sessionID.String(),
		Type:      accessTokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   userID.String(),
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(t.Key)
	return s, exp, err
}

// Parse verifies a token; an expired one is ErrTokenExpired, anything else wrong is ErrTokenInvalid.
func (t Tokens) Parse(token string) (Claims, error) {
	var c accessClaims
	_, err := jwt.ParseWithClaims(token, &c, func(*jwt.Token) (any, error) { return t.Key, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithTimeFunc(t.Now),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)
	if errors.Is(err, jwt.ErrTokenExpired) {
		return Claims{}, ErrTokenExpired
	}
	if err != nil || c.Type != accessTokenType || len(c.Audience) != 1 {
		return Claims{}, ErrTokenInvalid
	}
	aud := c.Audience[0]
	if aud != AudienceWeb && aud != AudienceMobile {
		return Claims{}, ErrTokenInvalid
	}
	user, uerr := uuid.Parse(c.Subject)
	session, serr := uuid.Parse(c.SessionID)
	if uerr != nil || serr != nil || c.IssuedAt == nil {
		return Claims{}, ErrTokenInvalid
	}
	return Claims{UserID: user, SessionID: session, Audience: aud, IssuedAt: c.IssuedAt.Time, ExpiresAt: c.ExpiresAt.Time}, nil
}

// NewRefreshToken returns an opaque refresh token and the hash stored for it (R141).
func NewRefreshToken() (plain, hash string, err error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", err
	}
	plain = base64.RawURLEncoding.EncodeToString(b[:])
	return plain, HashRefreshToken(plain), nil
}

// HashRefreshToken is the lookup key for a refresh token.
func HashRefreshToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// Fingerprint binds a session to a device's User-Agent (R142).
func Fingerprint(secret []byte, userAgent string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(userAgent))
	return hex.EncodeToString(mac.Sum(nil))
}
