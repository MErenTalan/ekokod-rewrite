//go:build integration

package billing_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	billingsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// R556/R558: a saved hour-bound window drives its key's bill, and the next
// unsaved key starts where it ends (Fuga: MEPAŞ reads at ~16:00).
func TestSavedPeriodDrivesGenerateAndNextKey(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7301)
	a, b := h.tenant.Analyzers[0].ID, h.tenant.Buildings[0].ID
	at := func(m time.Month, d, hr int) time.Time { return time.Date(2026, m, d, hr, 0, 0, 0, h.loc) }
	h.seed(a, at(1, 15, 0), "1000", "0", "0")
	h.seed(a, at(1, 17, 16), "1200", "0", "0")
	h.seed(a, at(2, 15, 0), "2000", "0", "0")
	h.seed(a, at(2, 18, 16), "2300", "0", "0")
	h.seed(a, at(3, 15, 0), "3000", "0", "0")
	h.clock.Set(at(3, 20, 0))

	saved, regen, err := h.svc.SavePeriod(h.ctx, h.tenant.AdminScope, billingsvc.SavePeriodRequest{
		BuildingID: b, PeriodKey: "2026-01", From: at(1, 17, 16), To: at(2, 18, 16),
	})
	require.NoError(t, err)
	require.True(t, saved.PeriodStart.Equal(at(1, 17, 16)))
	require.NotEmpty(t, regen, "the saved key's own bills are made")

	jan, err := h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a)
	require.NoError(t, err)
	require.True(t, jan.Bill.PeriodStart.Equal(at(1, 17, 16)) && jan.Bill.PeriodEnd.Equal(at(2, 18, 16)))
	require.Equal(t, "1100", jan.Bill.ActiveImport.String())

	feb, err := h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a, func(r *billingsvc.GenerateRequest) { r.PeriodKey = "2026-02" })
	require.NoError(t, err)
	require.True(t, feb.Bill.PeriodStart.Equal(at(2, 18, 16)) && feb.Bill.PeriodEnd.Equal(at(3, 15, 0)), "%s → %s", feb.Bill.PeriodStart, feb.Bill.PeriodEnd)
	require.Equal(t, "700", feb.Bill.ActiveImport.String())
}

// R557: an overlap with a saved neighbour is refused; a neighbour whose
// window moves is regenerated even though it is issued.
func TestSavePeriodConflictAndNeighbourRegeneration(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7302)
	a, b := h.tenant.Analyzers[0].ID, h.tenant.Buildings[0].ID
	at := func(m time.Month, d, hr int) time.Time { return time.Date(2026, m, d, hr, 0, 0, 0, h.loc) }
	for _, r := range []struct {
		ts time.Time
		v  string
	}{{at(1, 15, 0), "1000"}, {at(2, 15, 0), "2000"}, {at(2, 18, 0), "2300"}, {at(3, 15, 0), "3000"}} {
		h.seed(a, r.ts, r.v, "0", "0")
	}
	h.clock.Set(at(3, 20, 0))
	feb, err := h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a, func(r *billingsvc.GenerateRequest) { r.PeriodKey = "2026-02" })
	require.NoError(t, err)
	require.Equal(t, model.BillStatusIssued, feb.Bill.Status)

	_, regen, err := h.svc.SavePeriod(h.ctx, h.tenant.AdminScope, billingsvc.SavePeriodRequest{
		BuildingID: b, PeriodKey: "2026-01", From: at(1, 15, 0), To: at(2, 18, 0),
	})
	require.NoError(t, err)
	require.Contains(t, regen, billingsvc.Regeneration{Scope: model.BillScopeAnalyzer, SubjectID: a, PeriodKey: "2026-02"})

	_, _, err = h.svc.SavePeriod(h.ctx, h.tenant.AdminScope, billingsvc.SavePeriodRequest{
		BuildingID: b, PeriodKey: "2026-02", From: at(2, 17, 0), To: at(3, 15, 0),
	})
	require.Equal(t, billingsvc.CodePeriodConflict, h.computeCode(err))

	regen, err = h.svc.DeletePeriod(h.ctx, h.tenant.AdminScope, b, "2026-01")
	require.NoError(t, err)
	require.Contains(t, regen, billingsvc.Regeneration{Scope: model.BillScopeAnalyzer, SubjectID: a, PeriodKey: "2026-02"})
}

// R559: a preview prices any hour window, lists missing days and writes nothing.
func TestPreviewWritesNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7303)
	a := h.tenant.Analyzers[0].ID
	at := func(m time.Month, d, hr int) time.Time { return time.Date(2026, m, d, hr, 0, 0, 0, h.loc) }
	for d := 3; d <= 10; d++ { // Jan 1–2 missing: the start is in a gap
		h.seed(a, at(1, d, 0), strconv.Itoa(1000+10*(d-3)), "0", "0")
	}
	h.seed(a, at(1, 10, 16), "1075", "0", "0")
	h.clock.Set(at(2, 1, 0))

	res, err := h.svc.Preview(h.ctx, h.tenant.Scope, billingsvc.PreviewRequest{Scope: model.BillScopeAnalyzer, SubjectID: a, From: at(1, 1, 0), To: at(1, 10, 16)})
	require.NoError(t, err)
	require.Equal(t, "2026-01", res.Bill.PeriodKey)
	require.Equal(t, "75", res.Bill.ActiveImport.String(), "Jan 3 00:00 → Jan 10 16:00")
	require.Equal(t, []string{"2026-01-01", "2026-01-02"}, res.Bill.MissingDays)
	require.NotEmpty(t, res.Lines)

	bills, err := h.bills.List(h.ctx, h.tenant.AdminScope, store.BillFilter{})
	require.NoError(t, err)
	require.Empty(t, bills)
	anomalies, err := h.anomalies.List(h.ctx, h.tenant.AdminScope, store.AnomalyFilter{AnalyzerIDs: []uuid.UUID{a}})
	require.NoError(t, err)
	require.Empty(t, anomalies, "a preview records no missing_readings anomaly")

	_, err = h.svc.Preview(h.ctx, h.tenant.Scope, billingsvc.PreviewRequest{Scope: model.BillScopeAnalyzer, SubjectID: a, From: at(1, 1, 0), To: at(1, 1, 0).Add(30 * time.Minute)})
	require.ErrorIs(t, err, billingsvc.ErrInvalidRequest)
}
