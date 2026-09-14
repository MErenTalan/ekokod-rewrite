package postgres

// This file implements store.PlantRepository — the "Buildings, analyzers and
// plants (migration 00003)" block of internal/store/repository.go.

import (
	"context"
	"errors"
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

const (
	plantPageDefaultLimit = 50
	plantPageMaxLimit     = 500
)

// PlantRepository is the postgres store.PlantRepository.
//
// power_plants has NO building_id, so every query below stops at
// company_id = s.CompanyID: see the doc comment on store.PlantRepository.
type PlantRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

var _ store.PlantRepository = (*PlantRepository)(nil)

// NewPlantRepository builds a PlantRepository over pool.
func NewPlantRepository(pool *pgxpool.Pool) *PlantRepository {
	return &PlantRepository{q: sqlcgen.New(pool), pool: pool}
}

func plantDate(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

func plantDatePtr(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	t := d.Time
	return &t
}

func plantOrientation(o *model.PanelOrientation) sqlcgen.NullPanelOrientation {
	if o == nil {
		return sqlcgen.NullPanelOrientation{}
	}
	return sqlcgen.NullPanelOrientation{PanelOrientation: sqlcgen.PanelOrientation(*o), Valid: true}
}

func plantOrientationPtr(n sqlcgen.NullPanelOrientation) *model.PanelOrientation {
	if !n.Valid {
		return nil
	}
	o := model.PanelOrientation(n.PanelOrientation)
	return &o
}

func plantFromRow(row sqlcgen.PowerPlant) (model.PowerPlant, error) {
	panelPowerW, err := numericToDecimalPtr(row.PanelPowerW)
	if err != nil {
		return model.PowerPlant{}, err
	}
	panelEfficiency, err := numericToDecimalPtr(row.PanelEfficiencyPct)
	if err != nil {
		return model.PowerPlant{}, err
	}
	tiltAngle, err := numericToDecimalPtr(row.TiltAngleDeg)
	if err != nil {
		return model.PowerPlant{}, err
	}
	totalCapacity, err := numericToDecimalPtr(row.TotalCapacityKw)
	if err != nil {
		return model.PowerPlant{}, err
	}
	yearlyTarget, err := numericToDecimalPtr(row.YearlyTargetKwh)
	if err != nil {
		return model.PowerPlant{}, err
	}
	latitude, err := numericToDecimalPtr(row.Latitude)
	if err != nil {
		return model.PowerPlant{}, err
	}
	longitude, err := numericToDecimalPtr(row.Longitude)
	if err != nil {
		return model.PowerPlant{}, err
	}
	isolarInstalledKw, err := numericToDecimalPtr(row.IsolarInstalledKw)
	if err != nil {
		return model.PowerPlant{}, err
	}
	return model.PowerPlant{
		ID:                 row.ID,
		CompanyID:          row.CompanyID,
		Name:               row.Name,
		InstallationNumber: row.InstallationNumber,
		PlantKind:          row.PlantKind,
		PvBrandModel:       row.PvBrandModel,
		PanelPowerW:        panelPowerW,
		PanelEfficiencyPct: panelEfficiency,
		PanelCount:         row.PanelCount,
		StringCount:        row.StringCount,
		Orientation:        plantOrientationPtr(row.Orientation),
		TiltAngleDeg:       tiltAngle,
		TotalCapacityKw:    totalCapacity,
		YearlyTargetKwh:    yearlyTarget,
		InstallationDate:   plantDatePtr(row.InstallationDate),
		Address:            row.Address,
		Latitude:           latitude,
		Longitude:          longitude,
		IsolarPsID:         row.IsolarPsID,
		IsolarPsKey:        row.IsolarPsKey,
		IsolarPsName:       row.IsolarPsName,
		IsolarInstalledKw:  isolarInstalledKw,
		IsolarLinkedAt:     tsPtr(row.IsolarLinkedAt),
		CreatedAt:          row.CreatedAt.Time,
		UpdatedAt:          row.UpdatedAt.Time,
		DeletedAt:          tsPtr(row.DeletedAt),
	}, nil
}

// Get implements store.PlantRepository.Get.
func (r *PlantRepository) Get(ctx context.Context, s store.Scope, id uuid.UUID) (model.PowerPlant, error) {
	if !s.Valid() {
		return model.PowerPlant{}, store.ErrInvalidScope
	}
	row, err := r.q.PlantGet(ctx, sqlcgen.PlantGetParams{ID: id, CompanyID: s.CompanyID})
	if err != nil {
		return model.PowerPlant{}, pgerr.Translate(r.pool, "get plant", err)
	}
	return plantFromRow(row)
}

// List implements store.PlantRepository.List.
func (r *PlantRepository) List(ctx context.Context, s store.Scope, f store.PlantFilter) ([]model.PowerPlant, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	rows, err := r.q.PlantList(ctx, sqlcgen.PlantListParams{
		CompanyID:      s.CompanyID,
		FilterIds:      uuidsOrEmpty(f.IDs),
		PlantKind:      f.PlantKind,
		IsolarLinked:   f.IsolarLinked,
		IncludeDeleted: f.IncludeDeleted,
		PageOffset:     f.Page.Offset,
		PageLimit:      pageLimit(f.Page, plantPageDefaultLimit, plantPageMaxLimit),
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list plants", err)
	}
	out := make([]model.PowerPlant, 0, len(rows))
	for _, row := range rows {
		p, err := plantFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// Create implements store.PlantRepository.Create.
func (r *PlantRepository) Create(ctx context.Context, s store.Scope, p model.PowerPlant) (model.PowerPlant, error) {
	if !s.Valid() {
		return model.PowerPlant{}, store.ErrInvalidScope
	}
	if p.CompanyID != s.CompanyID {
		return model.PowerPlant{}, store.ErrNotFound
	}
	id := p.ID
	if id == uuid.Nil {
		id = uuid.New()
	}
	row, err := r.q.PlantCreate(ctx, sqlcgen.PlantCreateParams{
		ID:                 id,
		CompanyID:          s.CompanyID,
		Name:               p.Name,
		InstallationNumber: p.InstallationNumber,
		PlantKind:          p.PlantKind,
		PvBrandModel:       p.PvBrandModel,
		PanelPowerW:        decimalPtrToNumeric(p.PanelPowerW),
		PanelEfficiencyPct: decimalPtrToNumeric(p.PanelEfficiencyPct),
		PanelCount:         p.PanelCount,
		StringCount:        p.StringCount,
		Orientation:        plantOrientation(p.Orientation),
		TiltAngleDeg:       decimalPtrToNumeric(p.TiltAngleDeg),
		TotalCapacityKw:    decimalPtrToNumeric(p.TotalCapacityKw),
		YearlyTargetKwh:    decimalPtrToNumeric(p.YearlyTargetKwh),
		InstallationDate:   plantDate(p.InstallationDate),
		Address:            p.Address,
		Latitude:           decimalPtrToNumeric(p.Latitude),
		Longitude:          decimalPtrToNumeric(p.Longitude),
		IsolarPsID:         p.IsolarPsID,
		IsolarPsKey:        p.IsolarPsKey,
		IsolarPsName:       p.IsolarPsName,
		IsolarInstalledKw:  decimalPtrToNumeric(p.IsolarInstalledKw),
		IsolarLinkedAt:     tsPtrOrZero(p.IsolarLinkedAt),
		At:                 ts(p.CreatedAt),
	})
	if err != nil {
		return model.PowerPlant{}, pgerr.Translate(r.pool, "create plant", err)
	}
	return plantFromRow(row)
}

// Update implements store.PlantRepository.Update.
func (r *PlantRepository) Update(ctx context.Context, s store.Scope, p model.PowerPlant) (model.PowerPlant, error) {
	if !s.Valid() {
		return model.PowerPlant{}, store.ErrInvalidScope
	}
	row, err := r.q.PlantUpdate(ctx, sqlcgen.PlantUpdateParams{
		Name:               p.Name,
		InstallationNumber: p.InstallationNumber,
		PlantKind:          p.PlantKind,
		PvBrandModel:       p.PvBrandModel,
		PanelPowerW:        decimalPtrToNumeric(p.PanelPowerW),
		PanelEfficiencyPct: decimalPtrToNumeric(p.PanelEfficiencyPct),
		PanelCount:         p.PanelCount,
		StringCount:        p.StringCount,
		Orientation:        plantOrientation(p.Orientation),
		TiltAngleDeg:       decimalPtrToNumeric(p.TiltAngleDeg),
		TotalCapacityKw:    decimalPtrToNumeric(p.TotalCapacityKw),
		YearlyTargetKwh:    decimalPtrToNumeric(p.YearlyTargetKwh),
		InstallationDate:   plantDate(p.InstallationDate),
		Address:            p.Address,
		Latitude:           decimalPtrToNumeric(p.Latitude),
		Longitude:          decimalPtrToNumeric(p.Longitude),
		IsolarPsID:         p.IsolarPsID,
		IsolarPsKey:        p.IsolarPsKey,
		IsolarPsName:       p.IsolarPsName,
		IsolarInstalledKw:  decimalPtrToNumeric(p.IsolarInstalledKw),
		IsolarLinkedAt:     tsPtrOrZero(p.IsolarLinkedAt),
		UpdatedAt:          ts(p.UpdatedAt),
		ID:                 p.ID,
		CompanyID:          s.CompanyID,
	})
	if err != nil {
		return model.PowerPlant{}, pgerr.Translate(r.pool, "update plant", err)
	}
	return plantFromRow(row)
}

// SoftDelete implements store.PlantRepository.SoftDelete.
func (r *PlantRepository) SoftDelete(ctx context.Context, s store.Scope, id uuid.UUID, at time.Time) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	n, err := r.q.PlantSoftDelete(ctx, sqlcgen.PlantSoftDeleteParams{DeletedAt: ts(at), ID: id, CompanyID: s.CompanyID})
	if err != nil {
		return pgerr.Translate(r.pool, "soft delete plant", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// visible reports whether plantID belongs to s.CompanyID and is live: the
// shared existence check every child collection method below needs, since a
// visible parent with no children must return empty rather than ErrNotFound.
func (r *PlantRepository) visible(ctx context.Context, s store.Scope, plantID uuid.UUID) (bool, error) {
	return r.q.PlantVisible(ctx, sqlcgen.PlantVisibleParams{ID: plantID, CompanyID: s.CompanyID})
}

// MonthlyTargets — Isolation: power_plant_monthly_targets has no company_id —
// join through power_plants. Another tenant's plantID returns ErrNotFound.
func (r *PlantRepository) MonthlyTargets(ctx context.Context, s store.Scope, plantID uuid.UUID) ([]model.PlantMonthlyTarget, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	visible, err := r.visible(ctx, s, plantID)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "check plant visibility", err)
	}
	if !visible {
		return nil, store.ErrNotFound
	}
	rows, err := r.q.PlantMonthlyTargetsList(ctx, sqlcgen.PlantMonthlyTargetsListParams{PlantID: plantID, CompanyID: s.CompanyID})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list plant monthly targets", err)
	}
	out := make([]model.PlantMonthlyTarget, 0, len(rows))
	for _, row := range rows {
		target, err := numericToDecimal(row.TargetKwh)
		if err != nil {
			return nil, err
		}
		out = append(out, model.PlantMonthlyTarget{PlantID: row.PlantID, Month: row.Month, TargetKwh: target})
	}
	return out, nil
}

// ReplaceMonthlyTargets — Isolation: join power_plant_monthly_targets through
// power_plants. Another tenant's plantID returns ErrNotFound and nothing is
// replaced.
func (r *PlantRepository) ReplaceMonthlyTargets(ctx context.Context, s store.Scope, plantID uuid.UUID, targets []model.PlantMonthlyTarget) ([]model.PlantMonthlyTarget, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	visible, err := r.visible(ctx, s, plantID)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "check plant visibility", err)
	}
	if !visible {
		return nil, store.ErrNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "begin replace monthly targets", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.q.WithTx(tx)
	if err := qtx.PlantMonthlyTargetsDelete(ctx, plantID); err != nil {
		return nil, pgerr.Translate(r.pool, "delete plant monthly targets", err)
	}
	out := make([]model.PlantMonthlyTarget, 0, len(targets))
	for _, t := range targets {
		row, err := qtx.PlantMonthlyTargetInsert(ctx, sqlcgen.PlantMonthlyTargetInsertParams{
			PlantID: plantID, Month: t.Month, TargetKwh: decimalToNumeric(t.TargetKwh),
		})
		if err != nil {
			return nil, pgerr.Translate(r.pool, "insert plant monthly target", err)
		}
		target, err := numericToDecimal(row.TargetKwh)
		if err != nil {
			return nil, err
		}
		out = append(out, model.PlantMonthlyTarget{PlantID: row.PlantID, Month: row.Month, TargetKwh: target})
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, pgerr.Translate(r.pool, "commit replace monthly targets", err)
	}
	return out, nil
}

// Devices — Isolation: power_plant_devices has no company_id — join through
// power_plants. Another tenant's plantID returns ErrNotFound.
func (r *PlantRepository) Devices(ctx context.Context, s store.Scope, plantID uuid.UUID) ([]model.PlantDevice, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	visible, err := r.visible(ctx, s, plantID)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "check plant visibility", err)
	}
	if !visible {
		return nil, store.ErrNotFound
	}
	rows, err := r.q.PlantDevicesList(ctx, sqlcgen.PlantDevicesListParams{PlantID: plantID, CompanyID: s.CompanyID})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list plant devices", err)
	}
	out := make([]model.PlantDevice, 0, len(rows))
	for _, row := range rows {
		d, err := plantDeviceFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func plantDeviceFromRow(row sqlcgen.PowerPlantDevice) (model.PlantDevice, error) {
	ratedPower, err := numericToDecimalPtr(row.RatedPowerKw)
	if err != nil {
		return model.PlantDevice{}, err
	}
	efficiency, err := numericToDecimalPtr(row.EfficiencyPct)
	if err != nil {
		return model.PlantDevice{}, err
	}
	return model.PlantDevice{
		ID:             row.ID,
		PlantID:        row.PlantID,
		DeviceSN:       row.DeviceSn,
		DeviceName:     row.DeviceName,
		DeviceType:     row.DeviceType,
		DeviceTypeName: row.DeviceTypeName,
		ProviderKey:    row.ProviderKey,
		Brand:          row.Brand,
		Model:          row.Model,
		RatedPowerKw:   ratedPower,
		Status:         row.Status,
		EfficiencyPct:  efficiency,
		LastSeenAt:     tsPtr(row.LastSeenAt),
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}, nil
}

// UpsertDevice is keyed on (plant_id, device_sn). Isolation: join
// power_plant_devices through power_plants on d.PlantID. A d.PlantID of
// another tenant is refused with ErrNotFound. The update branch never
// changes plant_id, so a device can be neither moved to nor taken from
// another plant.
//
// Implemented as UPDATE-then-INSERT-if-absent rather than a single `insert
// ... on conflict (plant_id, device_sn) do update`: see the comment on
// PlantDeviceUpdateBySerial in queries/plants.sql for why the atomic form
// is unavailable here, and for the resulting race this trades away (two
// concurrent UpsertDevice calls racing to create the SAME new device can
// both attempt the insert; the loser gets store.ErrConflict rather than
// silently converging).
func (r *PlantRepository) UpsertDevice(ctx context.Context, s store.Scope, d model.PlantDevice) (model.PlantDevice, error) {
	if !s.Valid() {
		return model.PlantDevice{}, store.ErrInvalidScope
	}
	visible, err := r.visible(ctx, s, d.PlantID)
	if err != nil {
		return model.PlantDevice{}, pgerr.Translate(r.pool, "check plant visibility", err)
	}
	if !visible {
		return model.PlantDevice{}, store.ErrNotFound
	}

	updated, err := r.q.PlantDeviceUpdateBySerial(ctx, sqlcgen.PlantDeviceUpdateBySerialParams{
		DeviceName:     d.DeviceName,
		DeviceType:     d.DeviceType,
		DeviceTypeName: d.DeviceTypeName,
		ProviderKey:    d.ProviderKey,
		Brand:          d.Brand,
		Model:          d.Model,
		RatedPowerKw:   decimalPtrToNumeric(d.RatedPowerKw),
		Status:         d.Status,
		EfficiencyPct:  decimalPtrToNumeric(d.EfficiencyPct),
		LastSeenAt:     tsPtrOrZero(d.LastSeenAt),
		At:             ts(d.UpdatedAt),
		PlantID:        d.PlantID,
		DeviceSn:       d.DeviceSN,
	})
	switch {
	case err == nil:
		return plantDeviceFromRow(updated)
	case !errors.Is(err, pgx.ErrNoRows):
		return model.PlantDevice{}, pgerr.Translate(r.pool, "update plant device", err)
	}

	id := d.ID
	if id == uuid.Nil {
		id = uuid.New()
	}
	inserted, err := r.q.PlantDeviceInsert(ctx, sqlcgen.PlantDeviceInsertParams{
		ID:             id,
		PlantID:        d.PlantID,
		DeviceSn:       d.DeviceSN,
		DeviceName:     d.DeviceName,
		DeviceType:     d.DeviceType,
		DeviceTypeName: d.DeviceTypeName,
		ProviderKey:    d.ProviderKey,
		Brand:          d.Brand,
		Model:          d.Model,
		RatedPowerKw:   decimalPtrToNumeric(d.RatedPowerKw),
		Status:         d.Status,
		EfficiencyPct:  decimalPtrToNumeric(d.EfficiencyPct),
		LastSeenAt:     tsPtrOrZero(d.LastSeenAt),
		At:             ts(d.UpdatedAt),
	})
	if err != nil {
		return model.PlantDevice{}, pgerr.Translate(r.pool, "insert plant device", err)
	}
	return plantDeviceFromRow(inserted)
}

// AlarmRecipients — Isolation: power_plant_alarm_recipients has no
// company_id — join through power_plants. Another tenant's plantID returns
// ErrNotFound.
func (r *PlantRepository) AlarmRecipients(ctx context.Context, s store.Scope, plantID uuid.UUID) ([]model.PlantAlarmRecipient, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	visible, err := r.visible(ctx, s, plantID)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "check plant visibility", err)
	}
	if !visible {
		return nil, store.ErrNotFound
	}
	rows, err := r.q.PlantAlarmRecipientsList(ctx, sqlcgen.PlantAlarmRecipientsListParams{PlantID: plantID, CompanyID: s.CompanyID})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list plant alarm recipients", err)
	}
	out := make([]model.PlantAlarmRecipient, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.PlantAlarmRecipient{PlantID: row.PlantID, Email: row.Email})
	}
	return out, nil
}

