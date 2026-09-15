package fake_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration/fake"
)

// pinnedClient builds an *http.Client that trusts only the certificates in
// pins, the same way httpx.Pool (Task 2) is meant to build its transport
// from httpx.PoolOptions.PinnedCerts.
func pinnedClient(t *testing.T, pins map[string]string) *http.Client {
	t.Helper()

	pool := x509.NewCertPool()
	for _, b64 := range pins {
		der, err := base64.StdEncoding.DecodeString(b64)
		require.NoError(t, err)
		cert, err := x509.ParseCertificate(der)
		require.NoError(t, err)
		pool.AddCert(cert)
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
	}
}

// getCtx issues a GET through client with an explicit context, satisfying
// the noctx linter that forbids the context-less client.Get shortcut.
func getCtx(t *testing.T, client *http.Client, url string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	require.NoError(t, err)
	return client.Do(req)
}

func TestFakeServerIsTLSAndRecordsRequests(t *testing.T) {
	s := fake.NewTLSServer(t, fake.Route{
		Method:  http.MethodGet,
		Path:    "/status",
		Respond: fake.JSON(http.StatusOK, []byte(`{"ok":true}`)),
	})

	require.True(t, len(s.URL) > 8 && s.URL[:8] == "https://", "URL must be https, got %s", s.URL)

	// A client that trusts only the server's pinned certificate succeeds.
	client := pinnedClient(t, s.Pins)
	resp, err := getCtx(t, client, s.URL+"/status?x=1")
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, resp.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, `{"ok":true}`, string(body))

	// A plain client with no pin fails with an x509 error: tests cannot
	// pass here without pinning.
	plain := &http.Client{}
	plainResp, err := getCtx(t, plain, s.URL+"/status")
	if plainResp != nil {
		_ = plainResp.Body.Close()
	}
	require.Error(t, err)
	var unknownAuth x509.UnknownAuthorityError
	require.ErrorAs(t, err, &unknownAuth, "expected an x509 unknown-authority error, got %v", err)

	requests := s.Requests()
	require.Len(t, requests, 1, "the unpinned client aborts during the TLS handshake, before any HTTP request reaches the handler, so only the pinned request should have landed")
}

// TestTwoFakeServersHaveDistinctCertificates proves R4: two NewTLSServer
// calls never share a certificate, and each server's own Pins value decodes
// back to its own Cert.Raw.
func TestTwoFakeServersHaveDistinctCertificates(t *testing.T) {
	a := fake.NewTLSServer(t)
	b := fake.NewTLSServer(t)

	require.False(t, bytes.Equal(a.Cert.Raw, b.Cert.Raw), "two fake servers must never share a certificate")

	for _, s := range []*fake.Server{a, b} {
		pin, ok := s.Pins["127.0.0.1"]
		require.True(t, ok)
		der, err := base64.StdEncoding.DecodeString(pin)
		require.NoError(t, err)
		require.True(t, bytes.Equal(der, s.Cert.Raw), "the server's own pin must decode back to its own certificate")
	}
}

func TestSequenceRepeatsLastResponder(t *testing.T) {
	s := fake.NewTLSServer(t, fake.Route{
		Method: http.MethodGet,
		Path:   "/seq",
		Respond: fake.Sequence(
			fake.JSON(http.StatusOK, []byte(`{"n":1}`)),
			fake.JSON(http.StatusOK, []byte(`{"n":2}`)),
		),
	})
	client := pinnedClient(t, s.Pins)

	var bodies []string
	for i := 0; i < 4; i++ {
		resp, err := getCtx(t, client, s.URL+"/seq")
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, resp.Body.Close())
		require.NoError(t, err)
		bodies = append(bodies, string(body))
	}

	require.Equal(t, []string{`{"n":1}`, `{"n":2}`, `{"n":2}`, `{"n":2}`}, bodies,
		"the sequence's last responder must repeat once exhausted")
}

// unmatchedHelperEnv, when set to "1", tells this test binary to run the
// unmatched-request scenario itself (the "recorded t") instead of
// re-exec'ing — see TestUnmatchedRouteFailsTheTest.
const unmatchedHelperEnv = "FAKE_UNMATCHED_HELPER"

// TestUnmatchedRouteFailsTheTest proves that hitting a fake.Server with no
// matching Route fails the *testing.T it was built with. t.Run cannot show
// this on its own: a failed subtest also marks its PARENT (this test)
// failed, which is not what we want to assert here — we want THIS test to
// PASS by observing that the unmatched request fails A test.
//
// So the scenario runs as a genuinely separate process: this test re-execs
// its own compiled binary with -test.run pinned to itself and
// FAKE_UNMATCHED_HELPER=1 set, which makes that child process take the
// helper branch below instead of recursing. The child is a real `go test`
// run of the unmatched-route scenario; its exit code and t.Errorf output
// are ordinary process exit status and stdout, fully isolated from this
// (parent) test's own pass/fail.
func TestUnmatchedRouteFailsTheTest(t *testing.T) {
	if os.Getenv(unmatchedHelperEnv) == "1" {
		s := fake.NewTLSServer(t) // no routes: every request is unmatched
		client := pinnedClient(t, s.Pins)

		resp, err := getCtx(t, client, s.URL+"/nope")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		_ = resp.Body.Close()
		return
	}

	cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=^TestUnmatchedRouteFailsTheTest$", "-test.v")
	cmd.Env = append(os.Environ(), unmatchedHelperEnv+"=1")
	out, err := cmd.CombinedOutput()

	require.Error(t, err, "the child process must exit non-zero: an unmatched route must fail its test\noutput:\n%s", out)
	require.Contains(t, string(out), "fake: unmatched request", "expected the unmatched-request failure message in child output:\n%s", out)
}
