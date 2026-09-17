package mail_test

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/mail"
)

type fakeSMTP struct {
	addr       string
	implicit   bool
	startTLS   bool
	mu         sync.Mutex
	commands   []string
	authBefore bool // AUTH arrived before TLS
	data       string
}

func (f *fakeSMTP) record(cmd string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commands = append(f.commands, cmd)
}

func (f *fakeSMTP) snapshot() ([]string, bool, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.commands...), f.authBefore, f.data
}

func testCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	require.NoError(t, err)
	parsed, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	pool.AddCert(parsed)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pool
}

func startFake(t *testing.T, implicit, startTLS bool, cert tls.Certificate) *fakeSMTP {
	t.Helper()
	cfg := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	var ln net.Listener
	var err error
	if implicit {
		ln, err = tls.Listen("tcp", "127.0.0.1:0", cfg)
	} else {
		ln, err = (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	f := &fakeSMTP{addr: ln.Addr().String(), implicit: implicit, startTLS: startTLS}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(conn, cfg)
		}
	}()
	return f
}

func (f *fakeSMTP) serve(conn net.Conn, cfg *tls.Config) {
	defer func() { _ = conn.Close() }()
	secure := f.implicit
	r := bufio.NewReader(conn)
	w := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
	w("220 localhost ESMTP fake")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.TrimRight(line, "\r\n")
		f.record(cmd)
		upper := strings.ToUpper(cmd)
		switch {
		case strings.HasPrefix(upper, "EHLO"):
			exts := []string{"250-localhost"}
			if f.startTLS && !secure {
				exts = append(exts, "250-STARTTLS")
			}
			exts = append(exts, "250 AUTH PLAIN")
			for _, e := range exts {
				w(e)
			}
		case upper == "STARTTLS":
			w("220 go ahead")
			tlsConn := tls.Server(conn, cfg)
			if tlsConn.HandshakeContext(context.Background()) != nil {
				return
			}
			conn = tlsConn
			r = bufio.NewReader(conn)
			secure = true
		case strings.HasPrefix(upper, "AUTH"):
			if !secure {
				f.mu.Lock()
				f.authBefore = true
				f.mu.Unlock()
			}
			w("235 ok")
		case strings.HasPrefix(upper, "MAIL FROM"), strings.HasPrefix(upper, "RCPT TO"):
			w("250 ok")
		case upper == "DATA":
			w("354 send")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			f.mu.Lock()
			f.data = b.String()
			f.mu.Unlock()
			w("250 queued")
		case upper == "QUIT":
			w("221 bye")
			return
		default:
			w("250 ok")
		}
	}
}

func settingsFor(t *testing.T, addr string, secure bool) model.SMTPSettings {
	t.Helper()
	host, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	var p int32
	for _, c := range port {
		p = p*10 + (c - '0')
	}
	return model.SMTPSettings{Host: host, Port: p, Secure: secure, Username: "mailer", FromAddress: "bildirim@ekokod.com.tr"}
}

func TestSMTPSenderImplicitTLS(t *testing.T) {
	cert, roots := testCert(t)
	f := startFake(t, true, false, cert)
	s := mail.NewSMTPSender(5*time.Second, roots)
	msg := mail.PasswordReset("tr", "Ayşe", "https://app.example/auth/reset-password?token=t")
	msg.To = []string{"alici@example.com"}
	require.NoError(t, s.Send(context.Background(), settingsFor(t, f.addr, true), []byte("s3cret"), msg))

	cmds, authBefore, data := f.snapshot()
	require.False(t, authBefore)
	require.Contains(t, cmds, "MAIL FROM:<bildirim@ekokod.com.tr>")
	require.Contains(t, cmds, "RCPT TO:<alici@example.com>")
	require.Contains(t, data, "Subject: =?utf-8?q?=C5=9Eifre_s=C4=B1f=C4=B1rlama?=", "non-ASCII subjects are RFC 2047 encoded")
	require.Contains(t, data, "Content-Type: text/plain; charset=utf-8")
	for _, c := range cmds {
		if strings.HasPrefix(c, "AUTH PLAIN ") {
			raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(c, "AUTH PLAIN "))
			require.NoError(t, err)
			require.Equal(t, "\x00mailer\x00s3cret", string(raw))
		}
	}
}

