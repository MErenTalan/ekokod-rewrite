package httpx

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net/http"
)

// pinnedRoundTripper selects a transport by request host: a host present in
// PinnedCerts gets a dedicated *http.Transport whose RootCAs pool contains
// ONLY that host's own certificate (never any other pin); every other host
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
}

// newPinnedRoundTripper builds one dedicated transport per (host, pin) and
// one shared transport for every other host. An invalid base64 payload or
// DER certificate is an error naming the offending host only.
func newPinnedRoundTripper(pins map[string]string) (*pinnedRoundTripper, error) {
	perHost := make(map[string]http.RoundTripper, len(pins))
	for host, encoded := range pins {
		der, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("httpx: invalid pinned certificate for host %q", host)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, fmt.Errorf("httpx: invalid pinned certificate for host %q", host)
		}

		pool := x509.NewCertPool()
		pool.AddCert(cert)

		perHost[host] = &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    pool,
				ServerName: host,
				MinVersion: tls.VersionTLS12,
			},
		}
	}

	shared := &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}

	return &pinnedRoundTripper{perHost: perHost, shared: shared}, nil
}

// RoundTrip dispatches by req.URL.Hostname(): the host's own pinned
// transport when one is configured, the shared system-root transport
// otherwise. A host never falls back to a DIFFERENT host's pinned
// transport.
func (p *pinnedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt, ok := p.perHost[req.URL.Hostname()]; ok {
		return rt.RoundTrip(req)
	}
	return p.shared.RoundTrip(req)
}
