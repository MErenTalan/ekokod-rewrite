// Package production is the blocker-aware store path for iSolarCloud
// production data (task-13 brief; the plan's ⛔ BLOCKER section,
// docs/superpowers/plans/2026-09-15-f2-integration-layer.md).
//
// plant_production's primary key is (plant_id, ts, device_id), and
// device_id is NOT NULL — even though 04-data-model.md §4.5 describes it as
// nullable, PostgreSQL does not allow a nullable column inside a primary
// key (internal/store/postgres/production.go's own doc comment records the
// same constraint). A plant-level iSolarCloud sample — one with no device
// serial, from getPowerStationPointMinuteDataList /
// getPowerStationPointDayMonthYearDataList — therefore cannot be stored
// under the current schema. Whether iSolarCloud actually reports such
// samples for real installations is an open product-owner question
// (HANDOFF_NEXT_SESSION.md); until it is answered, Store NEVER stores a
// plant-level sample and NEVER attributes it to a fabricated or "first"
// device. It counts the sample as Quarantined and reports the count via one
// operational message per call (BLOCKER item 2). If the product owner
// answers "yes", the fix is a schema migration (next free number) and this
// package's quarantine branch becomes a store call — no code here should be
// read as a permanent decision.
//
// isolar.sync_plant itself (the scheduled job that calls Store) is F9's
// job; this package delivers only the store seam so the blocker cannot leak
// fabricated data into F9 later.
package production

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// productionSource is the fixed model.PlantProduction.Source value every
// row this package writes carries — the column's own documented default.
const productionSource = "isolar"

// quarantineCode / unknownDeviceCode / conflictingDuplicateCode are the
// operational-message codes this package writes into
// OperationalMessage.Metadata's "code" field — OperationalMessage has no
// dedicated code column (internal/domain/model/operations.go), so BLOCKER
// item 2's "code plant_level_production_unstorable", the task brief's
// "code unknown_device" and fix-round-1 finding I3's
// "conflicting_duplicate" all live in Metadata.
const (
	quarantineCode           = "plant_level_production_unstorable"
	unknownDeviceCode        = "unknown_device"
	conflictingDuplicateCode = "conflicting_duplicate"
)

// Deps is the store surface Store needs: F1's PlantRepository (to resolve
// ps_key/device_sn to a stored device id via Devices), ProductionRepository
// (BulkInsert) and OpsRepository (AppendMessage).
type Deps struct {
	Plants     store.PlantRepository
	Production store.ProductionRepository
	Ops        store.OpsRepository
}

// Result is Store's outcome for one call: how many rows were written
// (split into inserted/updated, matching ProductionRepository.BulkInsert's
// own two counts), how many samples were quarantined (the BLOCKER case),
// how many named a device not yet known to this plant, and how many were
// discarded as conflicting duplicates (fix-round-1 finding I3: two samples
// sharing (ts, device) whose values genuinely differ are ALL rejected and
// counted here — never silently resolved by "last value wins").
type Result struct {
	Inserted, Updated, Quarantined, UnknownDevice, ConflictingDuplicate int
}

