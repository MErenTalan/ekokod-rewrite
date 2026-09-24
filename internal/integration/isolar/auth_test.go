package isolar_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/fake"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
)

// TestISolarAuthorizeURLFormat: exact string for region EU with a fixture
// app id and a redirect carrying "?state=" — the redirect is query-escaped
// once (R22's legacy format, isolarClient.ts:676-684).
func TestISolarAuthorizeURLFormat(t *testing.T) {
	srv := fake.NewTLSServer(t)
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)

	redirect := "https://app.example.invalid/integrations/isolar/callback?state=FIXTURE-STATE-abc123"
	got, err := c.AuthorizeURL(creds, redirect)
	require.NoError(t, err)

	want := "https://web3.isolarcloud.eu/#/authorized-app?cloudId=3&applicationId=" +
		url.QueryEscape(fixtureAppID) + "&redirectUrl=" + url.QueryEscape(redirect)
	require.Equal(t, want, got)

	// The redirect is escaped EXACTLY once: its own "?" and "=" survive as
	// %3F/%3D (one round of escaping), never doubled into %253F.
	require.Contains(t, got, "redirectUrl="+url.QueryEscape(redirect))
	require.NotContains(t, got, "%253F")
}

// TestISolarAuthorizeURLRequiresCloudID: R40 — cloud_id comes from
// creds.Endpoints (integration_definitions.json's isolar row), never a
// per-Region constant table, so a credential whose Endpoints carries no
// cloud_id is a config error, not a URL with a zero/garbage cloudId. (This
// replaces round 1's TestISolarAuthorizeURLRejectsUnknownRegion, whose
// premise — that AuthorizeURL resolves cloud_id FROM creds.Region via a
// hard-coded map — is exactly what R40 forbids; creds.Region is no longer
// read by this package at all.)
func TestISolarAuthorizeURLRequiresCloudID(t *testing.T) {
	srv := fake.NewTLSServer(t)
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)
	delete(creds.Endpoints, isolarEndpointCloudID)

	_, err := c.AuthorizeURL(creds, "https://app.example.invalid/callback")
	require.ErrorIs(t, err, integration.ErrConfig) // R48/I5
	require.NotErrorIs(t, err, integration.ErrAuth)
}

// TestISolarExchangeAndRefreshParseTokens: expires_in absent -> +7200s from
// the fake clock (isolar_refresh.json has no expires_in field); present ->
// exactly that many seconds (isolar_token.json's 3600).
func TestISolarExchangeAndRefreshParseTokens(t *testing.T) {
	srv := fake.NewTLSServer(t,
		fake.Route{Method: http.MethodPost, Path: "/openapi/apiManage/token", Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_token.json"))},
		fake.Route{Method: http.MethodPost, Path: "/openapi/apiManage/refreshToken", Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_refresh.json"))},
	)
	fakeClock := clock.NewFake(fixtureFrom)
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: fakeClock})
	creds := isolarTestCreds(srv)

	tok, err := c.ExchangeCode(context.Background(), creds, "FIXTURE-CODE-1", "https://app.example.invalid/callback")
	require.NoError(t, err)
	require.Equal(t, "FIXTURE-ACCESS-9f21", tok.AccessToken.Reveal())
	require.Equal(t, "FIXTURE-REFRESH-4b77", tok.RefreshToken.Reveal())
	require.True(t, tok.ExpiresAt.Equal(fixtureFrom.Add(3600*time.Second)))

	refreshed, err := c.Refresh(context.Background(), creds)
	require.NoError(t, err)
	require.Equal(t, "FIXTURE-ACCESS-b204", refreshed.AccessToken.Reveal())
	require.True(t, refreshed.ExpiresAt.Equal(fixtureFrom.Add(7200*time.Second)),
		"expires_in absent must default to 7200s from the fake clock")
}

// TestISolarRefreshRequiresRefreshToken: Refresh refuses before sending a
// request when creds.Extra["refresh_token"] is missing.
func TestISolarRefreshRequiresRefreshToken(t *testing.T) {
	srv := fake.NewTLSServer(t)
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)
	delete(creds.Extra, "refresh_token")

	_, err := c.Refresh(context.Background(), creds)
	require.ErrorIs(t, err, integration.ErrConfig) // R48/I5
	require.NotErrorIs(t, err, integration.ErrAuth)
	require.Empty(t, srv.Requests())
}

