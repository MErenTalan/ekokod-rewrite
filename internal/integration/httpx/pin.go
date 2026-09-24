package httpx

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// errPlainHTTPToPinnedHost is returned by RoundTrip when a request targets
// a pinned host over plain http:// (M13). A pinned host's protection lives
// entirely in the dedicated transport's TLSClientConfig; a plain-HTTP
// request to that same host never reaches TLS at all, so it would carry
// its credentials in the clear and bypass the pin completely if allowed
// through. classify.go treats this the same as a certificate verification
// failure: a deterministic misconfiguration, never retried in-client.
var errPlainHTTPToPinnedHost = errors.New("httpx: refusing to dial a pinned host over plain http")

// pinnedRoundTripper selects a transport by request host: a host present in
// PinnedCerts gets a dedicated *http.Transport whose RootCAs pool contains
// ONLY that host's own certificate; every other host
// shares one transport backed by the system root CAs. Both kinds of
// transport are built with MinVersion: tls.VersionTLS12, and TLS
// verification is never disabled anywhere in this file: neither transport's
// tls.Config sets the certificate-skip field, nor a custom
// VerifyPeerCertificate/VerifyConnection callback.
type pinnedRoundTripper struct {
	// perHost is one *http.Transport per pinned host, each trusting only
	// its own pin — never a pool built from every pin together, which
	// would let host A's presented certificate verify against host B's
	// pin (Step 5(b)'s mutation).
	perHost map[string]http.RoundTripper
	shared  http.RoundTripper
	// pinnedHosts is the same key set as perHost, kept separately only so
	// RoundTrip can check host membership without a map-lookup-plus-zero
	// value ambiguity; it is always built alongside perHost from the same
	// normalized keys.
	pinnedHosts map[string]bool
}

// normalizeHost lowercases host and strips one trailing '.' (a
// syntactically legal but semantically identical FQDN form), so a
// PinnedCerts key and an incoming request's Hostname() agree even when
// configured or requested with different case or a trailing dot (M13).
func normalizeHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(host), ".")
}

// dialContextFunc is the shape of (*net.Dialer).DialContext, and of the
// unexported test-only override below.
type dialContextFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// newPinnedRoundTripper builds one dedicated transport per (host, pin) and
// one shared transport for every other host. An invalid base64 payload or
// DER certificate is an error naming the offending host only. dialContext,
// when non-nil, overrides both kinds of transport's dialer — an unexported
// hook this package's own tests use (set only from a _test.go file in this
// package, via PoolOptions' unexported dialContext field) to route a
// minted test hostname to a loopback listener without needing real DNS.
// Production code always passes nil, which leaves Go's normal dialer in
// place.
func newPinnedRoundTripper(pins map[string]string, dialContext dialContextFunc) (*pinnedRoundTripper, error) {
	perHost := make(map[string]http.RoundTripper, len(pins))
	pinnedHosts := make(map[string]bool, len(pins))
	for rawHost, encoded := range pins {
		host := normalizeHost(rawHost)

		der, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("httpx: invalid pinned certificate for host %q", rawHost)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, fmt.Errorf("httpx: invalid pinned certificate for host %q", rawHost)
		}

		pool := x509.NewCertPool()
		pool.AddCert(cert)

		t := baseTransport(dialContext)
		t.TLSClientConfig = &tls.Config{
			RootCAs:    pool,
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		}
		perHost[host] = t
		pinnedHosts[host] = true
	}

	shared := baseTransport(dialContext)
	shared.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}

	return &pinnedRoundTripper{perHost: perHost, shared: shared, pinnedHosts: pinnedHosts}, nil
}

// baseTransport clones http.DefaultTransport (M12) so every transport this
// package builds keeps the standard library's own tuned connection-pool
// and timeout behaviour (IdleConnTimeout, TLSHandshakeTimeout,
// ExpectContinueTimeout, ForceAttemptHTTP2, MaxIdleConns...) instead of a
// bespoke, likely-worse-tuned set of zero values, and Proxy:
// http.ProxyFromEnvironment (ruling R33: on-prem deployments sit behind an
// HTTP(S) proxy; TLS and pinning stay end-to-end through the proxy's
// CONNECT tunnel regardless — the proxy only ever sees an opaque TLS
// stream to the pinned host, never the plaintext or the certificate
// check). Its own TLSClientConfig is always overwritten by the caller
// immediately after this returns.
func baseTransport(dialContext dialContextFunc) *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = http.ProxyFromEnvironment
	if dialContext != nil {
		t.DialContext = dialContext
	}
	return t
}

// RoundTrip dispatches by req.URL.Hostname(): the host's own pinned
// transport when one is configured, the shared system-root transport
// otherwise. A host never falls back to a DIFFERENT host's pinned
// transport. A pinned host reached over plain http:// is refused outright
// (M13) rather than silently handled as an accidental plaintext
// connection that bypasses the pin entirely.
func (p *pinnedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	host := normalizeHost(req.URL.Hostname())

	if p.pinnedHosts[host] {
		if req.URL.Scheme != "https" {
			return nil, errPlainHTTPToPinnedHost
		}
		return p.perHost[host].RoundTrip(req)
	}
	return p.shared.RoundTrip(req)
}
