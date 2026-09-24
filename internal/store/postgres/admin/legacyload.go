package admin

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// legacyTables is the closed set `migrate legacy load` may write (R414).
var legacyTables = map[string]bool{
	"integration_definitions": true, "companies": true, "company_weekend_days": true, "company_vacations": true,
	"calendar_events": true, "users": true, "user_password_history": true, "buildings": true, "building_contacts": true,
	"analyzers": true, "power_plants": true, "power_plant_monthly_targets": true, "power_plant_devices": true,
	"power_plant_alarm_recipients": true, "integration_credentials": true, "smtp_settings": true, "tariffs": true,
	"tariff_taxes": true, "tariff_manual_yekdem": true, "tariff_templates": true, "solar_tariffs": true, "alarms": true,
	"alarm_analyzers": true, "alarm_channels": true, "alarm_events": true, "emission_factors": true,
	"emission_factor_conversions": true, "carbon_selected_activities": true, "carbon_activities": true,
	"carbon_reports": true, "iso50001_projects": true, "iso50001_clause_dates": true, "iso50001_notes": true,
	"market_prices_hourly": true, "yekdem_monthly": true, "operational_messages": true, "meter_readings": true,
	"plant_production_totals": true, "consumption_anomalies": true, "legacy_bills": true, "legacy_reports": true,
	"legacy_ids": true, "stored_files": true,
}

// LegacyLoader upserts transform output (JSON objects shaped like rows) into
// the closed table set. It is the deliberate unscoped surface: a migration
// writes every tenant.
type LegacyLoader struct{ pool *pgxpool.Pool }

// NewLegacyLoader builds a LegacyLoader over pool.
func NewLegacyLoader(pool *pgxpool.Pool) *LegacyLoader { return &LegacyLoader{pool: pool} }

// LegacyTable is what the loader needs to know about one target table.
type LegacyTable struct {
	Columns map[string]bool
	Key     []string // primary key columns, the match condition
	// Defaults are NOT NULL columns' default expressions: a JSON null there
	// takes the default on insert and keeps the stored value on update.
	Defaults map[string]string
	// Hypertable: TimescaleDB refuses MERGE … UPDATE on compressed hypertables,
	// so these use insert … on conflict instead.
	Hypertable bool
}

// Describe reads a table's columns and primary key; unknown tables are refused.
func (l *LegacyLoader) Describe(ctx context.Context, table string) (LegacyTable, error) {
	if !legacyTables[table] {
		return LegacyTable{}, fmt.Errorf("legacy load: %q is not a migration target", table)
	}
	t := LegacyTable{Columns: map[string]bool{}, Defaults: map[string]string{}}
	rows, err := l.pool.Query(ctx, `select column_name from information_schema.columns where table_schema = 'public' and table_name = $1`, table)
	if err != nil {
		return t, err
	}
	cols, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return t, err
	}
	for _, c := range cols {
		t.Columns[c] = true
	}
	rows, err = l.pool.Query(ctx, `select a.attname from pg_index i join pg_attribute a on a.attrelid = i.indrelid and a.attnum = any(i.indkey)
		where i.indrelid = $1::regclass and i.indisprimary order by array_position(i.indkey, a.attnum)`, table)
	if err != nil {
		return t, err
	}
	if t.Key, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
		return t, err
	}
	if len(t.Key) == 0 {
		return t, fmt.Errorf("legacy load: %s has no primary key", table)
	}
	if err := l.pool.QueryRow(ctx, `select exists (select 1 from timescaledb_information.hypertables where hypertable_schema = 'public' and hypertable_name = $1)`, table).Scan(&t.Hypertable); err != nil {
		return t, err
	}
	rows, err = l.pool.Query(ctx, `select a.attname, pg_get_expr(d.adbin, d.adrelid) from pg_attribute a
		join pg_attrdef d on d.adrelid = a.attrelid and d.adnum = a.attnum
		where a.attrelid = $1::regclass and a.attnotnull and not a.attisdropped and a.attgenerated = ''`, table)
	if err != nil {
		return t, err
	}
	defer rows.Close()
	for rows.Next() {
		var col, expr string
		if err := rows.Scan(&col, &expr); err != nil {
			return t, err
		}
		t.Defaults[col] = expr
	}
	return t, rows.Err()
}

