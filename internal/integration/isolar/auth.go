package isolar

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
)

// opToken/opRefreshToken are integration_definitions.json's isolar row keys
// (R40) — creds.Endpoints["token"]/creds.Endpoints["refresh_token"] name
// the two token-related relative paths, both under gateway +
// "/openapi/apiManage/..." (internal/seed/data/integration_definitions.json).
const (
	opToken        = "token"
	opRefreshToken = "refresh_token"
)

// AuthorizeURL builds the iSolarCloud authorisation-request URL (06 §6
// "Authorisation flow" step 1, R22's exact legacy format —
// isolarClient.ts:676-684 buildAuthorizeUrl):
//
//	{authorize_origin}/#/authorized-app?cloudId={cloud_id}&applicationId={app_id}&redirectUrl={redirect}
//
// R40: authorize_origin and cloud_id both come from creds.Endpoints — the
// integration_definitions.json isolar row's own fields, never a
// per-Region constant table (round 1's regionCloudID map is removed; this
// package no longer reads creds.Region at all). app_id comes from
// creds.Extra["app_id"].
//
// redirectURI is taken EXACTLY as given and query-escaped once. R22's
// HMAC-signed state ("redirectUrl=<callback>?state=<signed>") is produced
// and verified by Task 14's credential service, not by this package: this
// Client has no access to EKOKOD_JWT_SIGNING_KEY (it would break 06 §1 rule
// 2's purity — no config/store dependency), so the caller is expected to
// hand redirectURI already carrying its own "?state=..." query when one is
// wanted. This function's only job is the R22 string format.
func (c *Client) AuthorizeURL(creds integration.Credentials, redirectURI string) (string, error) {
	origin, ok := creds.Endpoints[endpointAuthorizeOrigin]
	if !ok || origin == "" {
		return "", c.configError(endpointAuthorizeOrigin)
	}
	cloudID, ok := creds.Endpoints[endpointCloudID]
	if !ok || cloudID == "" {
		return "", c.configError(endpointCloudID)
	}
	appID := creds.Extra["app_id"]
	if appID.IsZero() {
		return "", c.configError("app_id")
	}

	return origin + "/#/authorized-app?cloudId=" + url.QueryEscape(cloudID) +
		"&applicationId=" + url.QueryEscape(appID.Reveal()) +
		"&redirectUrl=" + url.QueryEscape(redirectURI), nil
}

// ExchangeCode exchanges an authorisation code for a Token (06 §6 step 3,
// POST /openapi/apiManage/token via creds.Endpoints["token"] —
// isolarClient.ts:265-309 exchangeAuthCode). It goes through authCall (R32:
// exactly one attempt — a retried code exchange after the provider has
// already processed it fails the second time no matter what, and would
// burn the one-time code for nothing).
func (c *Client) ExchangeCode(ctx context.Context, creds integration.Credentials, code, redirectURI string) (Token, error) {
	raw, err := c.authCall(ctx, creds, opToken, map[string]any{
		"grant_type":   "authorization_code",
		"code":         code,
		"redirect_uri": redirectURI,
	})
	if err != nil {
		return Token{}, err
	}
	var w wireTokenData
	if err := json.Unmarshal(raw, &w); err != nil {
		return Token{}, malformedErr(opToken)
	}
	return tokenFromWire(w, c.clock.Now(), opToken)
}

// Refresh exchanges creds.Extra["refresh_token"] for a new Token (06 §6
// step 4, POST /openapi/apiManage/refreshToken via
// creds.Endpoints["refresh_token"] — isolarClient.ts:314-354
// refreshAccessToken). R32/authCall applies here for the exact reason 06
// §6 states: "iSolar refreshToken invalidates the previous token — a
// retry after a processed timeout burns it."
func (c *Client) Refresh(ctx context.Context, creds integration.Credentials) (Token, error) {
	rt := creds.Extra["refresh_token"]
	if rt.IsZero() {
		return Token{}, c.configError(opRefreshToken)
	}
	raw, err := c.authCall(ctx, creds, opRefreshToken, map[string]any{
		"grant_type":    "refresh_token",
		"refresh_token": rt.Reveal(),
	})
	if err != nil {
		return Token{}, err
	}
	var w wireTokenData
	if err := json.Unmarshal(raw, &w); err != nil {
		return Token{}, malformedErr(opRefreshToken)
	}
	return tokenFromWire(w, c.clock.Now(), opRefreshToken)
}

// authCall is the ONE helper every non-idempotent auth/refresh POST goes
// through (R32): it forces callOptions.noRetry, so token.go/auth.go never
// need to remember to set NoRetry themselves at each call site. Neither
// call carries the bearer header — 06 §6: the access token does not exist
// yet (token) or is being replaced (refreshToken).
func (c *Client) authCall(ctx context.Context, creds integration.Credentials, op string, body map[string]any) (json.RawMessage, error) {
	return c.call(ctx, creds, callOptions{op: op, bearer: false, noRetry: true, body: body})
}

// Verify proves creds can authenticate and reach the platform: 06 §6
// documents no dedicated "verify" operation, so — per the task brief —
// Verify calls queryPowerStationList with page 1, size 1 and discards the
// result, exactly like Plants but bounded to the smallest possible page.
func (c *Client) Verify(ctx context.Context, creds integration.Credentials) error {
	_, err := c.call(ctx, creds, callOptions{
		op:     opQueryPowerStationList,
		bearer: true,
		body:   map[string]any{"page": 1, "size": 1},
	})
	return err
}
