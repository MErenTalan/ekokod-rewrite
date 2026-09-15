package httpx

// TestPinOnlyTrustDoesNotFallBackToSystemRoots (I5-ii) proves a pinned
// host's trust is EXACTLY its pin — never ALSO the system root CAs — even
// when a perfectly valid, system-trusted certificate chain exists for that
// same host. This is an in-package (`package httpx`, not httpx_test) test
// because it needs the unexported PoolOptions.dialContext hook: the
// fixture certificates below carry DNS SANs for names that do not (and
// must not) resolve via real DNS, so the client has to be told, out of
// band, which loopback listener each fixture hostname actually reaches.
//
// The property can only be observed from a FRESH process: Go's system
// root pool (consulted via SSL_CERT_FILE on this platform) is read and
// cached once per process the first time anything asks for it, so setting
// SSL_CERT_FILE from within an already-running `go test` binary — one
// that has almost certainly already made a TLS connection in some earlier
// test — would silently do nothing. The parent test below therefore mints
// the fixtures, starts the two TLS listeners, and re-executes ITSELF as a
// subprocess with SSL_CERT_FILE set before that subprocess has ever
// touched the system roots; the subprocess (gated by an env var so a
// plain `go test` never takes this branch on its own) does the actual
// dialling and reports its two outcomes on stdout for the parent to
// assert on, under a timeout.
import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big" //nolint:depguard // X.509 SerialNumber is *big.Int per the stdlib API; this is a certificate serial, never money/energy arithmetic
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"crypto/tls"

	"github.com/stretchr/testify/require"
)

// pinOnlyHelperEnv, when set to any non-empty value, switches
// TestPinOnlyTrustDoesNotFallBackToSystemRoots from "spawn a subprocess"
// mode into "I AM the subprocess: dial the two fixture hosts named by the
// other HTTPX_PINONLY_* env vars and report the outcome" mode. A plain
// `go test` run never sets this, so the helper branch never runs on its
// own — only ever underneath the parent test's own exec.Command.
const pinOnlyHelperEnv = "HTTPX_PINONLY_HELPER"

func TestPinOnlyTrustDoesNotFallBackToSystemRoots(t *testing.T) {
	if os.Getenv(pinOnlyHelperEnv) != "" {
		runPinOnlyHelper(t)
		return
	}

	caCert, caKey, caDER := mintFixtureCA(t)
	caPath := writeFixtureCertPEM(t, caDER)

	const pinnedHost = "pinned.httpx-pinonly-fixture.test"
	const unpinnedHost = "unpinned.httpx-pinonly-fixture.test"

	_, pinnedLeafKey, pinnedLeafDER := mintFixtureLeaf(t, pinnedHost, caCert, caKey)
	_, unpinnedLeafKey, unpinnedLeafDER := mintFixtureLeaf(t, unpinnedHost, caCert, caKey)

	pinnedSrv := startFixtureTLSListener(t, pinnedLeafDER, pinnedLeafKey)
	unpinnedSrv := startFixtureTLSListener(t, unpinnedLeafDER, unpinnedLeafKey)

	// The pinned host's configured pin is deliberately WRONG — it names
	// the *other* fixture's certificate, not its own. The certificate the
	// pinned listener actually presents is perfectly valid under the
	// system roots once SSL_CERT_FILE is honoured, so if pin-only trust
	// is broken (a pinned host ALSO falls back to the system roots), this
	// connection would wrongly succeed anyway; only a genuinely pin-only
	// implementation refuses it.
	wrongPinB64 := base64.StdEncoding.EncodeToString(unpinnedLeafDER)

	// A dedicated timeout context, not a bare exec.Command (noctx): the
	// helper subprocess is killed outright if it somehow hangs, rather
	// than relying solely on the select-with-timer fallback below to
	// notice and Kill() it after the fact.
	spawnCtx, cancelSpawn := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelSpawn()

	cmd := exec.CommandContext(spawnCtx, os.Args[0], "-test.run=^TestPinOnlyTrustDoesNotFallBackToSystemRoots$", "-test.v")
	cmd.Env = append(os.Environ(),
		pinOnlyHelperEnv+"=1",
		"SSL_CERT_FILE="+caPath,
		"HTTPX_PINONLY_PIN_HOST="+pinnedHost,
		"HTTPX_PINONLY_PIN_CERT_B64="+wrongPinB64,
		"HTTPX_PINONLY_PINNED_ADDR="+fixtureListenerAddr(pinnedSrv),
		"HTTPX_PINONLY_UNPINNED_HOST="+unpinnedHost,
		"HTTPX_PINONLY_UNPINNED_ADDR="+fixtureListenerAddr(unpinnedSrv),
	)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	require.NoError(t, cmd.Start())

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case waitErr := <-done:
		t.Logf("subprocess output:\n%s", out.String())
		require.NoError(t, waitErr, "helper subprocess exited non-zero")
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("helper subprocess did not finish within 20s")
	}

	require.Contains(t, out.String(), "PINNED_REFUSED_OK",
		"a pinned host whose pin does not match must be refused even though its certificate chains to a system-trusted CA")
	require.Contains(t, out.String(), "UNPINNED_ACCEPTED_OK",
		"positive control: an UNPINNED host with a CA-signed leaf must be accepted via system roots — proves SSL_CERT_FILE is actually taking effect in this subprocess, not merely refusing everything")
}

