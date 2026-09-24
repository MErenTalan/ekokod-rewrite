package mail

import (
	"bytes"
	"embed"
	"strings"
	"text/template"
	"time"
)

//go:embed templates/*.txt
var templateFS embed.FS

var templates = template.Must(template.ParseFS(templateFS, "templates/*.txt"))

var subjects = map[string]map[string]string{
	"password_reset": {"tr": "Şifre sıfırlama", "en": "Password reset"},
	"smtp_test":      {"tr": "ekokod SMTP test iletisi", "en": "ekokod SMTP test message"},
	"alarm_fired":    {"tr": "Alarm tetiklendi", "en": "Alarm triggered"},
}

func render(name, locale string, data any) Message {
	if locale != "en" {
		locale = "tr"
	}
	var b bytes.Buffer
	if err := templates.ExecuteTemplate(&b, name+"."+locale+".txt", data); err != nil {
		panic(err) // templates are embedded and covered by tests
	}
	return Message{Subject: subjects[name][locale], Text: strings.TrimSpace(b.String()) + "\n"}
}

// PasswordReset is the reset-link e-mail (R148); the caller sets To.
func PasswordReset(locale, name, link string) Message {
	return render("password_reset", locale, map[string]string{"Name": name, "Link": link})
}

// SMTPTest is the settings test message (R172); the caller sets To.
func SMTPTest(locale string) Message { return render("smtp_test", locale, nil) }

// AlarmFired is the alarm notification body (R221): plain text in both
// locales. Legacy's inline-HTML template is deliberately dropped — the Sender
// composes quoted-printable text, and a multipart sender is not in F7's scope.
//
// firedAt is already rendered in the reader's zone by the caller; the template
// only formats what it is given. The caller sets To.
func AlarmFired(locale, alarmName, analyzerLabel string, lines []string, firedAt time.Time) Message {
	return render("alarm_fired", locale, map[string]any{
		"AlarmName": alarmName, "AnalyzerLabel": analyzerLabel, "Lines": lines,
		"FiredAt": firedAt.Format("02.01.2006 15:04"),
	})
}
