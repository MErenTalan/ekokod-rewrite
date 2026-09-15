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

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// productionSource is the fixed model.PlantProduction.Source value every
// row this package writes carries — the column's own documented default.
const productionSource = "isolar"

// quarantineCode / unknownDeviceCode are the operational-message codes this
// package writes into OperationalMessage.Metadata's "code" field —
// OperationalMessage has no dedicated code column (internal/domain/model/
// operations.go), so BLOCKER item 2's "code plant_level_production_unstorable"
// and the task brief's "code unknown_device" both live in Metadata.
const (
	quarantineCode    = "plant_level_production_unstorable"
	unknownDeviceCode = "unknown_device"
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
// own two counts), how many samples were quarantined (the BLOCKER case) and
// how many named a device not yet known to this plant.
type Result struct {
	Inserted, Updated, Quarantined, UnknownDevice int
}

// Store persists device-attributed samples and quarantines plant-level ones
// (the BLOCKER). s must already be a valid Scope for the plant's company —
// a background sync job calls this with store.SystemScope(companyID)
// (global constraint: "background jobs act for a whole company through
// store.SystemScope"); Store itself never constructs one.
//
// Steps (task brief, verbatim):
//  1. Resolve Devices(s, plantID) into a map[ProviderKey]id, falling back to
//     DeviceSN when a sample carries no PSKey match.
//  2. A sample with PSKey == nil is the BLOCKER case: Quarantined++, never
//     stored, never attributed to a fabricated device.
//  3. A sample whose PSKey/DeviceSN matches no known device is
//     UnknownDevice++ — device sync is F9's job; this function never
//     creates a power_plant_devices row.
//  4. The remaining samples are deduplicated on (ts, device) — Task 10 rule
//     D1, last-value-wins (this package's own tie-break choice: D1 does not
//     specify one, and BulkInsert refuses the whole batch on ANY duplicate
//     key, so silently letting `on conflict` arbitrate is not an option) —
//     then written with one BulkInsert call.
//  5. Exactly one AppendMessage when Quarantined > 0 (kind job, category
//     plant-production, status warning, code plant_level_production_unstorable,
//     metadata {plant_id, samples, from, to}) and exactly one more when
//     UnknownDevice > 0 (same kind/category/status, code unknown_device).
func Store(ctx context.Context, s store.Scope, d Deps, plantID uuid.UUID, samples []isolar.ProductionSample) (Result, error) {
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
	rows := make([]model.PlantProduction, 0, len(samples))
	index := make(map[rowKey]int, len(samples))

	var quarantined, unknownDevice int
	var windowFrom, windowTo time.Time
	for _, sample := range samples {
		if windowFrom.IsZero() || sample.Ts.Before(windowFrom) {
			windowFrom = sample.Ts
		}
		if sample.Ts.After(windowTo) {
			windowTo = sample.Ts
		}

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
		if i, exists := index[key]; exists {
			rows[i] = row // dedupe (Task 10 rule D1): last value wins.
			continue
		}
		index[key] = len(rows)
		rows = append(rows, row)
	}

	inserted, updated, err := d.Production.BulkInsert(ctx, s, rows)
	if err != nil {
		return Result{}, err
	}

	result := Result{Inserted: inserted, Updated: updated, Quarantined: quarantined, UnknownDevice: unknownDevice}

	if quarantined > 0 {
		if err := appendMessage(ctx, s, d.Ops, plantID, quarantineCode,
			fmt.Sprintf("iSolarCloud reported %d plant-level production sample(s) for plant %s with no device attribution; "+
				"plant_production's primary key requires a device id, so they were discarded rather than stored against a "+
				"fabricated device. See the plant_production primary-key blocker.", quarantined, plantID),
			quarantined, windowFrom, windowTo); err != nil {
			return result, err
		}
	}
	if unknownDevice > 0 {
		if err := appendMessage(ctx, s, d.Ops, plantID, unknownDeviceCode,
			fmt.Sprintf("iSolarCloud reported %d production sample(s) for plant %s referencing a device not yet known to this "+
				"plant; they were skipped. Device discovery is a separate sync.", unknownDevice, plantID),
			unknownDevice, windowFrom, windowTo); err != nil {
			return result, err
		}
	}

	return result, nil
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
