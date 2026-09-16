package consumption

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// TestMaxDemandInWindowExcludesDisallowedKindsPerLevel is a compensating
// white-box test: loadAnalyzerBoundaryData no longer fetches daily-kind
// readings for MaxDemand at Hourly, nor billing-kind readings for MaxDemand
// at Hourly or Daily, since energy.MaxDemandKindsFor already excludes them
// there — those Range calls are dead code once R101 makes their result
// unreachable through the kind filter below. That means the black-box
// billing_test.go fixtures that used to prove the EXCLUSION by loading a
// decoy reading through the real query path can no longer reach
// maxDemandInWindow with that decoy at all — the fetch is skipped before
// the decoy is ever loaded, so a regression in maxDemandInWindow's own kind
// filter (e.g. reverting to the unqualified energy.MaxDemandKinds) would no
// longer be provable from outside the package. This white-box test calls
// maxDemandInWindow directly, passing the disallowed-kind reading straight
// in as a `sources` slice — bypassing loadAnalyzerBoundaryData entirely —
// so the function's own kind filter stays independently provable regardless
// of which caller loads what.
func TestMaxDemandInWindowExcludesDisallowedKindsPerLevel(t *testing.T) {
	w := energy.Window{
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC),
	}
	mid := w.From.Add(30 * time.Minute)

	reading := func(kind energy.Kind, maxDemand string) energy.Reading {
		v := decimal.RequireFromString(maxDemand)
		return energy.Reading{TS: mid, Kind: kind, MaxDemandKw: &v}
	}

	loadProfile := []energy.Reading{reading(energy.KindLoadProfile, "5")}
	daily := []energy.Reading{reading(energy.KindDaily, "500")}
	billing := []energy.Reading{reading(energy.KindBilling, "9000")}

	hourly := maxDemandInWindow(energy.Hourly, w, loadProfile, daily, billing)
	require.NotNil(t, hourly)
	require.Equal(t, "5", hourly.String(), "Hourly excludes both daily- and billing-kind readings even when present in the sources")

	dailyLevel := maxDemandInWindow(energy.Daily, w, loadProfile, daily, billing)
	require.NotNil(t, dailyLevel)
	require.Equal(t, "500", dailyLevel.String(), "Daily allows daily-kind but still excludes billing-kind")

	monthlyLevel := maxDemandInWindow(energy.Monthly, w, loadProfile, daily, billing)
	require.NotNil(t, monthlyLevel)
	require.Equal(t, "9000", monthlyLevel.String(), "Monthly (and Yearly) allow billing-kind too")
}

// stubAnomaliesForGapOverrideTest implements only List (returning a fixed
// set of rows); every other method panics — TestApplyResolvedGapOverrides
// MaxDemandRespectsRequestLevel only ever reaches List, through
// listAllAnomalies.
type stubAnomaliesForGapOverrideTest struct {
	rows []model.ConsumptionAnomaly
}

func (s stubAnomaliesForGapOverrideTest) Get(context.Context, store.Scope, uuid.UUID) (model.ConsumptionAnomaly, error) {
	panic("not used by this test")
}

func (s stubAnomaliesForGapOverrideTest) List(context.Context, store.Scope, store.AnomalyFilter) ([]model.ConsumptionAnomaly, error) {
	return s.rows, nil
}

func (s stubAnomaliesForGapOverrideTest) Create(context.Context, store.Scope, model.ConsumptionAnomaly) (model.ConsumptionAnomaly, error) {
	panic("not used by this test")
}

func (s stubAnomaliesForGapOverrideTest) Resolve(context.Context, store.Scope, uuid.UUID, uuid.UUID, string, []byte, time.Time) (model.ConsumptionAnomaly, error) {
	panic("not used by this test")
}

// TestApplyResolvedGapOverridesMaxDemandRespectsRequestLevel proves the
// gap-override emission call site
// (applyResolvedGapOverrides, billing.go) computes MaxDemandKw with the
// REQUEST's own level, never a hardcoded one. No black-box test builds a
// Daily gap override with a
// billing-kind reading actually present in data.maxDemandBilling —
// loadAnalyzerBoundaryData never even fetches billing-kind
// readings for MaxDemand at Daily (there is nowhere left in production for
// one to come from), so this white-box test builds the
// analyzerBoundaryData directly, planting a billing-kind reading in
// data.maxDemandBilling by hand, to prove the CALL SITE itself still
// respects the level it is given rather than a hardcoded one.
func TestApplyResolvedGapOverridesMaxDemandRespectsRequestLevel(t *testing.T) {
	ctx := context.Background()
	sc := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	day := energy.Window{
		From: time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 1, 11, 0, 0, 0, 0, time.UTC),
	}

	resolvedAt := time.Now()
	resolution := string(ResolveByOverride)
	anomalies := stubAnomaliesForGapOverrideTest{rows: []model.ConsumptionAnomaly{{
		ID:             uuid.New(),
		AnalyzerID:     analyzerID,
		PeriodStart:    day.From,
		PeriodEnd:      day.To,
		Reason:         string(energy.ReasonMissingReadings),
		ResolvedAt:     &resolvedAt,
		Resolution:     &resolution,
		OverrideValues: []byte(`{"active_import":"42"}`),
	}}}

	b := &Billing{deps: BillingDeps{Anomalies: anomalies}}

	billingKw := decimal.RequireFromString("9000")
	data := analyzerBoundaryData{
		maxDemandBilling: []energy.Reading{{
			TS:          day.From.Add(12 * time.Hour),
			Kind:        energy.KindBilling,
			MaxDemandKw: &billingKw,
		}},
		resolved: map[int64]*energy.Reading{},
	}

	rows, remaining, err := b.applyResolvedGapOverrides(ctx, sc, analyzerID, energy.Daily, []bucketGap{{Window: day, Boundaries: []string{"start", "end"}}}, data)
	require.NoError(t, err)
	require.Empty(t, remaining, "the gap resolved to an override row")
	require.Len(t, rows, 1)
	require.Nil(t, rows[0].MaxDemandKw, "a Daily gap override must not take a billing-kind peak, even though one is present in the loaded data")
}
