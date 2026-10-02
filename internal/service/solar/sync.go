package solar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Closed sync failure codes (R288); nothing else leaves the worker.
const (
	CodeISolarAuth         = "isolar_auth"
	CodeISolarUnavailable  = "isolar_unavailable"
	CodeNotLinked          = "isolar_not_linked"
	CodeCredentialMissing  = "credential_missing"
	CodeCredentialInactive = "credential_inactive"
)

// dailyChunk is MaxWindowDay in days (31); R281's backfill runs in these chunks.
const dailyChunk = 31

// SyncResult counts what one sync wrote.
type SyncResult struct {
	DevicesUpserted, Snapshots, TotalRows, DeviceRows, Resets int
	Basis                                                     map[string]int
}

// syncError carries a closed code; retry says whether asynq should try again.
type syncError struct {
	code  string
	retry bool
	err   error
}

func (e *syncError) Error() string { return "solar sync: " + e.code }
func (e *syncError) Unwrap() error { return e.err }

func classify(err error) *syncError {
	var se *syncError
	switch {
	case errors.As(err, &se):
		return se
	case errors.Is(err, integration.ErrAuth), errors.Is(err, integration.ErrConfig):
		return &syncError{code: CodeISolarAuth, err: err}
	case errors.Is(err, store.ErrNotFound):
		return &syncError{code: CodeCredentialMissing, err: err}
	default:
		return &syncError{code: CodeISolarUnavailable, retry: true, err: err}
	}
}

// SyncPlantTask is isolar.sync_plant: sync, then journal the run under the
// task id (R267) and the plant's sync state. Only a transient failure is
// returned for retry; auth, missing link and missing credential are final.
func (s *Service) SyncPlantTask(ctx context.Context, p job.SolarSyncPayload) error {
	sc := store.SystemScope(p.CompanyID)
	taskID := job.SolarSyncTaskID(p)
	scope, _ := json.Marshal(map[string]any{"plant_id": p.PlantID, "backfill_days": p.BackfillDays})
	companyID := p.CompanyID
	run, err := s.d.Ops.StartRun(ctx, sc, model.JobRun{CompanyID: &companyID, JobType: job.TypeSolarSyncPlant, TaskID: &taskID,
		Scope: scope, StartedAt: s.d.Clock.Now(), Status: "running"})
	if err != nil {
		return err
	}
	res, syncErr := s.SyncPlant(ctx, p.CompanyID, p.PlantID, p.BackfillDays)
	status, detail := "success", map[string]any{"total_rows": res.TotalRows, "device_rows": res.DeviceRows, "resets": res.Resets}
	var failure *syncError
	if syncErr != nil {
		failure = classify(syncErr)
		status, detail = "failed", map[string]any{"code": failure.code}
	}
	raw, _ := json.Marshal(detail)
	failed := int32(0)
	if failure != nil {
		failed = 1
	}
	if _, err := s.d.Ops.FinishRun(ctx, sc, run.ID, status, 1-failed, 0, failed, nil, raw, s.d.Clock.Now()); err != nil {
		return err
	}
	if failure != nil && failure.retry {
		return failure.err
	}
	return nil
}

// SyncPlant fetches and stores one plant: devices, snapshots, yesterday and
// today from minute series, and the days before yesterday that are still owed
// from the daily series (R279, R506). It records the plant's sync state either way.
func (s *Service) SyncPlant(ctx context.Context, companyID, plantID uuid.UUID, backfillDays int) (SyncResult, error) {
	sc := store.SystemScope(companyID)
	now := s.d.Clock.Now()
	res, err := s.syncPlant(ctx, sc, plantID, backfillDays, now)
	var code *string
	if err != nil {
		c := classify(err).code
		code = &c
	}
	if stateErr := s.d.Plants.SetSyncState(ctx, sc, plantID, now, code); stateErr != nil && err == nil {
		return res, stateErr
	}
	return res, err
}

