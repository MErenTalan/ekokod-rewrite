//go:build integration

package v1_test

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

var ist = func() *time.Location { l, _ := time.LoadLocation("Europe/Istanbul"); return l }()

// seedAnalysis gives A1's analyzer 10 kWh/h (inductive share 0.35) and a
// second A1 analyzer 30 kWh/h, from 25 Jul to 17 Sep 2026 (Istanbul): a
// composed month needs a daily bucket before it (R94).
func (h *harness) seedAnalysis() uuid.UUID {
	h.t.Helper()
	ctx := h.t.Context()
	h.clock.Set(time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC))
	from, to := time.Date(2026, 7, 25, 0, 0, 0, 0, ist), time.Date(2026, 9, 17, 0, 0, 0, 0, ist)
	buildingID := h.fx.BuildingA1
	second, err := postgres.NewAnalyzerRepository(h.pool).Create(ctx, store.SystemScope(h.fx.CompanyA), model.Analyzer{
		CompanyID: h.fx.CompanyA, BuildingID: &buildingID, Provider: model.IntegrationProviderOSOS, ProviderSubtype: "Baskent",
		InstallationNumber: "E2E-A1-2", MeterMultiplier: decimal.NewFromInt(1), IsActive: true,
	})
	require.NoError(h.t, err)
	testfixtures.InsertReadings(h.t, ctx, h.pool, h.fx.CompanyA, testfixtures.HourlyReadingsReactive(h.fx.AnalyzerA1, from, to, testfixtures.Constant("10"), "0.35", "0.05"))
	testfixtures.InsertReadings(h.t, ctx, h.pool, h.fx.CompanyA, testfixtures.HourlyReadings(second.ID, from, to, testfixtures.Constant("30")))
	testfixtures.InsertReadings(h.t, ctx, h.pool, h.fx.CompanyA, testfixtures.HourlyReadings(h.fx.AnalyzerA2, from, to, testfixtures.Constant("5")))
	return second.ID
}

