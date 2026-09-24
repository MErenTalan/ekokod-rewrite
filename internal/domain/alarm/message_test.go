package alarm_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/alarm"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

func TestDescribeRendersBothLocales(t *testing.T) {
	t.Parallel()
	a := reactiveRule()
	a.Name = "Endüktif izleme"
	in := alarm.ReactiveInput{Inductive: []model.MeterReading{
		reading(0, "1000", "100", "0"), reading(24, "1100", "125", "0")}}
	v := alarm.EvaluateReactive(a, in)

	tr, lines, detail := alarm.Describe(a, v, "A-1", "tr")
	require.Contains(t, tr, "Endüktif izleme")
	require.Contains(t, tr, "A-1")
	require.Len(t, lines, 1)
	require.Contains(t, lines[0], "25")
	require.Contains(t, lines[0], "20")
	require.Equal(t, "A-1", detail["analyzer"])

	breaches, ok := detail["breaches"].([]map[string]any)
	require.True(t, ok)
	require.Equal(t, alarm.FieldInductive, breaches[0]["field"])
	require.Equal(t, "25", breaches[0]["measured"])
	require.Equal(t, "20", breaches[0]["threshold"])

	_, enLines, _ := alarm.Describe(a, v, "A-1", "en")
	require.NotEqual(t, lines[0], enLines[0])
	require.Contains(t, enLines[0], "Inductive")
}

func TestDescribeFallsBackToTurkish(t *testing.T) {
	t.Parallel()
	a := commsRule()
	a.Name = "İletişim"
	last := base.Add(-9 * time.Hour)
	v := alarm.EvaluateComms(a, uuid.New(), &last, base)
	_, fallback, _ := alarm.Describe(a, v, "A-1", "de")
	_, turkish, _ := alarm.Describe(a, v, "A-1", "tr")
	require.Equal(t, turkish, fallback)
}

func TestDescribeDetailCarriesOnlyCuratedKeys(t *testing.T) {
	t.Parallel()
	// R224: detail is curated, so no provider or SMTP string can ride along.
	a := commsRule()
	a.Name = "İletişim"
	last := base.Add(-9 * time.Hour)
	_, lines, detail := alarm.Describe(a, alarm.EvaluateComms(a, uuid.New(), &last, base), "A-1", "tr")
	for k := range detail {
		require.Contains(t, []string{"analyzer", "alarm_type", "breaches", "no_verdict", "lines"}, k)
	}
	require.Equal(t, string(model.AlarmTypeDataCommunication), detail["alarm_type"])
	// The rendered sentences are part of the record, so the notifier repeats
	// what fired rather than re-deriving it later from different data.
	require.Equal(t, lines, detail["lines"])
}

func TestDescribeCarriesTheWindowAndTheTypeExtras(t *testing.T) {
	t.Parallel()
	a := reactiveRule()
	a.Name = "Endüktif"
	w := alarm.NewWindow(base, 24, model.PeriodUnitHours)
	in := alarm.ReactiveInput{
		Inductive: []model.MeterReading{reading(0, "1000", "100", "0"), reading(24, "1100", "125", "0")},
		Windows:   map[string]alarm.Window{alarm.FieldInductive: w}}
	_, _, detail := alarm.Describe(a, alarm.EvaluateReactive(a, in), "A-1", "tr")
	row := detail["breaches"].([]map[string]any)[0]
	require.Equal(t, w.From.Format(time.RFC3339), row["window_from"])
	require.Equal(t, w.To.Format(time.RFC3339), row["window_to"])

	// A point-in-time check carries no window but keeps its own extras.
	c := commsRule()
	c.Name = "İletişim"
	last := base.Add(-9 * time.Hour)
	_, _, commsDetail := alarm.Describe(c, alarm.EvaluateComms(c, uuid.New(), &last, base), "A-1", "tr")
	commsRow := commsDetail["breaches"].([]map[string]any)[0]
	require.NotContains(t, commsRow, "window_from")
	require.Equal(t, last.Format(time.RFC3339), commsRow["last_reading_at"])
}

func TestDescribeReportsNoVerdictSeparately(t *testing.T) {
	t.Parallel()
	// A no-verdict must never be rendered as a breach sentence: "we could not
	// decide" is not "nothing is wrong" and not "something is wrong".
	a := commsRule()
	a.Name = "İletişim"
	v := alarm.EvaluateComms(a, uuid.New(), nil, base)
	summary, lines, detail := alarm.Describe(a, v, "A-1", "tr")
	require.Empty(t, lines)
	require.Contains(t, summary, "İletişim")
	require.Equal(t, []string{alarm.FieldCommsHours}, detail["no_verdict"])
	require.Empty(t, detail["breaches"])
}

func TestDescribeHasASentenceForEveryField(t *testing.T) {
	t.Parallel()
	// A field with no phrase would render as a raw %!s(MISSING) in an e-mail.
	for _, locale := range []string{"tr", "en"} {
		for _, field := range alarm.DescribableFields() {
			require.NotEmpty(t, alarm.PhraseFor(locale, field), "%s/%s", locale, field)
		}
	}
}
