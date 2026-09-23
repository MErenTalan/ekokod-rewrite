//go:build integration

package solar_test

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/solar"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// fakeISolar serves canned data, filtered to each call's window like the real adapter.
type fakeISolar struct {
	plants   []isolar.Plant
	devices  []isolar.Device
	device   map[string][]isolar.YieldSample
	plant    []isolar.YieldSample
	daily    []isolar.DailyYield
	realtime []isolar.DeviceSnapshot
	faults   []isolar.Fault
	err      error
	calls    map[string]int
	dailyWin [][2]time.Time
}

func (f *fakeISolar) hit(op string) error {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[op]++
	return f.err
}

func inWindow(ts, from, to time.Time) bool { return !ts.Before(from) && ts.Before(to) }

func (f *fakeISolar) Plants(context.Context, integration.Credentials) ([]isolar.Plant, error) {
	return f.plants, f.hit("plants")
}
func (f *fakeISolar) Devices(context.Context, integration.Credentials, string) ([]isolar.Device, error) {
	return f.devices, f.hit("devices")
}
func (f *fakeISolar) DeviceMinuteSeries(_ context.Context, _ integration.Credentials, keys []string, from, to time.Time) ([]isolar.YieldSample, error) {
	var out []isolar.YieldSample
	for _, k := range keys {
		for _, s := range f.device[k] {
			if inWindow(s.Ts, from, to) {
				key := k
				s.PSKey = &key
				out = append(out, s)
			}
		}
	}
	return out, f.hit("device_minute")
}
func (f *fakeISolar) PlantMinuteSeries(_ context.Context, _ integration.Credentials, _ string, from, to time.Time) ([]isolar.YieldSample, error) {
	var out []isolar.YieldSample
	for _, s := range f.plant {
		if inWindow(s.Ts, from, to) {
			out = append(out, s)
		}
	}
	return out, f.hit("plant_minute")
}
func (f *fakeISolar) PlantDailySeries(_ context.Context, _ integration.Credentials, _ string, from, to time.Time) ([]isolar.DailyYield, error) {
	f.dailyWin = append(f.dailyWin, [2]time.Time{from, to})
	var out []isolar.DailyYield
	for _, d := range f.daily {
		if inWindow(d.Day, from, to) {
			out = append(out, d)
		}
	}
	return out, f.hit("daily")
}
func (f *fakeISolar) DeviceRealtime(_ context.Context, _ integration.Credentials, _ int32, keys []string) ([]isolar.DeviceSnapshot, error) {
	return f.realtime, f.hit("realtime")
}
func (f *fakeISolar) Faults(_ context.Context, _ integration.Credentials, from, to time.Time) ([]isolar.Fault, error) {
	var out []isolar.Fault
	for _, x := range f.faults {
		if inWindow(x.OccurredAt, from, to) {
			out = append(out, x)
		}
	}
	return out, f.hit("faults")
}

type openerFunc func(ctx context.Context, sc store.Scope, id uuid.UUID) (integration.Credentials, error)

func (o openerFunc) Open(ctx context.Context, sc store.Scope, id uuid.UUID) (integration.Credentials, error) {
	return o(ctx, sc, id)
}

func okOpener() openerFunc {
	return func(context.Context, store.Scope, uuid.UUID) (integration.Credentials, error) {
		return integration.Credentials{Provider: integration.ProviderISolar, IsActive: true}, nil
	}
}

type recordingEnqueuer struct{ tasks []*asynq.Task }

func (r *recordingEnqueuer) Enqueue(_ context.Context, t *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	r.tasks = append(r.tasks, t)
	return &asynq.TaskInfo{ID: "id-" + t.Type()}, nil
}

type noInspector struct{}

func (noInspector) GetTaskInfo(string, string) (*asynq.TaskInfo, error) {
	return nil, asynq.ErrTaskNotFound
}
func (noInspector) DeleteTask(string, string) error { return nil }

// syncNow is 12:00 Istanbul on 10 March 2026; "today" is the 10th.
var syncNow = time.Date(2026, time.March, 10, 12, 0, 0, 0, ist)

type harness struct {
	pool   *pgxpool.Pool
	tenant testfixtures.Tenant
	plant  model.PowerPlant
	svc    *solar.Service
	fake   *fakeISolar
	enq    *recordingEnqueuer
	sc     store.Scope
}