func (s *Service) syncPlant(ctx context.Context, sc store.Scope, plantID uuid.UUID, backfillDays int, now time.Time) (SyncResult, error) {
	res := SyncResult{Basis: map[string]int{}}
	plant, err := s.d.Plants.Get(ctx, sc, plantID)
	if err != nil {
		return res, err
	}
	if plant.IsolarPsID == nil {
		return res, &syncError{code: CodeNotLinked}
	}
	if plant.IsolarCredentialID == nil {
		return res, &syncError{code: CodeCredentialMissing}
	}
	creds, err := s.openActive(ctx, sc, *plant.IsolarCredentialID)
	if err != nil {
		return res, err
	}
	psID := *plant.IsolarPsID
	today := dayOf(now)
	yesterday := today.AddDate(0, 0, -1)

	// R506: the live half and the history are independent. A failure of one
	// is the sync's outcome, but it does not keep the other from arriving.
	liveErr := s.syncLive(ctx, sc, creds, plant, psID, today, now, &res)
	if isFinal(liveErr) {
		return res, liveErr // the credential is refused: the history call would be too
	}

	from := historyFrom(plant.IsolarHistoryThrough, today, backfillDays)
	if from.Before(yesterday) {
		// With no mark yet, a stored day's intervals are all that is known
		// about it; days owed after a mark stop where their sync did.
		keepIntervals := plant.IsolarHistoryThrough == nil
		if err := s.backfill(ctx, sc, creds, plant.ID, psID, from, yesterday, keepIntervals, &res); err != nil {
			return res, firstErr(liveErr, err)
		}
	}
	// Yesterday is whole only if its minute series arrived; otherwise it is
	// still owed and comes from the daily series once it is no longer yesterday.
	through := yesterday
	if liveErr != nil {
		through = yesterday.AddDate(0, 0, -1)
	}
	if plant.IsolarHistoryThrough == nil || dayFromDate(*plant.IsolarHistoryThrough).Before(through) {
		if err := s.d.Plants.SetHistoryThrough(ctx, sc, plant.ID, through); err != nil {
			return res, firstErr(liveErr, err)
		}
	}
	return res, liveErr
}

// syncLive is the part of a sync that only the present can answer: devices,
// realtime snapshots, and yesterday's and today's minute series.
func (s *Service) syncLive(ctx context.Context, sc store.Scope, creds integration.Credentials, plant model.PowerPlant, psID string, today, now time.Time, res *SyncResult) error {
	devices, err := s.syncDevices(ctx, sc, creds, plant, res)
	if err != nil {
		return err
	}
	if err := s.syncSnapshots(ctx, sc, creds, devices, now, res); err != nil {
		return err
	}
	for _, day := range []time.Time{today.AddDate(0, 0, -1), today} {
		to := day.AddDate(0, 0, 1)
		if to.After(now) {
			to = now
		}
		if !day.Before(to) {
			continue
		}
		if err := s.syncMinuteDay(ctx, sc, creds, plant.ID, psID, devices, day, to, res); err != nil {
			return err
		}
	}
	return nil
}

// historyFrom is the first day still owed (R506): the day after the history
// mark, or the whole link backfill when there is none; an explicit request for
// more days reaches further back.
func historyFrom(through *time.Time, today time.Time, backfillDays int) time.Time {
	from := today.AddDate(0, 0, -linkBackfillDays)
	if through != nil {
		from = dayFromDate(*through).AddDate(0, 0, 1)
	}
	if asked := today.AddDate(0, 0, -backfillDays); backfillDays > 0 && asked.Before(from) {
		from = asked
	}
	return from
}

// dayFromDate is the Istanbul midnight of a stored date.
func dayFromDate(d time.Time) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, istanbul)
}

// isFinal reports a failure retrying cannot heal (auth, a missing credential).
func isFinal(err error) bool {
	return err != nil && !classify(err).retry
}

func firstErr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}

// syncDevices upserts the provider's device list and returns the stored devices.
func (s *Service) syncDevices(ctx context.Context, sc store.Scope, creds integration.Credentials, plant model.PowerPlant, res *SyncResult) ([]model.PlantDevice, error) {
	list, err := s.d.ISolar.Devices(ctx, creds, *plant.IsolarPsID)
	if err != nil {
		return nil, err
	}
	for _, d := range list {
		key := d.PSKey
		if _, err := s.d.Plants.UpsertDevice(ctx, sc, model.PlantDevice{PlantID: plant.ID, DeviceSN: d.DeviceSN, DeviceName: d.DeviceName,
			DeviceType: d.DeviceType, ProviderKey: &key}); err != nil {
			return nil, err
		}
		res.DevicesUpserted++
	}
	return s.d.Plants.Devices(ctx, sc, plant.ID)
}

