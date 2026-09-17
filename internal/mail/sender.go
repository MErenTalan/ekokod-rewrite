// Package mail delivers outbound e-mail through a company's SMTP settings
// (R184). Credentials only ever travel over a verified TLS connection.
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// Message is one plain-text e-mail.
type Message struct {
	To      []string
	Subject string
	Text    string
}

// Sender delivers a Message with one company's SMTP settings.
type Sender interface {
	Send(ctx context.Context, cfg model.SMTPSettings, password []byte, m Message) error
}

// ErrPlaintextAuth is returned when the server offers no TLS but a password is configured.
var ErrPlaintextAuth = errors.New("mail: refusing to authenticate over plaintext")

type smtpSender struct {
	dialTimeout time.Duration
	roots       *x509.CertPool
}

// NewSMTPSender builds a Sender; roots nil means the system trust store.
func NewSMTPSender(dialTimeout time.Duration, roots *x509.CertPool) Sender {
	return &smtpSender{dialTimeout: dialTimeout, roots: roots}
}

func (s *smtpSender) Send(ctx context.Context, cfg model.SMTPSettings, password []byte, m Message) error {
	body, err := compose(cfg.FromAddress, m)
	if err != nil {
		return err
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(int(cfg.Port)))
	tlsCfg := &tls.Config{ServerName: cfg.Host, RootCAs: s.roots, MinVersion: tls.VersionTLS12}

	dialer := &net.Dialer{Timeout: s.dialTimeout}
	var conn net.Conn
	if cfg.Secure {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsCfg}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("mail: connect: %w", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(2 * time.Minute))
	}
	c, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("mail: handshake: %w", err)
	}
	defer func() { _ = c.Close() }()

	if !cfg.Secure {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(tlsCfg); err != nil {
				return fmt.Errorf("mail: starttls: %w", err)
			}
		}
	}
	if len(password) > 0 {
		if _, isTLS := c.TLSConnectionState(); !isTLS {
			return ErrPlaintextAuth
		}
		if err := c.Auth(smtp.PlainAuth("", cfg.Username, string(password), cfg.Host)); err != nil {
			return fmt.Errorf("mail: auth: %w", err)
		}
	}
	if err := c.Mail(cfg.FromAddress); err != nil {
		return fmt.Errorf("mail: sender rejected: %w", err)
	}
	for _, to := range m.To {
		if err := c.Rcpt(to); err != nil {
			return fmt.Errorf("mail: recipient rejected: %w", err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail: data: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("mail: write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: finish: %w", err)
	}
	return c.Quit()
}

func compose(from string, m Message) ([]byte, error) {
	if len(m.To) == 0 {
		return nil, errors.New("mail: no recipients")
	}
	for _, v := range append([]string{from, m.Subject}, m.To...) {
		if strings.ContainsAny(v, "\r\n") {
			return nil, errors.New("mail: header value contains a line break")
		}
	}
	for _, to := range append([]string{from}, m.To...) {
		if _, err := mail.ParseAddress(to); err != nil {
			return nil, fmt.Errorf("mail: invalid address: %w", err)
		}
	}
	var id [12]byte
	_, _ = rand.Read(id[:])
	domain := "ekokod"
	if at := strings.LastIndexByte(from, '@'); at >= 0 {
		domain = from[at+1:]
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(m.To, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", m.Subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: <%s@%s>\r\n", hex.EncodeToString(id[:]), domain)
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n")
	qp := quotedprintable.NewWriter(&b)
	if _, err := qp.Write([]byte(strings.ReplaceAll(m.Text, "\n", "\r\n"))); err != nil {
		return nil, err
	}
	if err := qp.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
