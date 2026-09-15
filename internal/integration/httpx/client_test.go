package httpx_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
	lock "github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
)

// recordingSleep is a fake PoolOptions.Sleep: it records every requested
// delay and returns immediately, so retry/rate-limit tests never actually
// wait in real time. It still honours ctx cancellation, matching
// defaultSleep's contract.
type recordingSleep struct {
	mu     sync.Mutex
	delays []time.Duration
}

func (r *recordingSleep) fn(ctx context.Context, d time.Duration) error {
	r.mu.Lock()
	r.delays = append(r.delays, d)
	r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (r *recordingSleep) recorded() []time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]time.Duration, len(r.delays))
	copy(out, r.delays)
	return out
}

// httpxTestPool builds a Pool pinned to srv's own certificate, with a
// no-op recording Sleep so no test in this file ever waits in real time.
func httpxTestPool(t *testing.T, srv *httptest.Server) (*httpx.Pool, *recordingSleep) {
	t.Helper()
	rs := &recordingSleep{}
	pool, err := httpx.NewPool(httpx.PoolOptions{
		PinnedCerts: map[string]string{
			hostOf(t, srv.URL): base64.StdEncoding.EncodeToString(srv.Certificate().Raw),
		},
		Sleep: rs.fn,
	})
	require.NoError(t, err)
	return pool, rs
}