// ReplaceAlarmRecipients — Isolation: join power_plant_alarm_recipients
// through power_plants. Another tenant's plantID returns ErrNotFound and
// nothing is replaced.
func (r *PlantRepository) ReplaceAlarmRecipients(ctx context.Context, s store.Scope, plantID uuid.UUID, emails []string) ([]model.PlantAlarmRecipient, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	visible, err := r.visible(ctx, s, plantID)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "check plant visibility", err)
	}
	if !visible {
		return nil, store.ErrNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "begin replace alarm recipients", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.q.WithTx(tx)
	if err := qtx.PlantAlarmRecipientsDelete(ctx, plantID); err != nil {
		return nil, pgerr.Translate(r.pool, "delete plant alarm recipients", err)
	}
	out := make([]model.PlantAlarmRecipient, 0, len(emails))
	for _, email := range emails {
		row, err := qtx.PlantAlarmRecipientInsert(ctx, sqlcgen.PlantAlarmRecipientInsertParams{PlantID: plantID, Email: email})
		if err != nil {
			return nil, pgerr.Translate(r.pool, "insert plant alarm recipient", err)
		}
		out = append(out, model.PlantAlarmRecipient{PlantID: row.PlantID, Email: row.Email})
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, pgerr.Translate(r.pool, "commit replace alarm recipients", err)
	}
	return out, nil
}

// tsPtrOrZero is ts for a NULLABLE timestamptz column: a nil pointer becomes
// SQL NULL rather than the zero instant.
func tsPtrOrZero(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return ts(*t)
}