// Upsert writes docs into table in one transaction: COPY into a temp jsonb
// stage, then MERGE on the primary key (R414). keys are the JSON
// keys to write; each must be a column. replaceLegacy deletes the rows a
// previous load wrote first, for tables without a deterministic key (Q-J15).
func (l *LegacyLoader) Upsert(ctx context.Context, table string, desc LegacyTable, keys []string, docs [][]byte, replaceLegacy bool) (int64, error) {
	if !legacyTables[table] {
		return 0, fmt.Errorf("legacy load: %q is not a migration target", table)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !desc.Columns[k] {
			return 0, fmt.Errorf("legacy load: %s has no column %q", table, k)
		}
	}
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `create temp table legacy_stage (doc jsonb not null) on commit drop`); err != nil {
		return 0, err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"legacy_stage"}, []string{"doc"}, pgx.CopyFromSlice(len(docs), func(i int) ([]any, error) {
		return []any{string(docs[i])}, nil
	})); err != nil {
		return 0, fmt.Errorf("legacy load: %s: copy: %w", table, err)
	}
	if replaceLegacy {
		if _, err := tx.Exec(ctx, `delete from `+pgx.Identifier{table}.Sanitize()+` where metadata ? 'legacy_id'`); err != nil {
			return 0, err
		}
	}
	tag, err := tx.Exec(ctx, mergeSQL(table, desc, keys))
	if err != nil {
		return 0, fmt.Errorf("legacy load: %s: %w", table, err)
	}
	return tag.RowsAffected(), tx.Commit(ctx)
}

func mergeSQL(table string, desc LegacyTable, cols []string) string {
	id := func(n string) string { return pgx.Identifier{n}.Sanitize() }
	t := id(table)
	isKey := map[string]bool{}
	on := make([]string, len(desc.Key))
	for i, k := range desc.Key {
		isKey[k] = true
		on[i] = "t." + id(k) + " = src." + id(k)
	}
	value := func(alias, c string) string {
		if def, ok := desc.Defaults[c]; ok {
			return "coalesce(" + alias + "." + id(c) + ", " + def + ")"
		}
		return alias + "." + id(c)
	}
	names, values, stageValues, set := make([]string, len(cols)), make([]string, len(cols)), make([]string, len(cols)), []string(nil)
	for i, c := range cols {
		names[i], values[i], stageValues[i] = id(c), value("src", c), value("r", c)
		if isKey[c] {
			continue
		}
		v := "src." + id(c)
		if _, ok := desc.Defaults[c]; ok {
			v = "coalesce(src." + id(c) + ", t." + id(c) + ")"
		}
		set = append(set, id(c)+" = "+v)
	}
	if desc.Hypertable {
		// Defaulted columns are ingest metadata here: kept as first written.
		var upd []string
		for _, c := range cols {
			if _, ok := desc.Defaults[c]; !ok && !isKey[c] {
				upd = append(upd, id(c)+" = excluded."+id(c))
			}
		}
		keys := make([]string, len(desc.Key))
		for i, k := range desc.Key {
			keys[i] = id(k)
		}
		sql := "insert into " + t + " (" + strings.Join(names, ", ") + ") select " + strings.Join(stageValues, ", ") + " from legacy_stage s, jsonb_populate_record(null::" + t +
			", s.doc) r on conflict (" + strings.Join(keys, ", ") + ") do "
		if len(upd) == 0 {
			return sql + "nothing"
		}
		return sql + "update set " + strings.Join(upd, ", ")
	}
	sql := "merge into " + t + " t using (select r.* from legacy_stage s, jsonb_populate_record(null::" + t + ", s.doc) r) src on " +
		strings.Join(on, " and ")
	if len(set) > 0 {
		sql += " when matched then update set " + strings.Join(set, ", ")
	}
	return sql + " when not matched then insert (" + strings.Join(names, ", ") + ") values (" + strings.Join(values, ", ") + ")"
}