func newHarness(t *testing.T, fake *fakeISolar, opener solar.CredentialOpener) *harness {
	t.Helper()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	defID, credID := uuid.New(), uuid.New()
	_, err := pool.Exec(ctx, `insert into integration_definitions (id, provider, subtype) values ($1, 'isolar', 'SolarTest')`, defID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `insert into integration_credentials (id, company_id, definition_id, is_active) values ($1, $2, $3, true)`,
		credID, tenant.Company.ID, defID)
	require.NoError(t, err)
	plants := postgres.NewPlantRepository(pool)
	plant := tenant.Plants[1]
	plant.IsolarCredentialID = &credID
	plant, err = plants.SetIsolarLink(ctx, tenant.AdminScope, plant)
	require.NoError(t, err)

	enq := &recordingEnqueuer{}
	svc := solar.New(solar.Deps{
		Plants: plants, Production: postgres.NewProductionRepository(pool), Totals: postgres.NewProductionTotalsRepository(pool),
		Faults: postgres.NewFaultRepository(pool), Ops: postgres.NewOpsRepository(pool), AdminSolar: admin.NewSolarRepository(pool),
		Creds: opener, ISolar: fake, Clock: clock.NewFake(syncNow), Enqueuer: enq, Inspector: noInspector{},
	})
	return &harness{pool: pool, tenant: tenant, plant: plant, svc: svc, fake: fake, enq: enq, sc: tenant.AdminScope}
}

func inverterFake() *fakeISolar {
	sn := "SN-1"
	typ := int32(1)
	status := int32(4)
	return &fakeISolar{
		devices: []isolar.Device{{PSKey: "K1", DeviceSN: sn, DeviceType: &typ}},
		device: map[string][]isolar.YieldSample{"K1": {
			{Ts: at(10, 9), YieldTodayKwh: d("1.0")}, {Ts: at(10, 10), YieldTodayKwh: d("3.0")},
		}},
		plant: []isolar.YieldSample{{Ts: at(10, 9), YieldTodayKwh: d("1.2"), ActivePowerKw: d("4")}, {Ts: at(10, 10), YieldTodayKwh: d("3.5")}},
		realtime: []isolar.DeviceSnapshot{{PSKey: "K1", DeviceSN: sn, FaultStatus: &status, At: syncNow.Add(-10 * time.Minute),
			ActivePowerKw: d("3.2"), YieldTodayKwh: d("3.0"), YieldTotalKwh: d("45000")}},
	}
}

func (h *harness) totals(t *testing.T, from, to time.Time) []model.PlantProductionTotal {
	t.Helper()
	rows, err := postgres.NewProductionTotalsRepository(h.pool).Range(context.Background(), h.sc, h.plant.ID, store.TimeRange{From: from, To: to})
	require.NoError(t, err)
	return rows
}

func TestSyncWritesPlantMeterTotals(t *testing.T) {
	t.Parallel()
	h := newHarness(t, inverterFake(), okOpener())
	res, err := h.svc.SyncPlant(context.Background(), h.tenant.Company.ID, h.plant.ID, 0)
	require.NoError(t, err)
	require.Equal(t, 1, res.Snapshots)

	rows := h.totals(t, at(10, 0), at(11, 0))
	require.Len(t, rows, 2)
	requireDec(t, "1.2", rows[0].ProductionKwh, "first of day")
	requireDec(t, "2.3", rows[1].ProductionKwh, "delta")
	require.Equal(t, model.BasisPlantMeter, rows[0].Basis)
	requireDec(t, "4", rows[0].ActivePowerKw, "plant power")

	devices, err := postgres.NewPlantRepository(h.pool).Devices(context.Background(), h.sc, h.plant.ID)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	requireDec(t, "3.2", devices[0].ActivePowerKw, "snapshot power")
	require.Equal(t, "normal", *devices[0].Status)
	dev, err := postgres.NewProductionRepository(h.pool).Range(context.Background(), h.sc, h.plant.ID, store.TimeRange{From: at(10, 0), To: at(11, 0)})
	require.NoError(t, err)
	require.Len(t, dev, 2)
	requireDec(t, "2.0", dev[1].ProductionKwh, "device interval")

	p, err := postgres.NewPlantRepository(h.pool).Get(context.Background(), h.sc, h.plant.ID)
	require.NoError(t, err)
	require.NotNil(t, p.IsolarLastSyncAt)
	require.Nil(t, p.IsolarLastSyncError)
}

func TestSyncFallsBackToInverterSum(t *testing.T) {
	t.Parallel()
	fake := inverterFake()
	fake.plant = nil
	h := newHarness(t, fake, okOpener())
	_, err := h.svc.SyncPlant(context.Background(), h.tenant.Company.ID, h.plant.ID, 0)
	require.NoError(t, err)
	rows := h.totals(t, at(10, 0), at(11, 0))
	require.Len(t, rows, 2)
	require.Equal(t, model.BasisInverterSum, rows[0].Basis)
	requireDec(t, "1.0", rows[0].ProductionKwh, "device first of day")
	requireDec(t, "2.0", rows[1].ProductionKwh, "device delta")
}

