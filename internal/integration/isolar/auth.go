package isolar

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
)

// opAuthorize/opToken/opRefreshToken/opVerify are the creds.Endpoints keys
// this file's calls resolve their template from — see auth.go/plants.go's
// callers for opQueryPowerStationList, which Verify reuses.
const (
	opAuthorize    = "authorize"
	opToken        = "token"
	opRefreshToken = "refreshToken"
)

// AuthorizeURL builds the iSolarCloud authorisation-request URL (06 §6
// "Authorisation flow" step 1, R22's exact legacy format):
//
//	{authorize_origin}/#/authorized-app?cloudId={cloud_id}&applicationId={app_id}&redirectUrl={redirect}
//
// creds.Endpoints["authorize"] supplies the origin (region-specific, e.g.
// https://web3.isolarcloud.eu for EU); regionCloudID resolves creds.Region
// to its Cloud id (06 §6's Regions table — fixed platform knowledge, not an
// endpoint template); app_id comes from creds.Extra["app_id"].
//
// redirectURI is taken EXACTLY as given and query-escaped once. R22's
// HMAC-signed state ("redirectUrl=<callback>?state=<signed>") is produced
// and verified by Task 14's credential service, not by this package: this
// Client has no access to EKOKOD_JWT_SIGNING_KEY (it would break 06 §1 rule
// 2's purity — no config/store dependency), so the caller is expected to
// hand redirectURI already carrying its own "?state=..." query when one is
// wanted. This function's only job is the R22 string format.
func (c *Client) AuthorizeURL(creds integration.Credentials, redirectURI string) (string, error) {
	origin, ok := creds.Endpoints[opAuthorize]
	if !ok || origin == "" {
		return "", c.configError(opAuthorize)
	}
	cloudID, err := c.requireRegion(creds)
	if err != nil {
		return "", err
	}
	appID := creds.Extra["app_id"]
	if appID.IsZero() {
		return "", c.configError(opAuthorize)
	}

	return origin + "/#/authorized-app?cloudId=" + strconv.Itoa(cloudID) +
		"&applicationId=" + url.QueryEscape(appID.Reveal()) +
		"&redirectUrl=" + url.QueryEscape(redirectURI), nil
}

// ExchangeCode exchanges an authorisation code for a Token (06 §6 step 3,
// POST /openapi/apiManage/token via creds.Endpoints["token"]). It goes
// through authCall (R32: exactly one attempt — a retried code exchange
// after the provider has already processed it fails the second time no
// matter what, and would burn the one-time code for nothing).
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
	return tokenFromWire(w, c.clock.Now())
}

// Refresh exchanges creds.Extra["refresh_token"] for a new Token (06 §6
// step 4, POST /openapi/apiManage/refreshToken via
// creds.Endpoints["refreshToken"]). R32/authCall applies here for the exact
// reason 06 §6 states: "iSolar refreshToken invalidates the previous token
// — a retry after a processed timeout burns it."
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
	return tokenFromWire(w, c.clock.Now())
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
		body:   map[string]any{"curPage": 1, "size": 1},
	})
	return err
}