func TestSMTPSenderStartTLS(t *testing.T) {
	cert, roots := testCert(t)
	f := startFake(t, false, true, cert)
	msg := mail.PasswordReset("en", "Ada", "https://app.example/auth/reset-password?token=abc")
	msg.To = []string{"ada@example.com"}
	require.NoError(t, mail.NewSMTPSender(5*time.Second, roots).Send(context.Background(), settingsFor(t, f.addr, false), []byte("pw"), msg))
	cmds, authBefore, data := f.snapshot()
	require.Contains(t, cmds, "STARTTLS")
	require.False(t, authBefore, "AUTH must only be sent after STARTTLS")
	require.Contains(t, data, "reset-password?token=3Dabc", "body is quoted-printable")
}

func TestSMTPSenderRejectsUntrustedCertificate(t *testing.T) {
	cert, _ := testCert(t)
	_, otherRoots := testCert(t)
	f := startFake(t, true, false, cert)
	msg := mail.SMTPTest("tr")
	msg.To = []string{"a@example.com"}
	err := mail.NewSMTPSender(5*time.Second, otherRoots).Send(context.Background(), settingsFor(t, f.addr, true), []byte("pw"), msg)
	require.Error(t, err)
	cmds, _, _ := f.snapshot()
	for _, c := range cmds {
		require.False(t, strings.HasPrefix(c, "AUTH"), "no credentials may be sent over an unverified connection")
	}
}

func TestSMTPSenderRefusesAuthWithoutTLS(t *testing.T) {
	cert, roots := testCert(t)
	f := startFake(t, false, false, cert)
	msg := mail.SMTPTest("tr")
	msg.To = []string{"a@example.com"}
	err := mail.NewSMTPSender(5*time.Second, roots).Send(context.Background(), settingsFor(t, f.addr, false), []byte("pw"), msg)
	require.ErrorContains(t, err, "refusing to authenticate over plaintext")
	_, authBefore, _ := f.snapshot()
	require.False(t, authBefore)
}

func TestSMTPSenderRejectsHeaderInjection(t *testing.T) {
	cert, roots := testCert(t)
	f := startFake(t, true, false, cert)
	s := mail.NewSMTPSender(5*time.Second, roots)
	msg := mail.Message{To: []string{"a@example.com"}, Subject: "Merhaba\r\nBcc: evil@example.com", Text: "x"}
	require.Error(t, s.Send(context.Background(), settingsFor(t, f.addr, true), nil, msg))
	msg = mail.Message{To: []string{"a@example.com\r\nBcc: evil@example.com"}, Subject: "x", Text: "x"}
	require.Error(t, s.Send(context.Background(), settingsFor(t, f.addr, true), nil, msg))
	msg = mail.Message{To: nil, Subject: "x", Text: "x"}
	require.Error(t, s.Send(context.Background(), settingsFor(t, f.addr, true), nil, msg))
}

func TestTemplatesBothLocales(t *testing.T) {
	tr := mail.PasswordReset("tr", "Ayşe", "https://app.example/auth/reset-password?token=t1")
	require.Equal(t, "Şifre sıfırlama", tr.Subject)
	require.Contains(t, tr.Text, "Ayşe")
	require.Contains(t, tr.Text, "https://app.example/auth/reset-password?token=t1")
	en := mail.PasswordReset("en", "Ada", "https://x/auth/reset-password?token=t2")
	require.Equal(t, "Password reset", en.Subject)
	require.Contains(t, en.Text, "token=t2")
	require.Equal(t, tr.Subject, mail.PasswordReset("de", "x", "y").Subject, "unknown locale falls back to tr")
	require.Equal(t, "ekokod SMTP test iletisi", mail.SMTPTest("tr").Subject)
	require.Equal(t, "ekokod SMTP test message", mail.SMTPTest("en").Subject)
}