// Store persists device-attributed samples and quarantines plant-level ones
// (the BLOCKER). s must already be a valid Scope for the plant's company —
// a background sync job calls this with store.SystemScope(companyID)
// (global constraint: "background jobs act for a whole company through
// store.SystemScope"); Store itself never constructs one.
//
// from/to is the FETCH WINDOW [From, To) the caller asked isolar for these
// samples over (e.g. the window it passed to DeviceMinuteSeries/
// PlantMinuteSeries) — fix-round-1 ruling R43: every operational message's
// metadata.from/to is this window, NOT min/max sample.Ts (a window can be
// wider than what happens to be present in the samples, and round 1
// derived it from the samples themselves, which is the bug this ruling
// fixes). R43 also documents: one message per Store call — a re-run over
// the same window that quarantines again appends ANOTHER message, it does
// not collapse into the first.
//
// Steps (task brief, as amended by fix-round-1 rulings R43/I3):
//  1. Resolve Devices(s, plantID) into a map[ProviderKey]id, falling back to
//     DeviceSN when a sample carries no PSKey match.
//  2. A sample with PSKey == nil is the BLOCKER case: Quarantined++, never
//     stored, never attributed to a fabricated device.
//  3. A sample whose PSKey/DeviceSN matches no known device is
//     UnknownDevice++ — device sync is F9's job; this function never
//     creates a power_plant_devices row.
//  4. The remaining samples are grouped by (ts, device) — Task 10 rule D1,
//     as fix-round-1 finding I3 spells it out: a group whose members are
//     ALL identical on every stored field collapses to one row; a group
//     with genuinely differing values is entirely rejected (none of its
//     rows are stored) and counted as ConflictingDuplicate — never
//     silently arbitrated by "last value wins", since BulkInsert itself
//     refuses the whole batch on any duplicate key.
//  5. Exactly one AppendMessage when Quarantined > 0 (kind job, category
//     plant-production, status warning, code plant_level_production_unstorable),
//     one more when UnknownDevice > 0 (code unknown_device), and one more
//     when ConflictingDuplicate > 0 (code conflicting_duplicate) — each
//     metadata {plant_id, samples, from, to} with from/to the fetch window
//     above.
func Store(ctx context.Context, s store.Scope, d Deps, plantID uuid.UUID, from, to time.Time, samples []isolar.ProductionSample) (Result, error) {
	if !s.Valid() {
		return Result{}, store.ErrInvalidScope
	}

	devices, err := d.Plants.Devices(ctx, s, plantID)
	if err != nil {
		return Result{}, err
	}
	byProviderKey := make(map[string]uuid.UUID, len(devices))
	byDeviceSN := make(map[string]uuid.UUID, len(devices))
	for _, dev := range devices {
		if dev.ProviderKey != nil && *dev.ProviderKey != "" {
			byProviderKey[*dev.ProviderKey] = dev.ID
		}
		if dev.DeviceSN != "" {
			byDeviceSN[dev.DeviceSN] = dev.ID
		}
	}

	type rowKey struct {
		ts       time.Time
		deviceID uuid.UUID
	}
	groups := make(map[rowKey][]model.PlantProduction, len(samples))
	var order []rowKey

	var quarantined, unknownDevice int
	for _, sample := range samples {
		if sample.PSKey == nil {
			// The BLOCKER: a plant-level sample carries no device
			// attribution and is never stored, never attributed to a
			// fabricated or "first" device.
			quarantined++
			continue
		}

		deviceID, ok := byProviderKey[*sample.PSKey]
		if !ok && sample.DeviceSN != nil {
			deviceID, ok = byDeviceSN[*sample.DeviceSN]
		}
		if !ok {
			unknownDevice++
			continue
		}

		row := model.PlantProduction{
			PlantID:       plantID,
			Ts:            sample.Ts,
			DeviceID:      deviceID,
			ProductionKwh: sample.ProductionKwh,
			ActivePowerKw: sample.ActivePowerKw,
			IrradianceWm2: sample.IrradianceWm2,
			ModuleTempC:   sample.ModuleTempC,
			AmbientTempC:  sample.AmbientTempC,
			Source:        productionSource,
		}
		key := rowKey{ts: sample.Ts, deviceID: deviceID}
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], row)
	}

	rows := make([]model.PlantProduction, 0, len(order))
	var conflicting int
	for _, key := range order {
		group := groups[key]
		if len(group) == 1 || rowsIdentical(group) {
			// A single sample, or several IDENTICAL ones (I3: "identical
			// duplicates collapse") — keep exactly one.
			rows = append(rows, group[0])
			continue
		}
		// I3: duplicate keys with DIFFERENT values are ALL rejected, never
		// arbitrated by "last value wins".
		conflicting += len(group)
	}

	inserted, updated, err := d.Production.BulkInsert(ctx, s, rows)
	if err != nil {
		return Result{}, err
	}

	result := Result{Inserted: inserted, Updated: updated, Quarantined: quarantined, UnknownDevice: unknownDevice, ConflictingDuplicate: conflicting}

	if quarantined > 0 {
		if err := appendMessage(ctx, s, d.Ops, plantID, quarantineCode,
			fmt.Sprintf("iSolarCloud reported %d plant-level production sample(s) for plant %s with no device attribution; "+
				"plant_production's primary key requires a device id, so they were discarded rather than stored against a "+
				"fabricated device. See the plant_production primary-key blocker.", quarantined, plantID),
			quarantined, from, to); err != nil {
			return result, err
		}
	}
	if unknownDevice > 0 {
		if err := appendMessage(ctx, s, d.Ops, plantID, unknownDeviceCode,
			fmt.Sprintf("iSolarCloud reported %d production sample(s) for plant %s referencing a device not yet known to this "+
				"plant; they were skipped. Device discovery is a separate sync.", unknownDevice, plantID),
			unknownDevice, from, to); err != nil {
			return result, err
		}
	}
	if conflicting > 0 {
		if err := appendMessage(ctx, s, d.Ops, plantID, conflictingDuplicateCode,
			fmt.Sprintf("iSolarCloud reported %d production sample(s) for plant %s sharing the same timestamp and device with "+
				"conflicting values; none were stored (Task 10 rule D1). Investigate the upstream fetch for duplicate or "+
				"overlapping windows.", conflicting, plantID),
			conflicting, from, to); err != nil {
			return result, err
		}
	}

	return result, nil
}

