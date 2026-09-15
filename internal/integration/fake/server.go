// Package fake is the TLS test harness every provider adapter test runs
// against instead of a real endpoint (F2 global constraint: "tests never
// hit real provider endpoints"). NewTLSServer starts an httptest TLS server
// whose certificate is pinned exactly the way production pins a provider's
// self-signed certificate, via the same map[string]string shape
// httpx.PoolOptions.PinnedCerts takes.
package fake

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// RecordedRequest is one request the fake server observed, available via
// Server.Requests for assertions on what an adapter actually sent.
type RecordedRequest struct {
	Method, Path, RawQuery string
	Header                 http.Header
	Body                   []byte
}

// Responder writes a response for a matched route.
type Responder func(w http.ResponseWriter, r *http.Request)

// Route matches a request by method and exact path, with an optional
// predicate (Match) for query or body inspection.
type Route struct {
	Method, Path string
	Match        func(r *http.Request) bool
	Respond      Responder
}

// Server is a pinned-TLS fake provider endpoint.
type Server struct {
	// URL is https://127.0.0.1:<port>.
	URL string
	// Pins is {"127.0.0.1": base64(DER)} — pass to
	// httpx.PoolOptions.PinnedCerts.
	Pins map[string]string
	// Cert is this server's own certificate. Every NewTLSServer call mints
	// a fresh one (R4): two Server values are provably distinct
	// certificates, never the same one twice.
	Cert *x509.Certificate

	httptest *httptest.Server
	routes   []Route
	t        *testing.T

	mu       sync.Mutex
	requests []RecordedRequest
}

// NewTLSServer starts an httptest TLS server routed by method+path.
// An unmatched request is answered 404 and fails t via t.Errorf.
//
// EVERY CALL mints a fresh ECDSA self-signed certificate (IP SAN 127.0.0.1)
// via crypto/ecdsa and x509.CreateCertificate, with a fresh crypto/rand
// read, set directly on httptest.NewUnstartedServer(...).TLS before
// StartTLS. This is deliberate (F2 plan ruling R4): Go's own
// httptest.NewTLSServer reuses ONE built-in certificate across every server
// in the process, so two of its servers are byte-identical certificates —
// a test that pins server B's cert under server A's host and expects
// rejection could not observe anything with them, because it would
// actually be pinning server A's own certificate. Two NewTLSServer calls
// here are provably distinct; see TestTwoFakeServersHaveDistinctCertificates.
func NewTLSServer(t *testing.T, routes ...Route) *Server {
	t.Helper()

	cert, key, der := newSelfSignedCert(t)

	s := &Server{
		Cert:   cert,
		routes: routes,
		t:      t,
	}
	s.Pins = map[string]string{"127.0.0.1": base64.StdEncoding.EncodeToString(der)}

	ts := httptest.NewUnstartedServer(http.HandlerFunc(s.handle))
	ts.TLS = &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
	}
	ts.StartTLS()
	t.Cleanup(ts.Close)

	s.httptest = ts
	s.URL = ts.URL
	return s
}

// Requests returns every request the server has observed so far, in the
// order it received them.
func (s *Server) Requests() []RecordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RecordedRequest, len(s.requests))
	copy(out, s.requests)
	return out
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()
	// Restore the body so a Route's Match predicate can read it too.
	r.Body = io.NopCloser(bytes.NewReader(body))

	s.mu.Lock()
	s.requests = append(s.requests, RecordedRequest{
		Method:   r.Method,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
		Header:   r.Header.Clone(),
		Body:     body,
	})
	s.mu.Unlock()

	for _, route := range s.routes {
		if route.Method != r.Method || route.Path != r.URL.Path {
			continue
		}
		if route.Match != nil {
			r.Body = io.NopCloser(bytes.NewReader(body))
			if !route.Match(r) {
				continue
			}
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		route.Respond(w, r)
		return
	}

	s.t.Errorf("fake: unmatched request %s %s", r.Method, r.URL.Path)
	http.NotFound(w, r)
}

// newSelfSignedCert generates a fresh ECDSA key and a self-signed
// certificate for 127.0.0.1, per call — see NewTLSServer's doc for why this
// must never be cached.
func newSelfSignedCert(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey, []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "ekokod-fake-server"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	return cert, key, der
}