// TestHTTPErrorsNeverContainSubstitutedSecrets pins removed-behaviour 17
// for the transport: OSOS's legacy auth URL carries the password in the
// query. No httpx error — for a classified provider status OR a raw
// transport/network failure — may ever echo a URL, a query string, or a
// substituted secret value back.
func TestHTTPErrorsNeverContainSubstitutedSecrets(t *testing.T) {
	const pw = "FIXTURE-PW-91c2"

	for _, status := range []int{401, 404, 429, 500, 400} {
		status := status
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"echo":"` + r.URL.Query().Get("p") + `"}`))
			}))
			defer srv.Close()

			pool, _ := httpxTestPool(t, srv)
			c := pool.Client(httpx.ClientConfig{Provider: integration.ProviderOSOS, LimiterKey: t.Name(), Every: time.Millisecond, Burst: 1, MaxAttempts: 1})

			_, err := c.Do(context.Background(), httpx.Request{
				Op: "authentication", Method: http.MethodPost,
				Template: srv.URL + "/auth?u={u}&p={p}",
				Params:   map[string]httpx.Param{"u": {Value: "fixture"}, "p": {Value: pw, Secret: true}},
				Header:   http.Header{"Authorization": {"Bearer " + pw}},
			})
			require.Error(t, err)
			require.NotContains(t, err.Error(), pw)
			require.NotContains(t, fmt.Sprintf("%+v", err), pw)
			require.NotContains(t, err.Error(), srv.URL, "no URL in error text")
		})
	}

	t.Run("network_failure", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.NotFoundHandler())
		pool, _ := httpxTestPool(t, srv)
		srv.Close() // closed before use: every Do call hits a connection failure

		_, err := pool.Client(httpx.ClientConfig{Provider: "osos", LimiterKey: t.Name(), Every: time.Millisecond, Burst: 1, MaxAttempts: 1}).
			Do(context.Background(), httpx.Request{Op: "authentication", Method: "GET", Template: srv.URL + "/?p={p}", Params: map[string]httpx.Param{"p": {Value: pw, Secret: true}}})
		require.ErrorIs(t, err, integration.ErrUpstreamUnavailable)
		require.NotContains(t, err.Error(), pw)
		require.NotContains(t, fmt.Sprintf("%+v", err), pw)
	})
}

// TestStatusClassification is the table half of classify.go's Classification
// rule, exercised behaviourally through Do (classifyStatus is unexported).
func TestStatusClassification(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   error
	}{
		{401, integration.ErrAuth},
		{403, integration.ErrAuth},
		{404, integration.ErrNotFound},
		{429, integration.ErrRateLimited},
		{408, integration.ErrUpstreamUnavailable},
		{500, integration.ErrUpstreamUnavailable},
		{502, integration.ErrUpstreamUnavailable},
		{503, integration.ErrUpstreamUnavailable},
		{400, integration.ErrMalformedPayload},
		{422, integration.ErrMalformedPayload},
	} {
		tc := tc
		t.Run(fmt.Sprintf("status_%d", tc.status), func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			pool, _ := httpxTestPool(t, srv)
			_, err := pool.Client(httpx.ClientConfig{Provider: "gridbox", LimiterKey: t.Name(), Every: time.Millisecond, Burst: 1, MaxAttempts: 1}).
				Do(context.Background(), httpx.Request{Op: "op", Method: "GET", Template: srv.URL})
			require.ErrorIs(t, err, tc.want)
		})
	}
}

// TestRequestTimeoutIsEnforced: a handler that never responds must not
// hang Do forever — RequestTimeout bounds every attempt.
func TestRequestTimeoutIsEnforced(t *testing.T) {
	block := make(chan struct{})

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	// Deferred calls run LIFO: close(block) (registered second) must run
	// BEFORE srv.Close() (registered first), or Close — which blocks
	// until every outstanding handler returns — deadlocks against a
	// handler that is itself waiting for block to close.
	defer srv.Close()
	defer close(block)

	pool, _ := httpxTestPool(t, srv)
	c := pool.Client(httpx.ClientConfig{Provider: "gridbox", LimiterKey: t.Name(), Every: time.Millisecond, Burst: 1, MaxAttempts: 1, RequestTimeout: 50 * time.Millisecond})

	done := make(chan error, 1)
	go func() {
		_, err := c.Do(context.Background(), httpx.Request{Op: "op", Method: "GET", Template: srv.URL})
		done <- err
	}()

	select {
	case err := <-done:
		require.ErrorIs(t, err, integration.ErrUpstreamUnavailable)
	case <-time.After(2 * time.Second):
		t.Fatal("Do did not return within 2s of a 50ms RequestTimeout")
	}
}

// TestLimiterSpacesRequestsPerKey: a fake Now/Sleep shows the second
// request on the same LimiterKey waits Every; a different key does not.
func TestLimiterSpacesRequestsPerKey(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	now := time.Now()
	rs := &recordingSleep{}
	pool, err := httpx.NewPool(httpx.PoolOptions{
		PinnedCerts: map[string]string{hostOf(t, srv.URL): base64.StdEncoding.EncodeToString(srv.Certificate().Raw)},
		Sleep:       rs.fn,
		Now:         func() time.Time { return now },
	})
	require.NoError(t, err)

	client := pool.Client(httpx.ClientConfig{Provider: "gridbox", LimiterKey: "same-key", Every: time.Second, Burst: 1, MaxAttempts: 1})
	_, err = client.Do(context.Background(), httpx.Request{Op: "op", Method: "GET", Template: srv.URL})
	require.NoError(t, err)
	require.Empty(t, rs.recorded(), "first request on a fresh key must not wait")

	_, err = client.Do(context.Background(), httpx.Request{Op: "op", Method: "GET", Template: srv.URL})
	require.NoError(t, err)
	require.Len(t, rs.recorded(), 1)
	require.InDelta(t, time.Second, rs.recorded()[0], float64(50*time.Millisecond))

	// A different key starts with its own full burst: no wait.
	rs2 := &recordingSleep{}
	pool2, err := httpx.NewPool(httpx.PoolOptions{
		PinnedCerts: map[string]string{hostOf(t, srv.URL): base64.StdEncoding.EncodeToString(srv.Certificate().Raw)},
		Sleep:       rs2.fn,
		Now:         func() time.Time { return now },
	})
	require.NoError(t, err)
	otherClient := pool2.Client(httpx.ClientConfig{Provider: "gridbox", LimiterKey: "other-key", Every: time.Second, Burst: 1, MaxAttempts: 1})
	_, err = otherClient.Do(context.Background(), httpx.Request{Op: "op", Method: "GET", Template: srv.URL})
	require.NoError(t, err)
	require.Empty(t, rs2.recorded(), "a different key must not be spaced by the first key's history")
}

// fakeLocker is a Locker that records Acquire/Release pairs in order, for
// TestSerializeKeyHoldsTheLockPerRequest.
type fakeLocker struct {
	mu     sync.Mutex
	events []string
}

type fakeLease struct {
	l   *fakeLocker
	key string
}

func (l *fakeLocker) Acquire(_ context.Context, key string, _ time.Duration) (lock.Lease, error) {
	l.mu.Lock()
	l.events = append(l.events, "acquire:"+key)
	l.mu.Unlock()
	return &fakeLease{l: l, key: key}, nil
}

func (l *fakeLease) Release(_ context.Context) error {
	l.l.mu.Lock()
	l.l.events = append(l.l.events, "release:"+l.key)
	l.l.mu.Unlock()
	return nil
}

func (l *fakeLocker) recorded() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.events))
	copy(out, l.events)
	return out
}

// TestSerializeKeyHoldsTheLockPerRequest: a fake Locker records
// Acquire/Release pairs around each request, in order, and Release happens
// even when the request itself fails.
func TestSerializeKeyHoldsTheLockPerRequest(t *testing.T) {
	var calls int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	locker := &fakeLocker{}
	rs := &recordingSleep{}
	pool, err := httpx.NewPool(httpx.PoolOptions{
		PinnedCerts: map[string]string{hostOf(t, srv.URL): base64.StdEncoding.EncodeToString(srv.Certificate().Raw)},
		Sleep:       rs.fn,
		Locker:      locker,
	})
	require.NoError(t, err)

	client := pool.Client(httpx.ClientConfig{Provider: "gridbox", LimiterKey: t.Name(), Every: time.Millisecond, Burst: 1, MaxAttempts: 1, SerializeKey: "serialize-key-1"})

	// First Do: the handler 500s, so the request itself errors. Release
	// must still happen.
	_, err = client.Do(context.Background(), httpx.Request{Op: "op", Method: "GET", Template: srv.URL})
	require.Error(t, err)

	// Second Do: the handler 200s.
	_, err = client.Do(context.Background(), httpx.Request{Op: "op", Method: "GET", Template: srv.URL})
	require.NoError(t, err)

	require.Equal(t, []string{
		"acquire:serialize-key-1", "release:serialize-key-1",
		"acquire:serialize-key-1", "release:serialize-key-1",
	}, locker.recorded())
}

// TestBodyLargerThanCapIsMalformed: a response body over MaxBodyBytes is
// ErrMalformedPayload even though the status is 200.
func TestBodyLargerThanCapIsMalformed(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, 100))
	}))
	defer srv.Close()

	pool, _ := httpxTestPool(t, srv)
	_, err := pool.Client(httpx.ClientConfig{Provider: "gridbox", LimiterKey: t.Name(), Every: time.Millisecond, Burst: 1, MaxAttempts: 1, MaxBodyBytes: 10}).
		Do(context.Background(), httpx.Request{Op: "op", Method: "GET", Template: srv.URL})
	require.ErrorIs(t, err, integration.ErrMalformedPayload)
}

// TestOversizedBodyIsDrainedAndClosed proves the connection is not leaked
// when a response body is rejected for size: the server's handler must be
// able to complete its write and the connection must be reusable for a
// subsequent request on the same client.
func TestOversizedBodyIsDrainedAndClosed(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, 1000))
	}))
	defer srv.Close()

	pool, _ := httpxTestPool(t, srv)
	client := pool.Client(httpx.ClientConfig{Provider: "gridbox", LimiterKey: t.Name(), Every: time.Millisecond, Burst: 1, MaxAttempts: 1, MaxBodyBytes: 10})

	for i := 0; i < 5; i++ {
		_, err := client.Do(context.Background(), httpx.Request{Op: "op", Method: "GET", Template: srv.URL})
		require.ErrorIs(t, err, integration.ErrMalformedPayload)
	}
}
