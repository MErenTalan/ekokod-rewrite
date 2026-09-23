//go:build integration

package solar_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/solar"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

func readHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, inverterFake(), okOpener())
	h.svc = solar.New(solar.Deps{
		Plants: postgres.NewPlantRepository(h.pool), Production: postgres.NewProductionRepository(h.pool),
		Totals: postgres.NewProductionTotalsRepository(h.pool), Faults: postgres.NewFaultRepository(h.pool),
		Solar: postgres.NewSolarTariffRepository(h.pool), Analytics: postgres.NewAnalyticsRepository(h.pool),
		Ops: postgres.NewOpsRepository(h.pool), Creds: okOpener(), ISolar: h.fake, Clock: fixedClock(syncNow),
		Enqueuer: h.enq, Inspector: noInspector{},
	})
	return h
}

func (h *harness) withDevice(t *testing.T, sn string, typ int32, power string, at time.Time) {
	t.Helper()
	plants := postgres.NewPlantRepository(h.pool)
	key := "K-" + sn
	dev, err := plants.UpsertDevice(context.Background(), h.sc, model.PlantDevice{PlantID: h.plant.ID, DeviceSN: sn, DeviceType: &typ, ProviderKey: &key})
	require.NoError(t, err)
	status := int32(4)
	dev.SnapshotAt, dev.FaultStatus, dev.ActivePowerKw = &at, &status, d(power)
	dev.YieldTodayKwh, dev.YieldTotalKwh = d("10"), d("1000")
	require.NoError(t, plants.UpdateDeviceSnapshot(context.Background(), h.sc, dev))
}

func (h *harness) setInstalled(t *testing.T, kw string) {
	t.Helper()
	p := h.plant
	p.IsolarInstalledKw = d(kw)
	var err error
	h.plant, err = postgres.NewPlantRepository(h.pool).SetIsolarLink(context.Background(), h.sc, p)
	require.NoError(t, err)
}

func (h *harness) seedDays(t *testing.T, from time.Time, n int, kwh string) {
	t.Helper()
	var rows []model.PlantProductionTotal
	var days []time.Time
	for i := range n {
		day := from.AddDate(0, 0, i)
		rows = append(rows, model.PlantProductionTotal{PlantID: h.plant.ID, Ts: day.Add(12 * time.Hour), ProductionKwh: d(kwh), Basis: model.BasisPlantMeter})
		days = append(days, day)
	}
	_, err := postgres.NewProductionTotalsRepository(h.pool).ReplaceDays(context.Background(), h.sc, h.plant.ID, days, rows)
	require.NoError(t, err)
}

func TestRealtimeSumsInvertersAndUtilisation(t *testing.T) {
	t.Parallel()
	h := readHarness(t)
	h.setInstalled(t, "10")
	h.withDevice(t, "A", 1, "3.2", syncNow.Add(-10*time.Minute))
	h.withDevice(t, "B", 14, "1.8", syncNow.Add(-20*time.Minute))
	h.withDevice(t, "M", 7, "99", syncNow.Add(-5*time.Minute)) // a meter is not an inverter
	require.NoError(t, postgres.NewPlantRepository(h.pool).SetSyncState(context.Background(), h.sc, h.plant.ID, syncNow.Add(-5*time.Minute), nil))

	rt, err := h.svc.Realtime(context.Background(), h.sc, h.plant.ID)
	require.NoError(t, err)
	requireDec(t, "5.0", rt.ActivePowerKw, "3.2 + 1.8")
	requireDec(t, "50.0", rt.CapacityUtilisationPct, "5 of 10 kW")
	requireDec(t, "20", rt.YieldTodayKwh, "two inverters")
	requireDec(t, "", rt.YieldMonthKwh, "no inverter reports it")
	require.Equal(t, 2, rt.InverterCount)
	require.True(t, rt.AsOf.Equal(syncNow.Add(-20*time.Minute)), "the oldest snapshot")
	require.False(t, rt.Stale)
	require.Equal(t, "connected", rt.Connection)
}