// rowsIdentical reports whether every row in group carries the same
// measurement values (ProductionKwh/ActivePowerKw/IrradianceWm2/
// ModuleTempC/AmbientTempC) as group[0] — I3's "identical duplicates
// collapse" test. group is never empty (called only for len(group) > 1).
func rowsIdentical(group []model.PlantProduction) bool {
	first := group[0]
	for _, row := range group[1:] {
		if !decimalPtrEqual(first.ProductionKwh, row.ProductionKwh) ||
			!decimalPtrEqual(first.ActivePowerKw, row.ActivePowerKw) ||
			!decimalPtrEqual(first.IrradianceWm2, row.IrradianceWm2) ||
			!decimalPtrEqual(first.ModuleTempC, row.ModuleTempC) ||
			!decimalPtrEqual(first.AmbientTempC, row.AmbientTempC) {
			return false
		}
	}
	return true
}

// decimalPtrEqual is a nil-safe decimal.Decimal.Equal: both nil is equal,
// exactly one nil is never equal, both non-nil compares by value (never by
// pointer identity or by scale-sensitive ==).
func decimalPtrEqual(a, b *decimal.Decimal) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// appendMessage writes one OperationalMessage of kind "job", category
// "plant-production", status "warning" — the shape BLOCKER item 2 and the
// task brief's unknown_device rule both share, differing only in code and
// text. metadata's "code" field is why: OperationalMessage has no dedicated
// code column.
func appendMessage(ctx context.Context, s store.Scope, ops store.OpsRepository, plantID uuid.UUID, code, text string, samples int, from, to time.Time) error {
	companyID := s.CompanyID
	metadata, err := json.Marshal(map[string]any{
		"code":     code,
		"plant_id": plantID.String(),
		"samples":  samples,
		"from":     from.UTC().Format(time.RFC3339),
		"to":       to.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("production: marshalling operational message metadata: %w", err)
	}

	relatedType := "power_plant"
	_, err = ops.AppendMessage(ctx, s, model.OperationalMessage{
		CompanyID:   &companyID,
		Kind:        "job",
		Category:    "plant-production",
		Status:      "warning",
		Message:     text,
		RelatedType: &relatedType,
		RelatedID:   &plantID,
		Metadata:    metadata,
	})
	return err
}
