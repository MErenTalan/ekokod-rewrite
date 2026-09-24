//go:build integration

package legacy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/migrate/legacy"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// TestReconcileEndToEnd is 09 §F14's reconciliation over the migrated fixture:
// a missing recomputed bill is unexplained and blocks; once it exists and
// agrees, only attributed differences remain.
func TestReconcileEndToEnd(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	extract := t.TempDir()
	_, err := legacy.Extract(ctx, carbonISOSource(t), extract)
	require.NoError(t, err)
	dir, _, _ := transformFrom(t, extract, map[string]map[string]string{legacy.AnswerTariffClass: {building2Hex: "og/industrial/binomial/private"}})
	_, err = legacy.Artifacts(dir, []string{legacyTree(t)}, t.TempDir())
	require.NoError(t, err)
	_, err = legacy.Load(ctx, admin.NewLegacyLoader(pool), dir)
	require.NoError(t, err)

	company, building := legacy.ID("companies", companyHex), legacy.ID("buildings", buildingHex)
	analyzer := legacy.ID("analyzers", "64f0000000000000000000a1")
	bill := func(scope model.BillScope, subject *uuid.UUID, total string) {
		ist, _ := time.LoadLocation("Europe/Istanbul")
		b := model.Bill{CompanyID: company, BuildingID: &building, Scope: scope, PeriodKey: "2026-08",
			PeriodStart: time.Date(2026, 8, 1, 0, 0, 0, 0, ist), PeriodEnd: time.Date(2026, 9, 1, 0, 0, 0, 0, ist), DaysInPeriod: 31,
			Currency: model.CurrencyTRY, Status: model.BillStatusIssued, GenerationUsage: model.GenerationUsageNone, ComputedAt: time.Now(),
			IndexStart: []byte(`{}`), IndexEnd: []byte(`{}`), TotalCost: decimal.RequireFromString(total), ActiveImport: decimal.RequireFromString("1234.5")}
		if scope == model.BillScopeAnalyzer {
			b.AnalyzerID, b.ActiveImport = subject, decimal.Zero // its legacy history has no kWh
		} else {
			b.ReactivePenaltyApplied = true // as the legacy August bill says
		}
		_, err := postgres.NewBillRepository(pool).Create(ctx, store.SystemScope(company), b, nil, nil)
		require.NoError(t, err)
	}
	bill(model.BillScopeBuilding, nil, "1000.5")

	repo := admin.NewReconcileRepository(pool)
	in, err := legacy.GatherReconcile(ctx, repo, dir, extract, time.Now())
	require.NoError(t, err)
	rep := legacy.Reconcile(in)
	require.False(t, rep.Pass, "the analyzer's August bill was never recomputed")
	var acme legacy.CompanyReport
	for _, c := range rep.Companies {
		if c.ID == company.String() {
			acme = c
		}
	}
	require.Equal(t, 1, acme.Reasons[legacy.Unexplained])
	require.Equal(t, 1, acme.Reasons[legacy.ReasonAutomatedCarbon], "legacy's daily carbon rows (rate 1) are recomputed")
	require.Positive(t, acme.Reasons[legacy.ReasonMultiplierPending], "the OSOS × 40 analyzer's readings are withheld")
	for _, r := range rep.Global {
		require.False(t, r.Blocks(), "global row %s %s: %v", r.Check, r.Subject, r.Reasons)
	}

	bill(model.BillScopeAnalyzer, &analyzer, "250.25")
	in, err = legacy.GatherReconcile(ctx, repo, dir, extract, time.Now())
	require.NoError(t, err)
	rep = legacy.Reconcile(in)
	require.True(t, rep.Pass, "every remaining difference carries a documented reason")
	var html, csv bytes.Buffer
	require.NoError(t, legacy.RenderHTML(&html, rep))
	require.NoError(t, legacy.RenderCSV(&csv, rep))
	require.Contains(t, html.String(), "GEÇTİ")
	require.Contains(t, html.String(), "Acme")
	require.Contains(t, csv.String(), "automated_carbon_recomputed")

	// A legacy collection the transform never reads is a hole in the migration, never silent.
	m, err := legacy.ReadManifest(extract)
	require.NoError(t, err)
	m.Collections = append(m.Collections, legacy.ManifestEntry{Collection: "mysteries", Count: 3})
	raw, err := json.Marshal(m)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(extract, "manifest.json"), raw, 0o600))
	in, err = legacy.GatherReconcile(ctx, repo, dir, extract, time.Now())
	require.NoError(t, err)
	require.False(t, legacy.Reconcile(in).Pass)
}