func TestRealtimeStaleAndConnectionError(t *testing.T) {
	t.Parallel()
	h := readHarness(t)
	h.withDevice(t, "A", 1, "3.2", syncNow.Add(-3*time.Hour))
	code := "isolar_auth"
	require.NoError(t, postgres.NewPlantRepository(h.pool).SetSyncState(context.Background(), h.sc, h.plant.ID, syncNow.Add(-time.Minute), &code))
	rt, err := h.svc.Realtime(context.Background(), h.sc, h.plant.ID)
	require.NoError(t, err)
	require.True(t, rt.Stale)
	require.Equal(t, "error", rt.Connection)
	require.Equal(t, "isolar_auth", *rt.ConnectionError)
}

func TestRealtimeHiddenFromBuildingScope(t *testing.T) {
	t.Parallel()
	h := readHarness(t)
	_, err := h.svc.Realtime(context.Background(), h.tenant.Scope, h.plant.ID)
	require.ErrorIs(t, err, store.ErrNotFound, "plants are company-level (F-1)")
}

func TestProductionSeriesRangeLimits(t *testing.T) {
	t.Parallel()
	h := readHarness(t)
	for gran, days := range map[string]int{"hour": 32, "day": 367} {
		_, err := h.svc.ProductionSeries(context.Background(), h.sc, h.plant.ID, gran, at(1, 0), at(1, 0).AddDate(0, 0, days))
		require.Equal(t, "validation_failed", perr.CodeOf(err), gran)
	}
	_, err := h.svc.ProductionSeries(context.Background(), h.sc, h.plant.ID, "week", at(1, 0), at(2, 0))
	require.Equal(t, "validation_failed", perr.CodeOf(err))
}

func TestProductionSeriesHourAndDay(t *testing.T) {
	t.Parallel()
	h := readHarness(t)
	_, err := postgres.NewProductionTotalsRepository(h.pool).ReplaceDays(context.Background(), h.sc, h.plant.ID, []time.Time{at(9, 0)}, []model.PlantProductionTotal{
		{PlantID: h.plant.ID, Ts: at(9, 10).Add(15 * time.Minute), ProductionKwh: d("1"), Basis: model.BasisPlantMeter},
		{PlantID: h.plant.ID, Ts: at(9, 10).Add(45 * time.Minute), ProductionKwh: d("2"), Basis: model.BasisPlantMeter},
		{PlantID: h.plant.ID, Ts: at(9, 11), ProductionKwh: d("4"), Basis: model.BasisPlantMeter},
	})
	require.NoError(t, err)
	h.seedDays(t, at(8, 0), 1, "7")
	hours, err := h.svc.ProductionSeries(context.Background(), h.sc, h.plant.ID, "hour", at(9, 0), at(10, 0))
	require.NoError(t, err)
	require.Len(t, hours.Points, 2)
	require.True(t, hours.Points[0].Ts.Equal(at(9, 10)))
	requireDec(t, "3", hours.Points[0].ProductionKwh, "two quarter-hours")
	require.Equal(t, model.BasisPlantMeter, *hours.Points[0].Basis)

	days, err := h.svc.ProductionSeries(context.Background(), h.sc, h.plant.ID, "day", at(8, 0), at(10, 0))
	require.NoError(t, err)
	require.Len(t, days.Points, 2)
	requireDec(t, "7", days.Points[0].ProductionKwh, "day 8")
	requireDec(t, "7", days.Points[1].ProductionKwh, "day 9 = 1+2+4")
	require.False(t, days.MixedBasis)
}

func TestProductionMonthComposesOpenMonth(t *testing.T) {
	t.Parallel()
	h := readHarness(t)
	h.seedDays(t, time.Date(2026, time.February, 26, 0, 0, 0, 0, ist), 3, "10") // Feb 26–28
	h.seedDays(t, at(1, 0), 10, "5")                                            // Mar 1–10, the open month
	months, err := h.svc.ProductionSeries(context.Background(), h.sc, h.plant.ID, "month",
		time.Date(2026, time.February, 1, 0, 0, 0, 0, ist), time.Date(2026, time.April, 1, 0, 0, 0, 0, ist))
	require.NoError(t, err)
	require.Len(t, months.Points, 2)
	requireDec(t, "30", months.Points[0].ProductionKwh, "February")
	requireDec(t, "50", months.Points[1].ProductionKwh, "March so far")
}

