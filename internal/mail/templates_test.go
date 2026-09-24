package mail_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/mail"
)

func TestAlarmFiredRendersBothLocales(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 18, 14, 30, 0, 0, time.UTC)

	tr := mail.AlarmFired("tr", "İletişim", "A-1", []string{"Analizör 7 saattir veri göndermiyor"}, at)
	require.Contains(t, tr.Subject, "Alarm")
	require.Contains(t, tr.Text, "İletişim")
	require.Contains(t, tr.Text, "A-1")
	require.Contains(t, tr.Text, "7 saattir")
	require.Contains(t, tr.Text, "18.09.2026 14:30")

	en := mail.AlarmFired("en", "Comms", "A-1", []string{"No data for 7 hours"}, at)
	require.NotEqual(t, tr.Subject, en.Subject)
	require.Contains(t, en.Text, "No data for 7 hours")

	// An unknown locale falls back to Turkish, as render() already does.
	require.Equal(t, tr.Subject, mail.AlarmFired("de", "İletişim", "A-1",
		[]string{"Analizör 7 saattir veri göndermiyor"}, at).Subject)
}

func TestAlarmFiredListsEveryLine(t *testing.T) {
	t.Parallel()
	// A rule can breach several thresholds at once; dropping one would hide a
	// condition the operator needs.
	at := time.Date(2026, 9, 18, 14, 30, 0, 0, time.UTC)
	msg := mail.AlarmFired("tr", "Reaktif", "A-1", []string{"birinci", "ikinci", "üçüncü"}, at)
	for _, line := range []string{"birinci", "ikinci", "üçüncü"} {
		require.Contains(t, msg.Text, line)
	}
}