// runPinOnlyHelper is the subprocess body: read the fixture addresses and
// names from the environment, dial both hosts through an unexported
// dialContext hook that routes each fixture hostname to its real loopback
// listener, and print one fixed-format line per outcome.
func runPinOnlyHelper(t *testing.T) {
	pinHost := os.Getenv("HTTPX_PINONLY_PIN_HOST")
	pinCertB64 := os.Getenv("HTTPX_PINONLY_PIN_CERT_B64")
	pinnedAddr := os.Getenv("HTTPX_PINONLY_PINNED_ADDR")
	unpinnedHost := os.Getenv("HTTPX_PINONLY_UNPINNED_HOST")
	unpinnedAddr := os.Getenv("HTTPX_PINONLY_UNPINNED_ADDR")

	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		switch host {
		case pinHost:
			return (&net.Dialer{}).DialContext(ctx, network, pinnedAddr)
		case unpinnedHost:
			return (&net.Dialer{}).DialContext(ctx, network, unpinnedAddr)
		default:
			return nil, fmt.Errorf("pinonly helper: unexpected dial target")
		}
	}

	pinnedPool, err := NewPool(PoolOptions{
		PinnedCerts: map[string]string{pinHost: pinCertB64},
		dialContext: dial,
	})
	if err != nil {
		fmt.Println("PINNED_SETUP_ERROR")
		return
	}
	_, err = pinnedPool.Client(ClientConfig{Provider: "aril", LimiterKey: "pinned", Every: 0, Burst: 1, MaxAttempts: 1}).
		Do(context.Background(), Request{Op: "op", Method: http.MethodGet, Template: "https://" + pinHost + "/"})
	if err != nil {
		fmt.Println("PINNED_REFUSED_OK")
	} else {
		fmt.Println("PINNED_WRONGLY_ACCEPTED")
	}

	// No PinnedCerts entry for unpinnedHost at all: every request to it
	// falls through to the shared, system-root transport.
	unpinnedPool, err := NewPool(PoolOptions{dialContext: dial})
	if err != nil {
		fmt.Println("UNPINNED_SETUP_ERROR")
		return
	}
	_, err = unpinnedPool.Client(ClientConfig{Provider: "aril", LimiterKey: "unpinned", Every: 0, Burst: 1, MaxAttempts: 1}).
		Do(context.Background(), Request{Op: "op", Method: http.MethodGet, Template: "https://" + unpinnedHost + "/"})
	if err != nil {
		fmt.Println("UNPINNED_WRONGLY_REFUSED")
	} else {
		fmt.Println("UNPINNED_ACCEPTED_OK")
	}
}

// --- fixture minting helpers (this package's own, per I5-ii's note that
// internal/integration/fake is not to be touched for this) ---

func mintFixtureCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "httpx-pinonly-fixture-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert, key, der
}

func mintFixtureLeaf(t *testing.T, dnsName string, ca *x509.Certificate, caKey *ecdsa.PrivateKey) (*x509.Certificate, *ecdsa.PrivateKey, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: dnsName},
		DNSNames:     []string{dnsName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert, key, der
}

func writeFixtureCertPEM(t *testing.T, der []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "httpx-pinonly-ca.pem")
	f, err := os.Create(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	require.NoError(t, pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: der}))
	return path
}

func startFixtureTLSListener(t *testing.T, certDER []byte, key *ecdsa.PrivateKey) *httptest.Server {
	t.Helper()
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ts.TLS = &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{certDER}, PrivateKey: key}},
	}
	ts.StartTLS()
	t.Cleanup(ts.Close)
	return ts
}

func fixtureListenerAddr(ts *httptest.Server) string {
	return ts.Listener.Addr().String()
}
