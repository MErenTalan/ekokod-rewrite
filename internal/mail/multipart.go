package mail

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
)

// Attachment is one file sent with a Message.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// checkAttachments refuses anything that could smuggle a header: a name or
// content type with a line break, or an unparseable content type.
func checkAttachments(as []Attachment) error {
	for _, a := range as {
		if a.Filename == "" || strings.ContainsAny(a.Filename+a.ContentType, "\r\n") {
			return errors.New("mail: invalid attachment name or type")
		}
		if _, _, err := mime.ParseMediaType(a.ContentType); err != nil {
			return fmt.Errorf("mail: invalid attachment type: %w", err)
		}
	}
	return nil
}

func boundary() string {
	var b [16]byte
	_, _ = randRead(b[:])
	return "ekokod-" + hex.EncodeToString(b[:])
}

// writeMultipart writes the Content-Type header and the body: the text (with
// its HTML alternative, if any) and then each attachment in base64.
func writeMultipart(b *bytes.Buffer, m Message) error {
	if len(m.Attachments) == 0 {
		return writeAlternative(b, m)
	}
	w := multipart.NewWriter(b)
	if err := w.SetBoundary(boundary()); err != nil {
		return err
	}
	fmt.Fprintf(b, "Content-Type: multipart/mixed; boundary=%s\r\n\r\n", w.Boundary())
	if m.HTML != "" {
		// The alternative is a nested part: its own header goes through the
		// outer writer, its body is written straight after.
		// Derived, not drawn again: two boundaries must differ even when
		// the random source repeats itself.
		inner := w.Boundary() + "-alt"
		pw, err := w.CreatePart(textproto.MIMEHeader{"Content-Type": {"multipart/alternative; boundary=" + inner}})
		if err != nil {
			return err
		}
		var body bytes.Buffer
		if err := alternativeBody(&body, m, inner); err != nil {
			return err
		}
		if _, err := pw.Write(body.Bytes()); err != nil {
			return err
		}
	} else if err := textPart(w, "text/plain", m.Text); err != nil {
		return err
	}
	for _, a := range m.Attachments {
		h := textproto.MIMEHeader{
			"Content-Type":              {a.ContentType},
			"Content-Transfer-Encoding": {"base64"},
			// FormatMediaType switches to RFC 2231 for a non-ASCII name.
			"Content-Disposition": {mime.FormatMediaType("attachment", map[string]string{"filename": a.Filename})},
		}
		pw, err := w.CreatePart(h)
		if err != nil {
			return err
		}
		enc := base64.StdEncoding.EncodeToString(a.Data)
		for len(enc) > 76 {
			if _, err := pw.Write([]byte(enc[:76] + "\r\n")); err != nil {
				return err
			}
			enc = enc[76:]
		}
		if _, err := pw.Write([]byte(enc)); err != nil {
			return err
		}
	}
	return w.Close()
}

// writeAlternative writes a top-level multipart/alternative message.
func writeAlternative(b *bytes.Buffer, m Message) error {
	bnd := boundary()
	fmt.Fprintf(b, "Content-Type: multipart/alternative; boundary=%s\r\n\r\n", bnd)
	return alternativeBody(b, m, bnd)
}

// alternativeBody is text/plain first, then text/html: a client shows the
// last alternative it understands.
func alternativeBody(b *bytes.Buffer, m Message, bnd string) error {
	w := multipart.NewWriter(b)
	if err := w.SetBoundary(bnd); err != nil {
		return err
	}
	if err := textPart(w, "text/plain", m.Text); err != nil {
		return err
	}
	if err := textPart(w, "text/html", m.HTML); err != nil {
		return err
	}
	return w.Close()
}

func textPart(w *multipart.Writer, contentType, text string) error {
	pw, err := w.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {contentType + "; charset=utf-8"},
		"Content-Transfer-Encoding": {"quoted-printable"},
	})
	if err != nil {
		return err
	}
	return writeQP(pw, text)
}
