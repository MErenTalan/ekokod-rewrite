package billing

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// TestGenerateNeverReadsAnalyticsRows is R61's reflection guard (F3 carry-forward 1).
func TestGenerateNeverReadsAnalyticsRows(t *testing.T) {
	analytics := reflect.TypeOf((*store.AnalyticsRepository)(nil)).Elem()
	deps := reflect.TypeOf(Deps{})
	for i := range deps.NumField() {
		f := deps.Field(i)
		require.False(t, f.Type == analytics || (f.Type.Kind() == reflect.Interface && f.Type.Implements(analytics)) ||
			reflect.PointerTo(f.Type).Implements(analytics), "Deps.%s can reach analytics", f.Name)
	}
	reader := reflect.TypeOf((*ConsumptionReader)(nil)).Elem()
	require.True(t, reflect.TypeOf(&consumption.Billing{}).Implements(reader))
	require.False(t, reflect.TypeOf(&consumption.Analytics{}).Implements(reader), "the analytics service cannot stand in")
}

func TestSoundHoursDropsGapAbsorbingRows(t *testing.T) {
	h0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	v := decimal.NewFromInt(3)
	row := func(from, spanFrom time.Time, suspect bool) consumption.Row {
		r := consumption.Row{Window: energy.Window{From: from, To: from.Add(time.Hour)}, SpanFrom: spanFrom, SpanTo: from.Add(time.Hour),
			Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: &v}}
		if suspect {
			r.Suspect = map[energy.Register]energy.Suspicion{energy.ActiveImport: {}}
		}
		return r
	}
	got := soundHours([]consumption.Row{
		row(h0, h0, false),
		row(h0.Add(5*time.Hour), h0.Add(time.Hour), false), // absorbed 4 missing hours (I-3)
		row(h0.Add(6*time.Hour), h0.Add(6*time.Hour), true),
		{Window: energy.Window{From: h0.Add(7 * time.Hour)}, SpanFrom: h0.Add(7 * time.Hour), SpanTo: h0.Add(8 * time.Hour)}, // nil import
	})
	require.Equal(t, []tariff.HourConsumption{{Hour: h0, Kwh: v}}, got)
}

func TestOverlapsIsIntervalOverlap(t *testing.T) {
	w := energy.Window{From: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)}
	a := func(from, to time.Time) model.ConsumptionAnomaly {
		return model.ConsumptionAnomaly{PeriodStart: from, PeriodEnd: to}
	}
	require.True(t, overlaps(a(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)), w))
	require.False(t, overlaps(a(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), w.From), w), "ends at the window start")
	require.False(t, overlaps(a(w.To, w.To.Add(time.Hour)), w))
}

func TestChunksAndMonths(t *testing.T) {
	ids := make([]uuid.UUID, 101)
	var sizes []int
	for _, c := range chunks(ids, 50) {
		sizes = append(sizes, len(c))
	}
	require.Equal(t, []int{50, 50, 1}, sizes)
	loc, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)
	w := energy.Window{From: time.Date(2026, 1, 31, 0, 0, 0, 0, loc), To: time.Date(2026, 3, 1, 0, 0, 0, 0, loc)}
	require.Equal(t, []tariff.YearMonth{{Year: 2026, Month: 1}, {Year: 2026, Month: 2}}, monthsTouched(w, loc))
}

func TestNewRequiresEveryDependency(t *testing.T) {
	_, err := New(Deps{})
	require.Error(t, err)
}