func isInverter(d model.PlantDevice) bool {
	return d.DeviceType != nil && (*d.DeviceType == isolar.DeviceTypeInverter || *d.DeviceType == isolar.DeviceTypeStorageInverter)
}

func inverterKeys(devices []model.PlantDevice) map[int32][]string {
	out := map[int32][]string{}
	for _, d := range devices {
		if isInverter(d) && d.ProviderKey != nil {
			out[*d.DeviceType] = append(out[*d.DeviceType], *d.ProviderKey)
		}
	}
	return out
}

// syncSnapshots stores each inverter's realtime reading (R282).
func (s *Service) syncSnapshots(ctx context.Context, sc store.Scope, creds integration.Credentials, devices []model.PlantDevice, now time.Time, res *SyncResult) error {
	byKey := map[string]model.PlantDevice{}
	for _, d := range devices {
		if d.ProviderKey != nil {
			byKey[*d.ProviderKey] = d
		}
	}
	for typ, keys := range inverterKeys(devices) {
		snaps, err := s.d.ISolar.DeviceRealtime(ctx, creds, typ, keys)
		if err != nil {
			return err
		}
		for _, snap := range snaps {
			d, ok := byKey[snap.PSKey]
			if !ok {
				continue
			}
			at := snap.At
			d.SnapshotAt, d.FaultStatus = &at, snap.FaultStatus
			d.ActivePowerKw, d.YieldTodayKwh, d.YieldMonthKwh = snap.ActivePowerKw, snap.YieldTodayKwh, snap.YieldMonthKwh
			d.YieldYearKwh, d.YieldTotalKwh = snap.YieldYearKwh, snap.YieldTotalKwh
			d.Status = StatusOf(snap.FaultStatus, &at, now)
			if err := s.d.Plants.UpdateDeviceSnapshot(ctx, sc, d); err != nil {
				return err
			}
			res.Snapshots++
		}
	}
	return nil
}

// syncMinuteDay replaces one Istanbul day from the minute series (R278, R279).
// An empty answer leaves the stored day alone: silence is not zero production.
func (s *Service) syncMinuteDay(ctx context.Context, sc store.Scope, creds integration.Credentials, plantID uuid.UUID, psID string,
	devices []model.PlantDevice, day, to time.Time, res *SyncResult) error {
	idByKey := map[string]uuid.UUID{}
	for _, d := range devices {
		if d.ProviderKey != nil {
			idByKey[*d.ProviderKey] = d.ID
		}
	}
	var keys []string
	for _, ks := range inverterKeys(devices) {
		keys = append(keys, ks...)
	}
	samples, err := s.d.ISolar.DeviceMinuteSeries(ctx, creds, keys, day, to)
	if err != nil {
		return err
	}
	byDevice := map[string][]isolar.YieldSample{}
	for _, smp := range samples {
		if smp.PSKey != nil {
			byDevice[*smp.PSKey] = append(byDevice[*smp.PSKey], smp)
		}
	}
	var deviceRows []model.PlantProduction
	var deviceSeries [][]Interval
	for key, series := range byDevice {
		id, ok := idByKey[key]
		if !ok {
			continue
		}
		ivs, resets := Intervals(series)
		res.Resets += resets
		deviceSeries = append(deviceSeries, ivs)
		for _, iv := range ivs {
			if iv.Kwh == nil && iv.PowerKw == nil {
				continue
			}
			deviceRows = append(deviceRows, model.PlantProduction{PlantID: plantID, DeviceID: id, Ts: iv.Ts, ProductionKwh: iv.Kwh,
				ActivePowerKw: iv.PowerKw, Source: "isolar"})
		}
	}
	if len(deviceRows) > 0 {
		n, err := s.d.Production.ReplaceDeviceDays(ctx, sc, plantID, []time.Time{day}, deviceRows)
		if err != nil {
			return err
		}
		res.DeviceRows += n
	}

	plantSamples, err := s.d.ISolar.PlantMinuteSeries(ctx, creds, psID, day, to)
	if err != nil {
		return err
	}
	basis := model.BasisPlantMeter
	intervals, resets := Intervals(plantSamples)
	res.Resets += resets
	inverterSum := SumByTimestamp(deviceSeries...)
	if len(plantSamples) == 0 {
		basis, intervals = model.BasisInverterSum, inverterSum
	} else {
		FillPower(intervals, inverterSum)
	}
	var rows []model.PlantProductionTotal
	for _, iv := range intervals {
		if iv.Kwh == nil && iv.PowerKw == nil {
			continue
		}
		rows = append(rows, model.PlantProductionTotal{PlantID: plantID, Ts: iv.Ts, ProductionKwh: iv.Kwh, ActivePowerKw: iv.PowerKw, Basis: basis})
	}
	if len(rows) == 0 {
		return nil
	}
	n, err := s.d.Totals.ReplaceDays(ctx, sc, plantID, []time.Time{day}, rows)
	if err != nil {
		return err
	}
	res.TotalRows += n
	res.Basis[basis] += n
	return nil
}

