package admin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// DemoTableKind says what a demo reset does with a table (R546, R549).
type DemoTableKind string

// The kinds.
const (
	DemoPurge   DemoTableKind = "purge"   // every demo row is deleted and regenerated
	DemoKept    DemoTableKind = "kept"    // baseline rows stay (restored by the generator), others are deleted
	DemoSeries  DemoTableKind = "series"  // time series: kept for kept parents, deleted with the others
	DemoCascade DemoTableKind = "cascade" // removed by an ON DELETE CASCADE from a purged or kept table
	DemoRecord  DemoTableKind = "record"  // append-only record, never touched (audit)
)

// DemoTables classifies every table that reaches a company; the schema guard
// test fails when a migration adds one that is missing here.
var DemoTables = map[string]DemoTableKind{
	"companies": DemoKept, "buildings": DemoKept, "analyzers": DemoKept, "power_plants": DemoKept,
	"power_plant_devices": DemoKept, "users": DemoKept,

	"meter_readings": DemoSeries, "provider_hourly_values": DemoSeries, "forecasts": DemoSeries,
	"forecast_gaps": DemoSeries, "ingestion_cursors": DemoSeries, "generation_anchors": DemoSeries,
	"plant_production": DemoSeries, "plant_production_totals": DemoSeries,

	"audit_log": DemoRecord,

	"alarms": DemoPurge, "bills": DemoPurge, "calendar_events": DemoPurge, "carbon_activities": DemoPurge,
	"carbon_reports": DemoPurge, "carbon_selected_activities": DemoPurge, "company_vacations": DemoPurge,
	"company_weekend_days": DemoPurge, "emission_factors": DemoPurge, "icmal_imports": DemoPurge,
	"integration_credentials": DemoPurge, "iso50001_projects": DemoPurge, "job_runs": DemoPurge,
	"legacy_bills": DemoPurge, "legacy_reports": DemoPurge, "operational_messages": DemoPurge,
	"reports": DemoPurge, "smtp_settings": DemoPurge, "solar_tariffs": DemoPurge, "stored_files": DemoPurge,
	"tariff_bulk_assignments": DemoPurge, "tariff_templates": DemoPurge, "tariffs": DemoPurge,
	"building_contacts": DemoPurge, "consumption_anomalies": DemoPurge, "plant_faults": DemoPurge,
	"isolar_forwarded_alarms": DemoPurge, "power_plant_monthly_targets": DemoPurge,
	"power_plant_alarm_recipients": DemoPurge, "bill_periods": DemoPurge,

	"alarm_analyzers": DemoCascade, "alarm_channels": DemoCascade, "alarm_events": DemoCascade,
	"alarm_fired_bills": DemoCascade, "bill_lines": DemoCascade, "bill_members": DemoCascade,
	"bill_hourly_detail": DemoCascade, "emission_factor_conversions": DemoCascade, "icmal_rows": DemoCascade,
	"iso50001_clause_dates": DemoCascade, "iso50001_notes": DemoCascade, "tariff_taxes": DemoCascade,
	"tariff_manual_yekdem": DemoCascade, "tariff_extra_charges": DemoCascade, "sessions": DemoCascade,
	"user_password_history": DemoCascade, "password_reset_tokens": DemoCascade,
}

// DemoKeep names the baseline rows a reset keeps; devices of kept plants are kept too.
type DemoKeep struct {
	Buildings, Analyzers, Plants []uuid.UUID
	User                         uuid.UUID
}

// PurgeResult is what a purge removed that lives outside the transaction.
type PurgeResult struct {
	Files        []string         // stored_files.stored_path of deleted files
	ReadingRange *store.TimeRange // nil when no reading was deleted
	PlantRange   *store.TimeRange
}

// DemoRepository purges the demo company before it is regenerated.
type DemoRepository struct{ pool *pgxpool.Pool }

// NewDemoRepository builds a DemoRepository over pool.
func NewDemoRepository(pool *pgxpool.Pool) *DemoRepository { return &DemoRepository{pool: pool} }

// ErrNotDemo refuses a purge of a company that is not tagged demo.
var ErrNotDemo = errors.New("admin: refusing to purge a company that is not a demo company")

// Purge deletes every mutable row of the demo company in one transaction (R546, R547).
func (r *DemoRepository) Purge(ctx context.Context, companyID uuid.UUID, keep DemoKeep) (PurgeResult, error) {
	var res PurgeResult
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		var isDemo bool
		if err := tx.QueryRow(ctx, `select is_demo from companies where id = $1 for update`, companyID).Scan(&isDemo); err != nil {
			return fmt.Errorf("demo purge: lock company: %w", err)
		}
		if !isDemo {
			return ErrNotDemo
		}
		var err error
		args := pgx.NamedArgs{"company": companyID, "buildings": keep.Buildings, "analyzers": keep.Analyzers,
			"plants": keep.Plants, "user": keep.User}
		if res.ReadingRange, err = timeRange(ctx, tx, `select min(ts), max(ts) from meter_readings
			where analyzer_id in (select id from analyzers where company_id = @company and not id = any(@analyzers))`, args); err != nil {
			return err
		}
		if res.PlantRange, err = timeRange(ctx, tx, `select min(ts), max(ts) from (
			select ts from plant_production where plant_id in (select id from power_plants where company_id = @company and not id = any(@plants))
			union all
			select ts from plant_production_totals where plant_id in (select id from power_plants where company_id = @company and not id = any(@plants))) p`,
			args); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `delete from stored_files where company_id = @company returning stored_path`, args)
		if err != nil {
			return fmt.Errorf("demo purge: stored_files: %w", err)
		}
		if res.Files, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
			return fmt.Errorf("demo purge: stored_files: %w", err)
		}
		for _, st := range purgeStatements {
			if _, err := tx.Exec(ctx, st, args); err != nil {
				return fmt.Errorf("demo purge: %s: %w", st, err)
			}
		}
		return nil
	})
	if err != nil {
		return PurgeResult{}, err
	}
	return res, nil
}