// TestISolarTokenCallsAreNeverRetried proves R32 end to end (adapter-
// patterns.md item 10): ExchangeCode and Refresh go through the ONE authCall
// helper with NoRetry forced, so a retryable server failure (500, which
// httpx classifies ErrUpstreamUnavailable and would normally retry up to
// MaxAttempts=3 times) still reaches the server EXACTLY ONCE.
//
// MUTATION PROOF (task brief Step 5): removing authCall's noRetry:true
// (setting it false) makes this test fail with reqCount == 6, not 2 — see
// the task report for the observed failure output.
func TestISolarTokenCallsAreNeverRetried(t *testing.T) {
	srv := fake.NewTLSServer(t,
		fake.Route{Method: http.MethodPost, Path: "/openapi/apiManage/token", Respond: fake.Raw(http.StatusInternalServerError, "application/json", []byte(`{}`))},
		fake.Route{Method: http.MethodPost, Path: "/openapi/apiManage/refreshToken", Respond: fake.Raw(http.StatusInternalServerError, "application/json", []byte(`{}`))},
	)
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)

	_, err := c.ExchangeCode(context.Background(), creds, "FIXTURE-CODE-1", "https://app.example.invalid/callback")
	require.Error(t, err)
	_, err = c.Refresh(context.Background(), creds)
	require.Error(t, err)

	reqs := srv.Requests()
	require.Len(t, reqs, 2, "exactly one attempt per call: NoRetry must suppress httpx's normal 3-attempt retry")
}

// TestISolarExchangeCodeRejectsEmptyAccessToken: I4 — an access_token
// missing/empty on an otherwise-success envelope is ErrAuth (isolarClient.ts's
// own "No access_token in response" failure mode), never a Token whose
// Secret silently reveals "".
func TestISolarExchangeCodeRejectsEmptyAccessToken(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{
		Method: http.MethodPost, Path: "/openapi/apiManage/token",
		Respond: fake.JSON(http.StatusOK, []byte(`{"result_code":"1","result_msg":"success","result_data":{"access_token":"","refresh_token":"x"}}`)),
	})
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)

	_, err := c.ExchangeCode(context.Background(), creds, "FIXTURE-CODE-1", "https://app.example.invalid/callback")
	require.ErrorIs(t, err, integration.ErrAuth)
}

// TestISolarRefreshExposesZeroRefreshTokenWhenAbsent: I4 — a null (or
// missing) refresh_token on a successful refresh must not be silently
// dropped or defaulted; Token.RefreshToken.IsZero() must be true so Task 14
// knows to KEEP the prior refresh token rather than overwrite it with an
// empty one (see token.go's Token.RefreshToken doc).
func TestISolarRefreshExposesZeroRefreshTokenWhenAbsent(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{
		Method: http.MethodPost, Path: "/openapi/apiManage/refreshToken",
		Respond: fake.JSON(http.StatusOK, []byte(`{"result_code":"1","result_msg":"success","result_data":{"access_token":"FIXTURE-ACCESS-new","refresh_token":null}}`)),
	})
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)

	tok, err := c.Refresh(context.Background(), creds)
	require.NoError(t, err)
	require.Equal(t, "FIXTURE-ACCESS-new", tok.AccessToken.Reveal())
	require.True(t, tok.RefreshToken.IsZero(), "a null refresh_token must expose a zero Secret, never a fabricated or dropped-but-non-zero one")
}

// TestISolarResultDataNullIsMalformed: I4 — a success envelope
// (result_code "1") whose result_data is JSON null is ErrMalformedPayload,
// never silently decoded into a zero-value Token/empty list.
func TestISolarResultDataNullIsMalformed(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{
		Method: http.MethodPost, Path: "/openapi/apiManage/token",
		Respond: fake.JSON(http.StatusOK, []byte(`{"result_code":"1","result_msg":"success","result_data":null}`)),
	})
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)

	_, err := c.ExchangeCode(context.Background(), creds, "FIXTURE-CODE-1", "https://app.example.invalid/callback")
	require.ErrorIs(t, err, integration.ErrMalformedPayload)
}
