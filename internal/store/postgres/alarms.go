package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// alarmDefaultPageLimit and alarmMaxPageLimit are this file's Page defaults.
const (
	alarmDefaultPageLimit = 50
	alarmMaxPageLimit     = 500
)

func alarmPageLimits(p store.Page) (limit, offset int32) {
	limit = p.Limit
	if limit <= 0 {
		limit = alarmDefaultPageLimit
	}
	if limit > alarmMaxPageLimit {
		limit = alarmMaxPageLimit
	}
	offset = p.Offset
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// AlarmRepository implements store.AlarmRepository.
//
// alarms carries only company_id — no building_id — so a Scope narrows it
// to the company and no further, exactly as PlantRepository narrows
// power_plants. The attachments (alarm_analyzers, alarm_events by
// AnalyzerID) still enforce the Scope's building branch, because the
// analyzers they name are themselves building-scoped.
type AlarmRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewAlarmRepository builds an AlarmRepository over pool.
func NewAlarmRepository(pool *pgxpool.Pool) *AlarmRepository {
	return &AlarmRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.AlarmRepository = (*AlarmRepository)(nil)

// Get returns one alarm by id, scoped to the company.
func (r *AlarmRepository) Get(ctx context.Context, s store.Scope, id uuid.UUID) (model.Alarm, error) {
	if !s.Valid() {
		return model.Alarm{}, store.ErrInvalidScope
	}
	row, err := r.q.AlarmGet(ctx, sqlcgen.AlarmGetParams{ID: id, CompanyID: s.CompanyID})
	if err != nil {
		return model.Alarm{}, pgerr.Translate(r.pool, "get alarm", err)
	}
	return alarmFromRow(row)
}

// List returns the company's alarms matching f.
func (r *AlarmRepository) List(ctx context.Context, s store.Scope, f store.AlarmFilter) ([]model.Alarm, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	limit, offset := alarmPageLimits(f.Page)
	ids := f.IDs
	if ids == nil {
		ids = []uuid.UUID{}
	}
	types := make([]string, 0, len(f.Types))
	for _, t := range f.Types {
		types = append(types, string(t))
	}
	filterAnalyzer := f.AnalyzerID != nil
	var analyzerID uuid.UUID
	if filterAnalyzer {
		analyzerID = *f.AnalyzerID
	}
	buildingIDs, all := s.BuildingFilter()
	rows, err := r.q.AlarmList(ctx, sqlcgen.AlarmListParams{
		CompanyID: s.CompanyID, Ids: ids, Types: types, IsEnabled: f.IsEnabled,
		FilterAnalyzer: filterAnalyzer, AnalyzerID: analyzerID, AllBuildings: all, BuildingIds: buildingIDs,
		IncludeDeleted: f.IncludeDeleted, LimitVal: limit, OffsetVal: offset,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list alarms", err)
	}
	out := make([]model.Alarm, 0, len(rows))
	for _, row := range rows {
		a, err := alarmFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// Create inserts an alarm rule for the Scope's company.
func (r *AlarmRepository) Create(ctx context.Context, s store.Scope, a model.Alarm) (model.Alarm, error) {
	if !s.Valid() {
		return model.Alarm{}, store.ErrInvalidScope
	}
	if a.CompanyID != uuid.Nil && a.CompanyID != s.CompanyID {
		return model.Alarm{}, store.ErrNotFound
	}
	row, err := r.q.AlarmCreate(ctx, sqlcgen.AlarmCreateParams{
		CompanyID: s.CompanyID, Name: a.Name, Type: sqlcgen.AlarmType(a.Type), IsEnabled: a.IsEnabled,
		InductiveRatioThreshold: decimalPtrToNumeric(a.InductiveRatioThreshold), InductivePeriodValue: a.InductivePeriodValue,
		InductivePeriodUnit:      alarmNullPeriodUnit(a.InductivePeriodUnit),
		CapacitiveRatioThreshold: decimalPtrToNumeric(a.CapacitiveRatioThreshold), CapacitivePeriodValue: a.CapacitivePeriodValue,
		CapacitivePeriodUnit: alarmNullPeriodUnit(a.CapacitivePeriodUnit),
		ActiveConsumptionMax: decimalPtrToNumeric(a.ActiveConsumptionMax), ActiveConsumptionMaxPeriodValue: a.ActiveConsumptionMaxPeriodValue,
		ActiveConsumptionMaxPeriodUnit: alarmNullPeriodUnit(a.ActiveConsumptionMaxPeriodUnit),
		ActiveConsumptionMin:           decimalPtrToNumeric(a.ActiveConsumptionMin), ActiveConsumptionMinPeriodValue: a.ActiveConsumptionMinPeriodValue,
		ActiveConsumptionMinPeriodUnit: alarmNullPeriodUnit(a.ActiveConsumptionMinPeriodUnit),
		CommunicationThresholdHours:    a.CommunicationThresholdHours,
		VoltageMax:                     decimalPtrToNumeric(a.VoltageMax), VoltageMin: decimalPtrToNumeric(a.VoltageMin),
		PowerMax: decimalPtrToNumeric(a.PowerMax), PowerMin: decimalPtrToNumeric(a.PowerMin),
		InvoiceThresholdPct:        decimalPtrToNumeric(a.InvoiceThresholdPct),
		NotificationFrequencyValue: a.NotificationFrequencyValue, NotificationFrequencyUnit: alarmNullPeriodUnit(a.NotificationFrequencyUnit),
		CreatedAt: tariffTimestamptz(a.CreatedAt),
	})
	if err != nil {
		return model.Alarm{}, pgerr.Translate(r.pool, "create alarm", err)
	}
	return alarmFromRow(row)
}

// Update fully replaces an alarm's fields by id.
func (r *AlarmRepository) Update(ctx context.Context, s store.Scope, a model.Alarm) (model.Alarm, error) {
	if !s.Valid() {
		return model.Alarm{}, store.ErrInvalidScope
	}
	// Important Finding 3: same CompanyID check as Create's.
	if a.CompanyID != uuid.Nil && a.CompanyID != s.CompanyID {
		return model.Alarm{}, store.ErrNotFound
	}
	row, err := r.q.AlarmUpdate(ctx, sqlcgen.AlarmUpdateParams{
		ID: a.ID, CompanyID: s.CompanyID, Name: a.Name, Type: sqlcgen.AlarmType(a.Type), IsEnabled: a.IsEnabled,
		InductiveRatioThreshold: decimalPtrToNumeric(a.InductiveRatioThreshold), InductivePeriodValue: a.InductivePeriodValue,
		InductivePeriodUnit:      alarmNullPeriodUnit(a.InductivePeriodUnit),
		CapacitiveRatioThreshold: decimalPtrToNumeric(a.CapacitiveRatioThreshold), CapacitivePeriodValue: a.CapacitivePeriodValue,
		CapacitivePeriodUnit: alarmNullPeriodUnit(a.CapacitivePeriodUnit),
		ActiveConsumptionMax: decimalPtrToNumeric(a.ActiveConsumptionMax), ActiveConsumptionMaxPeriodValue: a.ActiveConsumptionMaxPeriodValue,
		ActiveConsumptionMaxPeriodUnit: alarmNullPeriodUnit(a.ActiveConsumptionMaxPeriodUnit),
		ActiveConsumptionMin:           decimalPtrToNumeric(a.ActiveConsumptionMin), ActiveConsumptionMinPeriodValue: a.ActiveConsumptionMinPeriodValue,
		ActiveConsumptionMinPeriodUnit: alarmNullPeriodUnit(a.ActiveConsumptionMinPeriodUnit),
		CommunicationThresholdHours:    a.CommunicationThresholdHours,
		VoltageMax:                     decimalPtrToNumeric(a.VoltageMax), VoltageMin: decimalPtrToNumeric(a.VoltageMin),
		PowerMax: decimalPtrToNumeric(a.PowerMax), PowerMin: decimalPtrToNumeric(a.PowerMin),
		InvoiceThresholdPct:        decimalPtrToNumeric(a.InvoiceThresholdPct),
		NotificationFrequencyValue: a.NotificationFrequencyValue, NotificationFrequencyUnit: alarmNullPeriodUnit(a.NotificationFrequencyUnit),
		UpdatedAt: tariffTimestamptz(a.UpdatedAt),
	})
	if err != nil {
		return model.Alarm{}, pgerr.Translate(r.pool, "update alarm", err)
	}
	return alarmFromRow(row)
}

// SoftDelete stamps deleted_at on an alarm by id.
func (r *AlarmRepository) SoftDelete(ctx context.Context, s store.Scope, id uuid.UUID, at time.Time) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	n, err := r.q.AlarmSoftDelete(ctx, sqlcgen.AlarmSoftDeleteParams{DeletedAt: tariffTimestamptz(at), ID: id, CompanyID: s.CompanyID})
	if err != nil {
		return pgerr.Translate(r.pool, "soft delete alarm", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// Analyzers — Isolation: alarm_analyzers has no company_id — join through
// alarms.
//
// Important Finding 2: alarms itself is company-wide, but a narrow Scope
// must still never see an attachment to an analyzer outside its buildings —
// AlarmAnalyzerList filters on the analyzer's own visibility, not only the
// alarm's.
func (r *AlarmRepository) Analyzers(ctx context.Context, s store.Scope, alarmID uuid.UUID) ([]model.AlarmAnalyzer, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if err := r.requireVisible(ctx, s, alarmID); err != nil {
		return nil, err
	}
	ids, all := s.BuildingFilter()
	rows, err := r.q.AlarmAnalyzerList(ctx, sqlcgen.AlarmAnalyzerListParams{
		AlarmID: alarmID, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list alarm analyzers", err)
	}
	out := make([]model.AlarmAnalyzer, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.AlarmAnalyzer{AlarmID: row.AlarmID, AnalyzerID: row.AnalyzerID})
	}
	return out, nil
}

// ReplaceAnalyzers — Isolation: join alarm_analyzers through alarms; every
// analyzerID must also be visible to the Scope, or the whole call is
// refused with ErrNotFound.
//
// Important Finding 2: this replaces ONLY the attachments visible to the
// Scope (AlarmAnalyzerDeleteVisibleForAlarm) and leaves an attachment to an
// analyzer outside it untouched — a narrow Scope replacing "its" list must
// not be able to silently detach Buildings[1]'s analyzer.
func (r *AlarmRepository) ReplaceAnalyzers(ctx context.Context, s store.Scope, alarmID uuid.UUID, analyzerIDs []uuid.UUID) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	if err := r.requireVisible(ctx, s, alarmID); err != nil {
		return err
	}
	buildingIDs, all := s.BuildingFilter()
	if len(analyzerIDs) > 0 {
		count, err := r.q.AlarmCountVisibleAnalyzers(ctx, sqlcgen.AlarmCountVisibleAnalyzersParams{
			CompanyID: s.CompanyID, AnalyzerIds: analyzerIDs, AllBuildings: all, BuildingIds: buildingIDs,
		})
		if err != nil {
			return pgerr.Translate(r.pool, "check alarm analyzer visibility", err)
		}
		if count != int64(len(alarmDistinctUUIDs(analyzerIDs))) {
			return store.ErrNotFound
		}
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return pgerr.Translate(r.pool, "begin replace alarm analyzers", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	if err := q.AlarmAnalyzerDeleteVisibleForAlarm(ctx, sqlcgen.AlarmAnalyzerDeleteVisibleForAlarmParams{
		AlarmID: alarmID, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: buildingIDs,
	}); err != nil {
		return pgerr.Translate(r.pool, "clear alarm analyzers", err)
	}
	for _, analyzerID := range analyzerIDs {
		if _, err := q.AlarmAnalyzerInsert(ctx, sqlcgen.AlarmAnalyzerInsertParams{
			AlarmID: alarmID, AnalyzerID: analyzerID, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: buildingIDs,
		}); err != nil {
			return pgerr.Translate(r.pool, "insert alarm analyzer", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return pgerr.Translate(r.pool, "commit replace alarm analyzers", err)
	}
	return nil
}

// Channels — Isolation: alarm_channels has no company_id — join through
// alarms.
func (r *AlarmRepository) Channels(ctx context.Context, s store.Scope, alarmID uuid.UUID) ([]model.AlarmChannel, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if err := r.requireVisible(ctx, s, alarmID); err != nil {
		return nil, err
	}
	rows, err := r.q.AlarmChannelList(ctx, sqlcgen.AlarmChannelListParams{AlarmID: alarmID, CompanyID: s.CompanyID})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list alarm channels", err)
	}
	out := make([]model.AlarmChannel, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.AlarmChannel{AlarmID: row.AlarmID, Channel: model.NotifyChannel(row.Channel), Target: row.Target})
	}
	return out, nil
}

// ReplaceChannels — Isolation: join alarm_channels through alarms.
func (r *AlarmRepository) ReplaceChannels(ctx context.Context, s store.Scope, alarmID uuid.UUID, channels []model.AlarmChannel) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	if err := r.requireVisible(ctx, s, alarmID); err != nil {
		return err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return pgerr.Translate(r.pool, "begin replace alarm channels", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	if err := q.AlarmChannelDeleteForAlarm(ctx, sqlcgen.AlarmChannelDeleteForAlarmParams{AlarmID: alarmID, CompanyID: s.CompanyID}); err != nil {
		return pgerr.Translate(r.pool, "clear alarm channels", err)
	}
	for _, ch := range channels {
		if _, err := q.AlarmChannelInsert(ctx, sqlcgen.AlarmChannelInsertParams{
			AlarmID: alarmID, Channel: sqlcgen.NotifyChannel(ch.Channel), Target: ch.Target, CompanyID: s.CompanyID,
		}); err != nil {
			return pgerr.Translate(r.pool, "insert alarm channel", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return pgerr.Translate(r.pool, "commit replace alarm channels", err)
	}
	return nil
}

// CreateEvent — Isolation: alarm_events has no company_id — join through
// alarms on e.AlarmID; e.AnalyzerID, if set, must also be visible.
func (r *AlarmRepository) CreateEvent(ctx context.Context, s store.Scope, e model.AlarmEvent) (model.AlarmEvent, error) {
	if !s.Valid() {
		return model.AlarmEvent{}, store.ErrInvalidScope
	}
	if err := r.requireVisible(ctx, s, e.AlarmID); err != nil {
		return model.AlarmEvent{}, err
	}
	buildingIDs, all := s.BuildingFilter()
	if e.AnalyzerID != nil {
		count, err := r.q.AlarmCountVisibleAnalyzers(ctx, sqlcgen.AlarmCountVisibleAnalyzersParams{
			CompanyID: s.CompanyID, AnalyzerIds: []uuid.UUID{*e.AnalyzerID}, AllBuildings: all, BuildingIds: buildingIDs,
		})
		if err != nil {
			return model.AlarmEvent{}, pgerr.Translate(r.pool, "check alarm event analyzer visibility", err)
		}
		if count == 0 {
			return model.AlarmEvent{}, store.ErrNotFound
		}
	}
	row, err := r.q.AlarmEventCreate(ctx, sqlcgen.AlarmEventCreateParams{
		AlarmID: e.AlarmID, AnalyzerID: e.AnalyzerID, TriggeredAt: tariffTimestamptz(e.TriggeredAt),
		Message: e.Message, Detail: []byte(e.Detail), NotifiedAt: tariffNullableTimestamptz(e.NotifiedAt),
		NotificationError: e.NotificationError, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: buildingIDs,
	})
	if err != nil {
		return model.AlarmEvent{}, pgerr.Translate(r.pool, "create alarm event", err)
	}
	return alarmEventFromRow(row), nil
}

// ListEvents — Isolation: join alarm_events through alarms; events of
// alarms not visible to the Scope are never listed.
//
// Important Finding 2: an event's own analyzer must ALSO be visible to the
// Scope, and an event with no analyzer at all (a company-level condition) is
// visible only to a Scope with AllBuildings — otherwise a narrow Scope could
// read another building's analyzer id and the event's message/detail through
// this method even though Analyzers() and Get would both refuse it.
func (r *AlarmRepository) ListEvents(ctx context.Context, s store.Scope, f store.AlarmEventFilter) ([]model.AlarmEvent, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if f.Range != nil && !f.Range.Valid() {
		return nil, store.ErrInvalidRange
	}
	limit, offset := alarmPageLimits(f.Page)
	var from, to pgtype.Timestamptz
	if f.Range != nil {
		from, to = tariffTimestamptz(f.Range.From), tariffTimestamptz(f.Range.To)
	}
	buildingIDs, all := s.BuildingFilter()
	rows, err := r.q.AlarmEventList(ctx, sqlcgen.AlarmEventListParams{
		CompanyID: s.CompanyID, AlarmID: f.AlarmID, AnalyzerID: f.AnalyzerID, Undelivered: f.Undelivered,
		Notified:  f.Notified,
		RangeFrom: from, RangeTo: to, AllBuildings: all, BuildingIds: buildingIDs, LimitVal: limit, OffsetVal: offset,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list alarm events", err)
	}
	out := make([]model.AlarmEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, alarmEventFromRow(row))
	}
	return out, nil
}

// MarkNotified — Isolation: join alarm_events through alarms by eventID.
//
// F1 final review pass A, Important Finding 3 (fix round): this used to
// check only alarms.company_id, ignoring both the alarm's soft delete and
// the event's own building/analyzer visibility — a narrow Scope could mark
// an event it could never list (ListEvents) as notified. AlarmMarkNotified
// now applies ListEvents' exact visibility rule, so all_buildings and
// building_ids are passed here too.
func (r *AlarmRepository) MarkNotified(ctx context.Context, s store.Scope, eventID uuid.UUID, at time.Time, notificationError *string) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	buildingIDs, all := s.BuildingFilter()
	// A ZERO at stores NULL, not year 1: model.AlarmEvent's contract is that a
	// nil NotifiedAt with a non-nil NotificationError means "fired and nobody
	// was told". Stamping the zero time made that state unreachable — every
	// failed delivery read back as delivered, which an e2e run caught.
	notifiedAt := tariffTimestamptz(at)
	if at.IsZero() {
		notifiedAt = pgtype.Timestamptz{}
	}
	n, err := r.q.AlarmMarkNotified(ctx, sqlcgen.AlarmMarkNotifiedParams{
		NotifiedAt: notifiedAt, NotificationError: notificationError, ID: eventID, CompanyID: s.CompanyID,
		AllBuildings: all, BuildingIds: buildingIDs,
	})
	if err != nil {
		return pgerr.Translate(r.pool, "mark alarm event notified", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// MarkBillFired — Isolation: alarm_fired_bills has no company_id — join
// through BOTH alarms and bills.
func (r *AlarmRepository) MarkBillFired(ctx context.Context, s store.Scope, alarmID, billID uuid.UUID) (bool, error) {
	if !s.Valid() {
		return false, store.ErrInvalidScope
	}
	if err := r.requireVisible(ctx, s, alarmID); err != nil {
		return false, err
	}
	ids, all := s.BuildingFilter()
	billVisible, err := r.q.BillVisible(ctx, sqlcgen.BillVisibleParams{ID: billID, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids})
	if err != nil {
		return false, pgerr.Translate(r.pool, "check bill visibility for mark bill fired", err)
	}
	if !billVisible {
		return false, store.ErrNotFound
	}

	_, err = r.q.AlarmMarkBillFired(ctx, sqlcgen.AlarmMarkBillFiredParams{
		AlarmID: alarmID, BillID: billID, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, pgerr.Translate(r.pool, "mark bill fired", err)
	}
	return true, nil
}

// MarkIsolarForwarded — Isolation: isolar_forwarded_alarms has no
// company_id — join through power_plants.
func (r *AlarmRepository) MarkIsolarForwarded(ctx context.Context, s store.Scope, plantID uuid.UUID, alarmRef string, at time.Time) (bool, error) {
	if !s.Valid() {
		return false, store.ErrInvalidScope
	}
	visible, err := r.q.AlarmPlantVisible(ctx, sqlcgen.AlarmPlantVisibleParams{PlantID: plantID, CompanyID: s.CompanyID})
	if err != nil {
		return false, pgerr.Translate(r.pool, "check plant visibility for mark isolar forwarded", err)
	}
	if !visible {
		return false, store.ErrNotFound
	}

	_, err = r.q.AlarmMarkIsolarForwarded(ctx, sqlcgen.AlarmMarkIsolarForwardedParams{
		PlantID: plantID, AlarmRef: alarmRef, SentAt: tariffTimestamptz(at), CompanyID: s.CompanyID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, pgerr.Translate(r.pool, "mark isolar forwarded", err)
	}
	return true, nil
}

func (r *AlarmRepository) requireVisible(ctx context.Context, s store.Scope, alarmID uuid.UUID) error {
	visible, err := r.q.AlarmVisible(ctx, sqlcgen.AlarmVisibleParams{ID: alarmID, CompanyID: s.CompanyID})
	if err != nil {
		return pgerr.Translate(r.pool, "check alarm visibility", err)
	}
	if !visible {
		return store.ErrNotFound
	}
	return nil
}

// alarmDistinctUUIDs is used by ReplaceAnalyzers to compare a visibility count
// against the number of DISTINCT ids requested — a duplicate id in the input
// must not make the count look short by one.
func alarmDistinctUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func alarmNullPeriodUnit(u *model.PeriodUnit) sqlcgen.NullPeriodUnit {
	if u == nil {
		return sqlcgen.NullPeriodUnit{}
	}
	return sqlcgen.NullPeriodUnit{PeriodUnit: sqlcgen.PeriodUnit(*u), Valid: true}
}

func alarmPeriodUnitPtr(u sqlcgen.NullPeriodUnit) *model.PeriodUnit {
	if !u.Valid {
		return nil
	}
	v := model.PeriodUnit(u.PeriodUnit)
	return &v
}

func alarmFromRow(row sqlcgen.Alarm) (model.Alarm, error) {
	out := model.Alarm{
		ID: row.ID, CompanyID: row.CompanyID, Name: row.Name, Type: model.AlarmType(row.Type),
		IsEnabled:            row.IsEnabled,
		InductivePeriodValue: row.InductivePeriodValue, InductivePeriodUnit: alarmPeriodUnitPtr(row.InductivePeriodUnit),
		CapacitivePeriodValue: row.CapacitivePeriodValue, CapacitivePeriodUnit: alarmPeriodUnitPtr(row.CapacitivePeriodUnit),
		ActiveConsumptionMaxPeriodValue: row.ActiveConsumptionMaxPeriodValue,
		ActiveConsumptionMaxPeriodUnit:  alarmPeriodUnitPtr(row.ActiveConsumptionMaxPeriodUnit),
		ActiveConsumptionMinPeriodValue: row.ActiveConsumptionMinPeriodValue,
		ActiveConsumptionMinPeriodUnit:  alarmPeriodUnitPtr(row.ActiveConsumptionMinPeriodUnit),
		CommunicationThresholdHours:     row.CommunicationThresholdHours,
		NotificationFrequencyValue:      row.NotificationFrequencyValue,
		NotificationFrequencyUnit:       alarmPeriodUnitPtr(row.NotificationFrequencyUnit),
		CreatedAt:                       row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time, DeletedAt: tariffNullTimestamptz(row.DeletedAt),
	}
	for _, f := range []tariffNumericField{
		{"inductive_ratio_threshold", row.InductiveRatioThreshold, &out.InductiveRatioThreshold},
		{"capacitive_ratio_threshold", row.CapacitiveRatioThreshold, &out.CapacitiveRatioThreshold},
		{"active_consumption_max", row.ActiveConsumptionMax, &out.ActiveConsumptionMax},
		{"active_consumption_min", row.ActiveConsumptionMin, &out.ActiveConsumptionMin},
		{"voltage_max", row.VoltageMax, &out.VoltageMax},
		{"voltage_min", row.VoltageMin, &out.VoltageMin},
		{"power_max", row.PowerMax, &out.PowerMax},
		{"power_min", row.PowerMin, &out.PowerMin},
		{"invoice_threshold_pct", row.InvoiceThresholdPct, &out.InvoiceThresholdPct},
	} {
		d, err := numericToDecimalPtr(f.src)
		if err != nil {
			return model.Alarm{}, fmt.Errorf("alarms.%s: %w", f.name, err)
		}
		*f.dst = d
	}
	return out, nil
}

func alarmEventFromRow(row sqlcgen.AlarmEvent) model.AlarmEvent {
	return model.AlarmEvent{
		ID: row.ID, AlarmID: row.AlarmID, AnalyzerID: row.AnalyzerID, TriggeredAt: row.TriggeredAt.Time,
		Message: row.Message, Detail: row.Detail, NotifiedAt: tariffNullTimestamptz(row.NotifiedAt),
		NotificationError: row.NotificationError,
	}
}