func timeRange(ctx context.Context, tx pgx.Tx, q string, args ...any) (*store.TimeRange, error) {
	var from, to *time.Time
	if err := tx.QueryRow(ctx, q, args...).Scan(&from, &to); err != nil {
		return nil, fmt.Errorf("demo purge: time range: %w", err)
	}
	if from == nil {
		return nil, nil
	}
	return &store.TimeRange{From: *from, To: *to}, nil
}

// purgeStatements run in order. Kept rows are first detached from purgeable
// ones; children go before their parents (FKs without cascade).
var purgeStatements = []string{
	`update buildings set responsible_user_id = null where company_id = @company`,
	`update analyzers set building_id = null where company_id = @company and not building_id = any(@buildings)`,
	`update power_plants set isolar_credential_id = null,
		netting_analyzer_id = case when netting_analyzer_id = any(@analyzers) then netting_analyzer_id end
		where company_id = @company`,
	`delete from alarms where company_id = @company`,
	`delete from bills where company_id = @company`,
	`delete from bill_periods where company_id = @company`,
	`delete from reports where company_id = @company`,
	`delete from legacy_reports where company_id = @company`,
	`delete from legacy_bills where company_id = @company`,
	`delete from tariff_bulk_assignments where company_id = @company`,
	`delete from solar_tariffs where company_id = @company`,
	`delete from tariffs where company_id = @company`,
	`delete from tariff_templates where company_id = @company`,
	`delete from icmal_imports where company_id = @company`,
	`delete from carbon_reports where company_id = @company`,
	`delete from carbon_activities where company_id = @company`,
	`delete from carbon_selected_activities where company_id = @company`,
	`delete from emission_factors where company_id = @company`,
	`delete from iso50001_projects where company_id = @company`,
	`delete from calendar_events where company_id = @company`,
	`delete from company_vacations where company_id = @company`,
	`delete from company_weekend_days where company_id = @company`,
	`delete from job_runs where company_id = @company`,
	`delete from operational_messages where company_id = @company`,
	`delete from smtp_settings where company_id = @company`,
	`delete from consumption_anomalies where analyzer_id in (select id from analyzers where company_id = @company)`,
	`delete from building_contacts where building_id in (select id from buildings where company_id = @company)`,
	`delete from plant_faults where plant_id in (select id from power_plants where company_id = @company)`,
	`delete from isolar_forwarded_alarms where plant_id in (select id from power_plants where company_id = @company)`,
	`delete from power_plant_monthly_targets where plant_id in (select id from power_plants where company_id = @company)`,
	`delete from power_plant_alarm_recipients where plant_id in (select id from power_plants where company_id = @company)`,
	`delete from integration_credentials where company_id = @company`,
	// Time series of analyzers and plants the visitor created, then those rows.
	`delete from meter_readings where analyzer_id in (select id from analyzers where company_id = @company and not id = any(@analyzers))`,
	`delete from provider_hourly_values where analyzer_id in (select id from analyzers where company_id = @company and not id = any(@analyzers))`,
	`delete from forecasts where analyzer_id in (select id from analyzers where company_id = @company and not id = any(@analyzers))`,
	`delete from forecast_gaps where analyzer_id in (select id from analyzers where company_id = @company and not id = any(@analyzers))`,
	`delete from ingestion_cursors where analyzer_id in (select id from analyzers where company_id = @company and not id = any(@analyzers))`,
	`delete from generation_anchors where analyzer_id in (select id from analyzers where company_id = @company and not id = any(@analyzers))`,
	`update power_plants set netting_analyzer_id = null where company_id = @company and not id = any(@plants)`,
	`delete from plant_production where plant_id in (select id from power_plants where company_id = @company and not id = any(@plants))`,
	`delete from plant_production_totals where plant_id in (select id from power_plants where company_id = @company and not id = any(@plants))`,
	`delete from power_plants where company_id = @company and not id = any(@plants)`,
	`delete from analyzers where company_id = @company and not id = any(@analyzers)`,
	`delete from buildings where company_id = @company and not id = any(@buildings)`,
	`delete from users where company_id = @company and id <> @user`,
}

// Restore clears deleted_at on the demo company and its kept rows, so the
// generator can write them back through the scoped repositories (R546). It
// does nothing for a company that is absent or not tagged demo.
func (r *DemoRepository) Restore(ctx context.Context, companyID uuid.UUID, keep DemoKeep) error {
	args := pgx.NamedArgs{"company": companyID, "buildings": keep.Buildings, "analyzers": keep.Analyzers,
		"plants": keep.Plants, "user": keep.User}
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		var isDemo bool
		switch err := tx.QueryRow(ctx, `select is_demo from companies where id = @company for update`, args).Scan(&isDemo); {
		case errors.Is(err, pgx.ErrNoRows):
			return nil
		case err != nil:
			return fmt.Errorf("demo restore: lock company: %w", err)
		case !isDemo:
			return nil
		}
		for _, st := range []string{
			`update companies set deleted_at = null where id = @company`,
			`update buildings set deleted_at = null where company_id = @company and id = any(@buildings)`,
			`update analyzers set deleted_at = null where company_id = @company and id = any(@analyzers)`,
			`update power_plants set deleted_at = null where company_id = @company and id = any(@plants)`,
			`update users set deleted_at = null where company_id = @company and id = @user`,
		} {
			if _, err := tx.Exec(ctx, st, args); err != nil {
				return fmt.Errorf("demo restore: %s: %w", st, err)
			}
		}
		return nil
	})
}