func TestDevicesSearchSerialCaseInsensitive(t *testing.T) {
	t.Parallel()
	h := readHarness(t)
	h.withDevice(t, "INV-Alpha", 1, "1", syncNow.Add(-time.Minute))
	h.withDevice(t, "INV-Beta", 1, "1", syncNow.Add(-3*time.Hour))
	all, err := h.svc.Devices(context.Background(), h.sc, h.plant.ID, "")
	require.NoError(t, err)
	require.Len(t, all, 2)
	got, err := h.svc.Devices(context.Background(), h.sc, h.plant.ID, "alpha")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "INV-Alpha", got[0].DeviceSN)
	require.Equal(t, "normal", *got[0].Status)
	beta, err := h.svc.Devices(context.Background(), h.sc, h.plant.ID, "BETA")
	require.NoError(t, err)
	require.Equal(t, "offline", *beta[0].Status, "a snapshot older than two hours")
}

func TestRevenueTotalSinceFirstDay(t *testing.T) {
	t.Parallel()
	h := readHarness(t)
	h.seedDays(t, at(1, 0), 10, "10")
	_, err := postgres.NewSolarTariffRepository(h.pool).Create(context.Background(), h.sc, model.SolarTariff{CompanyID: h.tenant.Company.ID,
		PlantID: h.plant.ID, EffectiveFrom: at(5, 0), FeedInTariff: decimal.RequireFromString("2.0"), Currency: model.CurrencyTRY})
	require.NoError(t, err)

	r, err := h.svc.Revenue(context.Background(), h.sc, h.plant.ID)
	require.NoError(t, err)
	require.True(t, r.Available)
	require.Equal(t, "20.00", r.Daily.Amounts[0].Amount.StringFixed(2), "today, 10 kWh × 2")
	require.Equal(t, "120.00", r.Monthly.Amounts[0].Amount.StringFixed(2), "6 priced days")
	require.Equal(t, 4, r.Monthly.UnpricedDays)
	require.True(t, r.Monthly.Partial)
	require.True(t, r.Total.Since.Equal(at(1, 0)))
}

func TestRevenueWithoutTariffIsUnavailable(t *testing.T) {
	t.Parallel()
	h := readHarness(t)
	h.seedDays(t, at(1, 0), 2, "10")
	r, err := h.svc.Revenue(context.Background(), h.sc, h.plant.ID)
	require.NoError(t, err)
	require.False(t, r.Available)
	require.Equal(t, "no_solar_tariff", r.Reason)
}

func linkHarness(t *testing.T, opener solar.CredentialOpener) (*harness, uuid.UUID) {
	t.Helper()
	h := readHarness(t)
	var credID uuid.UUID
	require.NoError(t, h.pool.QueryRow(context.Background(), `select isolar_credential_id from power_plants where id = $1`, h.plant.ID).Scan(&credID))
	kw := decimal.RequireFromString("250")
	typ := int32(1)
	h.fake.plants = []isolar.Plant{{PSID: "PS-NEW", Name: "Konya GES", InstalledKw: &kw}, {PSID: *h.tenant.Plants[0].IsolarPsID, Name: "Taken"}}
	h.fake.devices = []isolar.Device{{PSKey: "PK-1", DeviceSN: "SN-NEW", DeviceType: &typ}}
	h.svc = solar.New(solar.Deps{
		Plants: postgres.NewPlantRepository(h.pool), Production: postgres.NewProductionRepository(h.pool),
		Totals: postgres.NewProductionTotalsRepository(h.pool), Faults: postgres.NewFaultRepository(h.pool),
		Ops: postgres.NewOpsRepository(h.pool), Creds: opener, ISolar: h.fake, Clock: fixedClock(syncNow),
		Enqueuer: h.enq, Inspector: noInspector{},
	})
	return h, credID
}