func TestConsumptionColumnSet(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seedAnalysis()
	ca := h.as(seed.E2ECompanyAdminEmail)
	res := ca.do(http.MethodGet, "/consumption?analyzer_id="+h.fx.AnalyzerA1.String()+"&granularity=daily&from=2026-09-01&to=2026-09-02", nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	var raw struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	require.NoError(t, json.Unmarshal(res.body, &raw))
	require.Len(t, raw.Items, 2)
	var keys []string
	for k := range raw.Items[0] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{
		// 01 §7.3: period, every index (active, inductive, capacitive, T1–T3, generation indexes, U1–U3), consumption,
		// inductive/capacitive, ratios, T1–T3 consumption, generation, U1–U3 generation, max demand.
		"active_export", "active_export_index", "active_import", "active_import_index", "analyzer_id", "capacitive_ratio",
		"inductive_ratio", "max_demand_kw", "partial", "period_end", "period_start",
		"reactive_capacitive_export", "reactive_capacitive_export_index", "reactive_capacitive_import", "reactive_capacitive_import_index",
		"reactive_inductive_export", "reactive_inductive_export_index", "reactive_inductive_import", "reactive_inductive_import_index",
		"source", "suspect_registers",
		"t1_export", "t1_export_index", "t1_import", "t1_import_index", "t2_export", "t2_export_index", "t2_import", "t2_import_index",
		"t3_export", "t3_export_index", "t3_import", "t3_import_index",
	}
	require.Equal(t, want, keys)
	var series dto.ConsumptionSeries
	res.json(t, &series)
	require.Equal(t, "230", series.Items[0].ActiveImport.String(), "daily aggregates hold the 23 in-day steps; the midnight step is not in a bucket (04 §4.3)")
	require.True(t, strings.HasSuffix(series.Items[0].PeriodStart.Format(time.RFC3339), "+03:00"))
}

func TestConsumptionSubjectsAndScope(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seedAnalysis()
	ca := h.as(seed.E2ECompanyAdminEmail)
	q := "&granularity=monthly&from=2026-08-01&to=2026-08-31"
	res := ca.do(http.MethodGet, "/consumption?granularity=daily&from=2026-09-01&to=2026-09-02", nil)
	require.Equal(t, http.StatusBadRequest, res.status)
	require.Equal(t, "invalid_parameters", res.code(t))
	require.Equal(t, http.StatusBadRequest, ca.do(http.MethodGet, "/consumption?analyzer_id="+h.fx.AnalyzerA1.String()+"&building_id="+h.fx.BuildingA1.String()+q, nil).status)
	require.Equal(t, http.StatusUnprocessableEntity, ca.do(http.MethodGet, "/consumption?analyzer_id="+h.fx.AnalyzerA1.String()+"&granularity=weekly&from=2026-08-01&to=2026-08-31", nil).status)

	var building dto.ConsumptionSeries
	res = ca.do(http.MethodGet, "/consumption?building_id="+h.fx.BuildingA1.String()+q, nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	res.json(t, &building)
	require.Len(t, building.Items, 1)
	row := building.Items[0]
	require.Nil(t, row.AnalyzerID)
	require.Equal(t, "29760", row.ActiveImport.String(), "(10 + 30) kWh × 744 August hours")
	require.Nil(t, row.ActiveImportIndex)

	var summary dto.ConsumptionSummary
	ca.do(http.MethodGet, "/consumption/summary?building_id="+h.fx.BuildingA1.String()+"&granularity=daily&from=2026-08-01&to=2026-08-31", nil).json(t, &summary)
	require.Equal(t, 31, summary.Rows)
	require.Equal(t, "920", summary.Averages.ActiveImport.String(), "(10 + 30) × 23 in-day steps")

	ba := h.as(seed.E2EBuildingAdminEmail)
	require.Equal(t, http.StatusOK, ba.do(http.MethodGet, "/consumption?analyzer_id="+h.fx.AnalyzerA1.String()+q, nil).status)
	require.Equal(t, http.StatusNotFound, ba.do(http.MethodGet, "/consumption?analyzer_id="+h.fx.AnalyzerA2.String()+q, nil).status)
	require.Equal(t, http.StatusNotFound, ba.do(http.MethodGet, "/consumption?building_id="+h.fx.BuildingA2.String()+q, nil).status)
	require.Equal(t, http.StatusNotFound, ca.do(http.MethodGet, "/consumption?analyzer_id="+h.fx.AnalyzerB1.String()+q, nil).status)

	res = ca.do(http.MethodGet, "/consumption/export?format=csv&analyzer_id="+h.fx.AnalyzerA1.String()+q, nil)
	require.Equal(t, http.StatusOK, res.status)
	require.Equal(t, "text/csv; charset=utf-8", res.header.Get("Content-Type"))
	require.Equal(t, `attachment; filename="tuketim-20260801-20260831.csv"`, res.header.Get("Content-Disposition"))
	require.True(t, strings.HasPrefix(string(res.body), "\xef\xbb\xbfanalyzer_id,period_start,period_end,source,partial,active_import"))
	res = ca.do(http.MethodGet, "/consumption/export?format=xlsx&analyzer_id="+h.fx.AnalyzerA1.String()+q, nil)
	require.Equal(t, http.StatusOK, res.status)
	require.Equal(t, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", res.header.Get("Content-Type"))

	var balance dto.EnergyBalance
	ca.do(http.MethodGet, "/energy-balance?analyzer_id="+h.fx.AnalyzerA1.String()+q, nil).json(t, &balance)
	require.Len(t, balance.Items, 1)
	require.Equal(t, http.StatusOK, ca.do(http.MethodGet, "/generation?building_id="+h.fx.BuildingA1.String()+q, nil).status)
	var anomalies dto.Page[dto.Anomaly]
	res = ca.do(http.MethodGet, "/consumption/anomalies", nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	res.json(t, &anomalies)
	require.Empty(t, anomalies.Items)
}

func TestLoadProfileKeysAndLabels(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seedAnalysis()
	ca := h.as(seed.E2ECompanyAdminEmail)
	base := "/load-profile?analyzer_id=" + h.fx.AnalyzerA1.String() + "&from=2026-08-01&to=2026-09-16"
	var profiles dto.LoadProfiles
	res := ca.do(http.MethodGet, base, nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	res.json(t, &profiles)
	var keys []string
	for k := range profiles.Profiles {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	require.Equal(t, []string{"autumn_weekday", "autumn_weekend", "spring_weekday", "spring_weekend", "summer_weekday", "summer_weekend",
		"weekday", "weekend", "winter_weekday", "winter_weekend"}, keys)
	require.Len(t, profiles.Profiles["weekday"], 24)
	require.Equal(t, "10", profiles.Profiles["weekday"][10].String())
	require.Equal(t, []int{0, 6}, profiles.Config.WeekendDays)
	require.Equal(t, "default", profiles.Config.WeekendSource)

	res = ca.do(http.MethodGet, base+"&profiles=weekday,summer_holiday", nil)
	require.Equal(t, http.StatusBadRequest, res.status)
	var stats dto.LoadProfileStatistics
	ca.do(http.MethodGet, base+"&profiles=weekday", nil)
	res = ca.do(http.MethodGet, "/load-profile/statistics?analyzer_id="+h.fx.AnalyzerA1.String()+"&from=2026-08-01&to=2026-09-16&profiles=weekday", nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	res.json(t, &stats)
	require.Equal(t, "1", stats.Statistics["weekday"].LoadFactor.String(), "a flat load has load factor 1")
	res = ca.do(http.MethodGet, "/load-profile/export?analyzer_id="+h.fx.AnalyzerA1.String()+"&from=2026-08-01&to=2026-09-16", nil)
	require.Equal(t, http.StatusOK, res.status)
	require.Equal(t, http.StatusNotFound, h.as(seed.E2EBuildingAdminEmail).do(http.MethodGet, "/load-profile?analyzer_id="+h.fx.AnalyzerA2.String()+"&from=2026-08-01&to=2026-09-16", nil).status)
}

func TestReactiveStatusThresholds(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	second := h.seedAnalysis()
	ca := h.as(seed.E2ECompanyAdminEmail)
	require.Equal(t, http.StatusOK, ca.do(http.MethodPatch, "/analyzers/"+h.fx.AnalyzerA2.String(), map[string]any{"installed_power_kw": "8"}).status)

	res := ca.do(http.MethodGet, "/consumption/reactive-status?month=2026-08", nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	var status dto.ReactiveStatus
	res.json(t, &status)
	require.Equal(t, "2026-08", status.Month)
	byID := map[uuid.UUID]dto.ReactiveAnalyzer{}
	for _, a := range status.Analyzers {
		byID[a.AnalyzerID] = a
	}
	a1 := byID[h.fx.AnalyzerA1]
	require.True(t, a1.HasData)
	require.Equal(t, "0.2", a1.InductiveLimit.String(), "120 kW is in the ≥30 kW band")
	require.True(t, a1.PenaltyApplies, "inductive ratio 0.35 exceeds 0.20")
	require.Nil(t, a1.ExemptReason)
	a2 := byID[h.fx.AnalyzerA2]
	require.Equal(t, "below_kw", *a2.ExemptReason)
	require.False(t, a2.PenaltyApplies)
	require.False(t, byID[second].PenaltyApplies, "0.20 does not exceed 0.20")
	require.Equal(t, h.fx.AnalyzerA1, status.HighestInductive.AnalyzerID)
	require.Equal(t, "0.35", status.HighestInductive.Ratio.String())
	_, foreign := byID[h.fx.AnalyzerB1]
	require.False(t, foreign, "another company's analyzers never appear")
}

func TestAnomalyCheckUnavailable(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	body := map[string]any{"analyzer_id": h.fx.AnalyzerA1.String(), "ts": "2026-09-01T10:00:00+03:00", "actual": "0"}
	res := ca.do(http.MethodPost, "/anomaly/check", body)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	var check dto.AnomalyCheck
	res.json(t, &check)
	require.Equal(t, dto.AnomalyCheck{Available: false, Reason: "ml_service_unavailable"}, check)
	body["analyzer_id"] = h.fx.AnalyzerB1.String()
	require.Equal(t, http.StatusNotFound, ca.do(http.MethodPost, "/anomaly/check", body).status)
	require.Equal(t, http.StatusForbidden, h.as(seed.E2ECompanyReadonlyEmail).do(http.MethodPost, "/anomaly/check", body).status)
}
