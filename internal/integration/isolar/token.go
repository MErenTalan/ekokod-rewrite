package isolar

import (
	"strconv"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
)

// defaultExpiresInSeconds is R22's legacy default: "expires_in defaults to
// 7200 when absent".
const defaultExpiresInSeconds = 7200

// Token is the OAuth pair ExchangeCode and Refresh return. Task 14's
// credential service consumes this type directly, so its shape is fixed by
// this task's brief and must not change without coordinating there.
type Token struct {
	AccessToken, RefreshToken integration.Secret
	// ExpiresAt is now + expires_in (default 7200s when the provider omits
	// expires_in), computed against the Client's clock so a test can pin
	// it exactly.
	ExpiresAt time.Time
}

// tokenFromWire converts a decoded wireTokenData into a Token, resolving
// ExpiresAt against now (the Client's clock at the moment of the call).
func tokenFromWire(w wireTokenData, now time.Time) (Token, error) {
	seconds := defaultExpiresInSeconds
	if w.ExpiresIn != nil {
		n, err := strconv.Atoi(w.ExpiresIn.String())
		if err != nil {
			return Token{}, malformedErr("token")
		}
		seconds = n
	}
	return Token{
		AccessToken:  integration.NewSecret([]byte(w.AccessToken)),
		RefreshToken: integration.NewSecret([]byte(w.RefreshToken)),
		ExpiresAt:    now.Add(time.Duration(seconds) * time.Second),
	}, nil
}