func TestLinkKeepsPlantNameAndEnqueuesBackfill(t *testing.T) {
	t.Parallel()
	h, credID := linkHarness(t, okOpener())
	unlinked := h.plant
	unlinked.IsolarPsID, unlinked.TotalCapacityKw = nil, nil
	_, err := postgres.NewPlantRepository(h.pool).SetIsolarLink(context.Background(), h.sc, unlinked)
	require.NoError(t, err)

	p, jobID, err := h.svc.Link(context.Background(), h.sc, h.plant.ID, credID, "PS-NEW")
	require.NoError(t, err)
	require.Equal(t, h.plant.Name, p.Name, "the plant's own name is kept (R281)")
	require.Equal(t, "Konya GES", *p.IsolarPsName)
	requireDec(t, "250", p.IsolarInstalledKw, "imported")
	requireDec(t, "250", p.TotalCapacityKw, "filled only because it was empty")
	require.Equal(t, "isolar.sync_plant:"+h.plant.ID.String()+":400", jobID)
	devices, err := postgres.NewPlantRepository(h.pool).Devices(context.Background(), h.sc, h.plant.ID)
	require.NoError(t, err)
	require.Len(t, devices, 1)

	require.NoError(t, h.svc.Unlink(context.Background(), h.sc, h.plant.ID))
	p, err = postgres.NewPlantRepository(h.pool).Get(context.Background(), h.sc, h.plant.ID)
	require.NoError(t, err)
	require.Nil(t, p.IsolarPsID)
	require.Nil(t, p.IsolarCredentialID)
}

func TestLinkConflictWhenLinkedElsewhere(t *testing.T) {
	t.Parallel()
	h, credID := linkHarness(t, okOpener())
	_, _, err := h.svc.Link(context.Background(), h.sc, h.plant.ID, credID, *h.tenant.Plants[0].IsolarPsID)
	require.Equal(t, "isolar_plant_already_linked", perr.CodeOf(err))
	_, _, err = h.svc.Link(context.Background(), h.sc, h.plant.ID, credID, "PS-NOT-THERE")
	require.Equal(t, "validation_failed", perr.CodeOf(err))
}

func TestLinkRejectsForeignOrWrongCredential(t *testing.T) {
	t.Parallel()
	foreign := openerFunc(func(context.Context, store.Scope, uuid.UUID) (integration.Credentials, error) {
		return integration.Credentials{}, store.ErrNotFound
	})
	h, credID := linkHarness(t, foreign)
	_, _, err := h.svc.Link(context.Background(), h.sc, h.plant.ID, credID, "PS-NEW")
	require.Equal(t, "validation_failed", perr.CodeOf(err), "another company's credential is not_isolar, never a hint that it exists")

	osos := openerFunc(func(context.Context, store.Scope, uuid.UUID) (integration.Credentials, error) {
		return integration.Credentials{Provider: integration.ProviderOSOS}, nil
	})
	h, credID = linkHarness(t, osos)
	_, _, err = h.svc.Link(context.Background(), h.sc, h.plant.ID, credID, "PS-NEW")
	require.Equal(t, "validation_failed", perr.CodeOf(err))
}

func TestSetRecipientsValidatesAndDedupes(t *testing.T) {
	t.Parallel()
	h := readHarness(t)
	got, err := h.svc.SetRecipients(context.Background(), h.sc, h.plant.ID, []string{"A@x.test", "a@x.test", " b@x.test "})
	require.NoError(t, err)
	require.Equal(t, []string{"a@x.test", "b@x.test"}, got)
	_, err = h.svc.SetRecipients(context.Background(), h.sc, h.plant.ID, []string{"not-an-address"})
	require.Equal(t, "validation_failed", perr.CodeOf(err))
}

func fixedClock(t time.Time) clock.Clock { return clock.NewFake(t) }
