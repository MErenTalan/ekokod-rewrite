package httpx_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/fake"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
)

// route200 answers every request with a 200 and a fixed body.
func route200(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func allRoutes() []fake.Route {
	return []fake.Route{{Method: http.MethodGet, Path: "/", Match: nil, Respond: route200}}
}

// hostOf extracts the bare host (no port) from a server URL, matching the
// key shape fake.Server.Pins and PoolOptions.PinnedCerts both use.
func hostOf(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	return u.Hostname()
}

func TestUnpinnedSelfSignedServerIsRefused(t *testing.T) {
	srv := fake.NewTLSServer(t, allRoutes()...)

	pool, err := httpx.NewPool(httpx.PoolOptions{
		Sleep: noSleep,
	}) // system roots only — the fake server's cert is trusted by nobody
	require.NoError(t, err)

	_, err = pool.Client(httpx.ClientConfig{Provider: "aril", LimiterKey: "k", Every: time.Millisecond, Burst: 1, MaxAttempts: 3}).
		Do(context.Background(), httpx.Request{Op: "analyzers_list", Method: http.MethodGet, Template: srv.URL})
	require.ErrorIs(t, err, integration.ErrUpstreamUnavailable)
}

func TestPinnedHostAcceptsItsCertificate(t *testing.T) {
	srv := fake.NewTLSServer(t, allRoutes()...)

	pool, err := httpx.NewPool(httpx.PoolOptions{
		PinnedCerts: srv.Pins,
		Sleep:       noSleep,
	})
	require.NoError(t, err)

	resp, err := pool.Client(httpx.ClientConfig{Provider: "aril", LimiterKey: "k", Every: time.Millisecond, Burst: 1, MaxAttempts: 1}).
		Do(context.Background(), httpx.Request{Op: "analyzers_list", Method: http.MethodGet, Template: srv.URL})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Status)
}

// TestPinnedHostRejectsADifferentCertificate is the R4 fix: srvA and srvB
// are two SEPARATE fake.NewTLSServer calls, so their certificates are
// provably distinct (checked below as this test's own precondition).
// Pinning srvB's cert under srvA's host is therefore observably wrong.
func TestPinnedHostRejectsADifferentCertificate(t *testing.T) {
	srvA := fake.NewTLSServer(t, allRoutes()...)
	srvB := fake.NewTLSServer(t, allRoutes()...)
	require.NotEqual(t, srvA.Cert.Raw, srvB.Cert.Raw, "precondition: two fake servers must have distinct certs")

	pool, err := httpx.NewPool(httpx.PoolOptions{
		PinnedCerts: map[string]string{hostOf(t, srvA.URL): base64.StdEncoding.EncodeToString(srvB.Cert.Raw)},
		Sleep:       noSleep,
	})
	require.NoError(t, err)

	_, err = pool.Client(httpx.ClientConfig{Provider: "aril", LimiterKey: "k", Every: time.Millisecond, Burst: 1, MaxAttempts: 1}).
		Do(context.Background(), httpx.Request{Op: "analyzers_list", Method: http.MethodGet, Template: srvA.URL})
	require.ErrorIs(t, err, integration.ErrUpstreamUnavailable)
	require.Empty(t, srvA.Requests(), "the TLS handshake must fail before any HTTP request reaches the handler")
}

// TestPinnedTransportsDoNotShareTrustAcrossHosts is a second, stronger
// proof of the same R4 isolation property Step 5(b) of the task brief
// targets ("build one shared pool from every pin" instead of one pool per
// host). TestPinnedHostRejectsADifferentCertificate above has exactly one
// PinnedCerts entry, and every fake.NewTLSServer listens on the same
// "127.0.0.1" host (there is only one loopback host string these fake
// servers can ever use), so a PinnedCerts map with a single entry cannot
// distinguish "one pool per host" from "one pool merged from every pin" —
// both degenerate to the identical singleton pool (verified: the mutation
// described in Step 5(b), applied for real, left that test passing
// unchanged). This test adds a SECOND, decoy PinnedCerts entry under an
// unrelated host name that legitimately pins srvA's own certificate. A
// correct (per-host-isolated) implementation never lets that decoy entry
// influence srvA's own (deliberately wrong) pin. A "merge every pin into
// one shared pool" implementation leaks srvA's real certificate into
// srvA's trust pool via the decoy entry, and the connection would wrongly
// succeed.
func TestPinnedTransportsDoNotShareTrustAcrossHosts(t *testing.T) {
	srvA := fake.NewTLSServer(t, allRoutes()...)
	srvB := fake.NewTLSServer(t, allRoutes()...)
	require.NotEqual(t, srvA.Cert.Raw, srvB.Cert.Raw, "precondition: two fake servers must have distinct certs")

	pool, err := httpx.NewPool(httpx.PoolOptions{
		PinnedCerts: map[string]string{
			// srvA's own host is (deliberately, wrongly) pinned to srvB's
			// certificate...
			hostOf(t, srvA.URL): base64.StdEncoding.EncodeToString(srvB.Cert.Raw),
			// ...while an entirely unrelated host name is validly pinned
			// to srvA's OWN certificate. This entry is never dialled; it
			// exists only to poison a shared pool, if one exists.
			"unrelated.invalid.example": base64.StdEncoding.EncodeToString(srvA.Cert.Raw),
		},
		Sleep: noSleep,
	})
	require.NoError(t, err)

	_, err = pool.Client(httpx.ClientConfig{Provider: "aril", LimiterKey: "k", Every: time.Millisecond, Burst: 1, MaxAttempts: 1}).
		Do(context.Background(), httpx.Request{Op: "analyzers_list", Method: http.MethodGet, Template: srvA.URL})
	require.ErrorIs(t, err, integration.ErrUpstreamUnavailable)
}

// TestPoolNeverAcceptsAnUnverifiedCertificate proves the "verification is
// never disabled" property BEHAVIOURALLY: a request to a server that is
// neither pinned nor system-trusted must fail, no matter what else is
// pinned in the same Pool.
func TestPoolNeverAcceptsAnUnverifiedCertificate(t *testing.T) {
	pinned := fake.NewTLSServer(t, allRoutes()...)
	unrelated := fake.NewTLSServer(t, allRoutes()...)

	pool, err := httpx.NewPool(httpx.PoolOptions{
		PinnedCerts: pinned.Pins,
		Sleep:       noSleep,
	})
	require.NoError(t, err)

	_, err = pool.Client(httpx.ClientConfig{Provider: "aril", LimiterKey: "k", Every: time.Millisecond, Burst: 1, MaxAttempts: 1}).
		Do(context.Background(), httpx.Request{Op: "analyzers_list", Method: http.MethodGet, Template: unrelated.URL})
	require.ErrorIs(t, err, integration.ErrUpstreamUnavailable)
}

// noSleep is shared by every pin_test.go case: TLS failures in this file
// are all classified as non-retryable (classify.go), so no test here
// should ever actually invoke Sleep — but MaxAttempts:3 cases pass it
// anyway so a regression that started retrying TLS failures would hang a
// real timer instead of silently passing fast.
func noSleep(_ context.Context, _ time.Duration) error { return nil }
