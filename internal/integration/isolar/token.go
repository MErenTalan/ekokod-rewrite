package isolar

import (
	"strconv"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
)

// defaultExpiresInSeconds is R22's legacy default: "expires_in defaults to
// 7200 when absent" (isolarClient.ts:293,342).
const defaultExpiresInSeconds = 7200

// Token is the OAuth pair ExchangeCode and Refresh return. Task 14's
// credential service consumes this type directly, so its shape is fixed by
// this task's brief and must not change without coordinating there.
type Token struct {
	AccessToken integration.Secret

	// RefreshToken is the provider's new refresh token, or a ZERO Secret
	// (RefreshToken.IsZero() == true) when the response carried no
	// refresh_token (missing key or JSON null both decode to "" —
	// wire.go's wireTokenData doc). I4: a null refresh_token is legacy's
	// "no new refresh token" case (isolarClient.ts:339-341's refresh path
	// falls back to the OLD refresh_token it already has; this package
	// exposes the zero value instead of performing that fallback itself,
	// since it is stateless per-call — creds.Extra["refresh_token"] is
	// this call's own input, not a place to write back to). Task 14 MUST
	// NOT store token.RefreshToken over its existing value when IsZero()
	// is true: it must keep the prior refresh token, exactly as
	// isolarClient.ts's refreshAccessToken does.
	RefreshToken integration.Secret

	// ExpiresAt is now + expires_in (default 7200s when the provider omits
	// expires_in), computed against the Client's clock so a test can pin
	// it exactly.
	ExpiresAt time.Time
}

// tokenFromWire converts a decoded wireTokenData into a Token, resolving
// ExpiresAt against now (the Client's clock at the moment of the call). I4:
// an empty access_token — the provider's own "no access_token in response"
// failure mode (isolarClient.ts:297-299,ish "if (!tokenData.access_token)
// throw") — is ErrAuth, not a Token with a Secret that reveals "".
func tokenFromWire(w wireTokenData, now time.Time, op string) (Token, error) {
	if w.AccessToken == "" {
		return Token{}, &integration.Error{Kind: integration.ErrAuth, Provider: integration.ProviderISolar, Op: op}
	}

	seconds := defaultExpiresInSeconds
	if w.ExpiresIn != nil {
		n, err := strconv.Atoi(w.ExpiresIn.String())
		if err != nil {
			return Token{}, malformedErr(op)
		}
		seconds = n
	}
	return Token{
		AccessToken:  integration.NewSecret([]byte(w.AccessToken)),
		RefreshToken: integration.NewSecret([]byte(w.RefreshToken)),
		ExpiresAt:    now.Add(time.Duration(seconds) * time.Second),
	}, nil
}
