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
// once (R22's legacy format).
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

// TestISolarAuthorizeURLRejectsUnknownRegion proves regionCloudID is
// actually consulted: an unrecognised Region is a config error, not a URL
// with a zero/garbage cloudId.
func TestISolarAuthorizeURLRejectsUnknownRegion(t *testing.T) {
	srv := fake.NewTLSServer(t)
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)
	creds.Region = "MARS"

	_, err := c.AuthorizeURL(creds, "https://app.example.invalid/callback")
	require.ErrorIs(t, err, integration.ErrAuth)
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
	require.ErrorIs(t, err, integration.ErrAuth)
	require.Empty(t, srv.Requests())
}

// TestISolarTokenCallsAreNeverRetried proves R32 end to end (adapter-
// patterns.md item 10): ExchangeCode and Refresh go through the ONE authCall
// helper with NoRetry forced, so a retryable server failure (500, which
// httpx classifies ErrUpstreamUnavailable and would normally retry up to
// MaxAttempts=3 times) still reaches the server EXACTLY ONCE.
//
// MUTATION PROOF (task brief Step 5): removing authCall's noRetry:true
// (setting it false) makes this test fail with reqCount == 3, not 1 — see
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
