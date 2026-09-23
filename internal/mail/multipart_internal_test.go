package mail

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fixSeams pins the Date header and every random byte, so compose's output
// is a fixed string.
func fixSeams(t *testing.T) {
	t.Helper()
	oldNow, oldRand := now, randRead
	now = func() time.Time { return time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC) }
	randRead = func(b []byte) (int, error) {
		for i := range b {
			b[i] = 0xab
		}
		return len(b), nil
	}
	t.Cleanup(func() { now, randRead = oldNow, oldRand })
}

// TestComposePlainUnchanged pins today's single-part output byte for byte:
// the alarm, reset and SMTP-test mails must not change shape because a
// report needs attachments.
func TestComposePlainUnchanged(t *testing.T) {
	fixSeams(t)
	got, err := compose("rapor@ekokod.test", Message{To: []string{"a@b.test"}, Subject: "Şifre", Text: "satır 1\nsatır 2"})
	require.NoError(t, err)
	want := "From: rapor@ekokod.test\r\n" +
		"To: a@b.test\r\n" +
		"Subject: =?utf-8?q?=C5=9Eifre?=\r\n" +
		"Date: Wed, 23 Sep 2026 10:00:00 +0000\r\n" +
		"Message-ID: <abababababababababababab@ekokod.test>\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n" +
		"sat=C4=B1r 1\r\nsat=C4=B1r 2"
	require.Equal(t, want, string(got))
}

type part struct {
	contentType, filename string
	body                  []byte
}

// parts flattens a parsed message into its leaf parts, decoding each.
func parts(t *testing.T, raw []byte) (string, []part) {
	t.Helper()
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	require.NoError(t, err)
	top, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	require.NoError(t, err)
	var out []part
	var walk func(r io.Reader, boundary string)
	walk = func(r io.Reader, boundary string) {
		mr := multipart.NewReader(r, boundary)
		for {
			p, err := mr.NextRawPart()
			if err == io.EOF {
				return
			}
			require.NoError(t, err)
			ct, ps, err := mime.ParseMediaType(p.Header.Get("Content-Type"))
			require.NoError(t, err)
			if strings.HasPrefix(ct, "multipart/") {
				walk(p, ps["boundary"])
				continue
			}
			var body []byte
			switch p.Header.Get("Content-Transfer-Encoding") {
			case "base64":
				body, err = io.ReadAll(base64.NewDecoder(base64.StdEncoding, p))
			case "quoted-printable":
				body, err = io.ReadAll(quotedprintable.NewReader(p))
			default:
				body, err = io.ReadAll(p)
			}
			require.NoError(t, err)
			out = append(out, part{contentType: ct, filename: p.FileName(), body: body})
		}
	}
	walk(msg.Body, params["boundary"])
	return top, out
}

func TestComposeAttachmentsParse(t *testing.T) {
	fixSeams(t)
	pdf := bytes.Repeat([]byte{0x25, 0x50, 0x44, 0x46, 0x00, 0xff}, 50) // longer than one base64 line
	xlsx := []byte("PK\x03\x04 workbook")
	raw, err := compose("rapor@ekokod.test", Message{To: []string{"a@b.test"}, Subject: "Rapor", Text: "Özet",
		Attachments: []Attachment{
			{Filename: "rapor-2026-03.pdf", ContentType: "application/pdf", Data: pdf},
			{Filename: "rapor-2026-03.xlsx", ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Data: xlsx},
		}})
	require.NoError(t, err)
	for _, line := range strings.Split(string(raw), "\r\n") {
		require.LessOrEqual(t, len(line), 998, "RFC 5322 line limit")
	}
	top, ps := parts(t, raw)
	require.Equal(t, "multipart/mixed", top)
	require.Len(t, ps, 3)
	require.Equal(t, "text/plain", ps[0].contentType)
	require.Equal(t, "Özet", string(ps[0].body))
	require.Equal(t, "rapor-2026-03.pdf", ps[1].filename)
	require.Equal(t, pdf, ps[1].body)
	require.Equal(t, "rapor-2026-03.xlsx", ps[2].filename)
	require.Equal(t, xlsx, ps[2].body)
}

func TestComposeHTMLAlternative(t *testing.T) {
	fixSeams(t)
	raw, err := compose("rapor@ekokod.test", Message{To: []string{"a@b.test"}, Subject: "Rapor", Text: "düz", HTML: "<p>zengin</p>"})
	require.NoError(t, err)
	top, ps := parts(t, raw)
	require.Equal(t, "multipart/alternative", top)
	require.Len(t, ps, 2)
	require.Equal(t, "text/plain", ps[0].contentType, "plain first: clients show the last part they understand")
	require.Equal(t, "text/html", ps[1].contentType)
	require.Equal(t, "<p>zengin</p>", string(ps[1].body))

	raw, err = compose("rapor@ekokod.test", Message{To: []string{"a@b.test"}, Subject: "Rapor", Text: "düz", HTML: "<p>zengin</p>",
		Attachments: []Attachment{{Filename: "a.pdf", ContentType: "application/pdf", Data: []byte("x")}}})
	require.NoError(t, err)
	top, ps = parts(t, raw)
	require.Equal(t, "multipart/mixed", top)
	require.Equal(t, []string{"text/plain", "text/html", "application/pdf"}, []string{ps[0].contentType, ps[1].contentType, ps[2].contentType})
}

func TestComposeTurkishFilename(t *testing.T) {
	fixSeams(t)
	raw, err := compose("rapor@ekokod.test", Message{To: []string{"a@b.test"}, Subject: "Rapor", Text: "x",
		Attachments: []Attachment{{Filename: "rapor-şubat ğ.pdf", ContentType: "application/pdf", Data: []byte("x")}}})
	require.NoError(t, err)
	_, ps := parts(t, raw)
	require.Equal(t, "rapor-şubat ğ.pdf", ps[1].filename)
}

func TestComposeRejectsHeaderInjectionInFilename(t *testing.T) {
	fixSeams(t)
	for _, a := range []Attachment{
		{Filename: "a.pdf\r\nBcc: x@evil.test", ContentType: "application/pdf"},
		{Filename: "a.pdf", ContentType: "application/pdf\nBcc: x@evil.test"},
		{Filename: "", ContentType: "application/pdf"},
	} {
		_, err := compose("rapor@ekokod.test", Message{To: []string{"a@b.test"}, Subject: "s", Text: "x", Attachments: []Attachment{a}})
		require.Error(t, err, "%q", a.Filename+a.ContentType)
	}
}