// backfill writes end-of-day totals for [from, to) in ≤31-day chunks (R281);
// only days the provider answered for are replaced, and with keepIntervals
// never a day that already holds interval rows.
func (s *Service) backfill(ctx context.Context, sc store.Scope, creds integration.Credentials, plantID uuid.UUID, psID string, from, to time.Time, keepIntervals bool, res *SyncResult) error {
	for start := from; start.Before(to); start = start.AddDate(0, 0, dailyChunk) {
		end := start.AddDate(0, 0, dailyChunk)
		if end.After(to) {
			end = to
		}
		days, err := s.d.ISolar.PlantDailySeries(ctx, creds, psID, start, end)
		if err != nil {
			return err
		}
		kept := map[time.Time]bool{}
		if keepIntervals && len(days) > 0 {
			stored, err := s.d.Totals.Range(ctx, sc, plantID, store.TimeRange{From: start, To: end})
			if err != nil {
				return fmt.Errorf("backfill: %w", err)
			}
			for _, r := range stored {
				if r.Basis != model.BasisDailyTotal {
					kept[dayOf(r.Ts)] = true
				}
			}
		}
		var rows []model.PlantProductionTotal
		var replaced []time.Time
		for _, d := range days {
			day := dayOf(d.Day)
			if d.Kwh == nil || kept[day] {
				continue
			}
			rows = append(rows, model.PlantProductionTotal{PlantID: plantID, Ts: day, ProductionKwh: d.Kwh, Basis: model.BasisDailyTotal})
			replaced = append(replaced, day)
		}
		if len(rows) == 0 {
			continue
		}
		n, err := s.d.Totals.ReplaceDays(ctx, sc, plantID, replaced, rows)
		if err != nil {
			return fmt.Errorf("backfill: %w", err)
		}
		res.TotalRows += n
		res.Basis[model.BasisDailyTotal] += n
	}
	return nil
}

// DispatchSync enqueues a sync for every linked plant (R288); a plant whose
// previous sync is still queued or running keeps it. A finally failed sync
// is archived under the same id, so the inspector is required to replace it.
func (s *Service) DispatchSync(ctx context.Context) (int, error) {
	if s.d.Inspector == nil {
		return 0, errors.New("solar: dispatch needs a task inspector to replace archived syncs")
	}
	plants, err := s.d.AdminSolar.LinkedPlants(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, p := range plants {
		payload := job.SolarSyncPayload{CompanyID: p.CompanyID, PlantID: p.PlantID}
		task, err := job.NewSolarSyncTask(payload, job.TaskOptions{MaxRetry: s.d.MaxRetry})
		if err != nil {
			return n, err
		}
		if err := job.EnqueueReplacingFinished(ctx, s.d.Enqueuer, s.d.Inspector, task, job.SolarSyncTaskID(payload)); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