func TestSyncBackfillUsesDailyTotalsBeforeYesterday(t *testing.T) {
	t.Parallel()
	fake := inverterFake()
	fake.daily = []isolar.DailyYield{{Day: at(5, 0), Kwh: d("40")}, {Day: at(8, 0), Kwh: d("38")}, {Day: at(9, 0), Kwh: d("99")}}
	h := newHarness(t, fake, okOpener())
	_, err := h.svc.SyncPlant(context.Background(), h.tenant.Company.ID, h.plant.ID, 5)
	require.NoError(t, err)

	require.Len(t, fake.dailyWin, 1)
	require.True(t, fake.dailyWin[0][0].Equal(at(5, 0)), "from = today − 5 days")
	require.True(t, fake.dailyWin[0][1].Equal(at(9, 0)), "to = yesterday: yesterday and today come from minutes")
	rows := h.totals(t, at(1, 0), at(9, 0))
	require.Len(t, rows, 2)
	require.True(t, rows[0].Ts.Equal(at(5, 0)))
	require.Equal(t, model.BasisDailyTotal, rows[0].Basis)
	requireDec(t, "38", rows[1].ProductionKwh, "day 8")
}

func TestSyncTwiceSameRows(t *testing.T) {
	t.Parallel()
	h := newHarness(t, inverterFake(), okOpener())
	ctx := context.Background()
	_, err := h.svc.SyncPlant(ctx, h.tenant.Company.ID, h.plant.ID, 0)
	require.NoError(t, err)
	first := h.totals(t, at(9, 0), at(11, 0))
	_, err = h.svc.SyncPlant(ctx, h.tenant.Company.ID, h.plant.ID, 0)
	require.NoError(t, err)
	second := h.totals(t, at(9, 0), at(11, 0))
	require.Equal(t, first, second)

	_, err = h.pool.Exec(ctx, `call refresh_continuous_aggregate('plant_production_daily', NULL, NULL)`)
	require.NoError(t, err)
	buckets, err := postgres.NewAnalyticsRepository(h.pool).ProductionDaily(ctx, h.sc, []uuid.UUID{h.plant.ID}, store.TimeRange{From: at(10, 0), To: at(11, 0)})
	require.NoError(t, err)
	require.Len(t, buckets, 1)
	requireDec(t, "3.5", buckets[0].ProductionKwh, "the day is its yield, never doubled")
}

func TestSyncAuthFailureRecordsCode(t *testing.T) {
	t.Parallel()
	opener := openerFunc(func(context.Context, store.Scope, uuid.UUID) (integration.Credentials, error) {
		return integration.Credentials{}, &integration.Error{Kind: integration.ErrAuth, Provider: integration.ProviderISolar, Op: "token"}
	})
	h := newHarness(t, inverterFake(), opener)
	p := job.SolarSyncPayload{CompanyID: h.tenant.Company.ID, PlantID: h.plant.ID}
	err := h.svc.SyncPlantTask(context.Background(), p)
	require.NoError(t, err, "an auth failure will not heal by retrying: recorded, not retried")

	plant, err := postgres.NewPlantRepository(h.pool).Get(context.Background(), h.sc, h.plant.ID)
	require.NoError(t, err)
	require.Equal(t, "isolar_auth", *plant.IsolarLastSyncError)
	run, err := postgres.NewOpsRepository(h.pool).RunByTaskID(context.Background(), h.sc, job.SolarSyncTaskID(p))
	require.NoError(t, err)
	require.Equal(t, "failed", run.Status)
	require.JSONEq(t, `{"code":"isolar_auth"}`, string(run.Detail))
}

func TestSyncUnavailableIsRetried(t *testing.T) {
	t.Parallel()
	fake := inverterFake()
	fake.err = &integration.Error{Kind: integration.ErrUpstreamUnavailable, Provider: integration.ProviderISolar, Op: "devices"}
	h := newHarness(t, fake, okOpener())
	err := h.svc.SyncPlantTask(context.Background(), job.SolarSyncPayload{CompanyID: h.tenant.Company.ID, PlantID: h.plant.ID})
	require.Error(t, err)
	require.True(t, errors.Is(err, integration.ErrUpstreamUnavailable))
	plant, err := postgres.NewPlantRepository(h.pool).Get(context.Background(), h.sc, h.plant.ID)
	require.NoError(t, err)
	require.Equal(t, "isolar_unavailable", *plant.IsolarLastSyncError)
}

func TestDispatchEnqueuesEveryLinkedPlant(t *testing.T) {
	t.Parallel()
	h := newHarness(t, inverterFake(), okOpener())
	n, err := h.svc.DispatchSync(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, n, "both fixture plants carry an iSolar ps id")
	var ids []string
	for _, task := range h.enq.tasks {
		require.Equal(t, job.TypeSolarSyncPlant, task.Type())
		ids = append(ids, string(task.Payload()))
	}
	sort.Strings(ids)
	require.Contains(t, ids[0]+ids[1], h.plant.ID.String())
}
