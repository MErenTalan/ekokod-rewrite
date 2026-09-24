package legacy

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
)

// loadOrder is 08 §6's foreign-key order (R415).
var loadOrder = []string{
	"integration_definitions", "companies", "company_weekend_days", "company_vacations", "calendar_events",
	"users", "user_password_history", "buildings", "building_contacts", "analyzers",
	// Credentials before plants: a linked plant references its iSolar credential (08 §6 lists them after).
	"integration_credentials", "smtp_settings",
	"power_plants", "power_plant_monthly_targets", "power_plant_devices", "power_plant_alarm_recipients",
	"tariffs", "tariff_taxes", "tariff_manual_yekdem", "tariff_templates", "solar_tariffs",
	"alarms", "alarm_analyzers", "alarm_channels", "alarm_events",
	"emission_factors", "emission_factor_conversions", "carbon_selected_activities", "carbon_activities", "carbon_reports",
	"iso50001_projects", "iso50001_clause_dates", "iso50001_notes",
	"market_prices_hourly", "yekdem_monthly", "operational_messages",
	"meter_readings", "plant_production_totals", "consumption_anomalies",
	"legacy_bills", "legacy_reports", "legacy_ids", "stored_files",
}

// loadChunk bounds one transaction's rows (R415).
const loadChunk = 50000

// Target is the database side of a load (admin.LegacyLoader).
type Target interface {
	Describe(ctx context.Context, table string) (admin.LegacyTable, error)
	Upsert(ctx context.Context, table string, desc admin.LegacyTable, keys []string, docs [][]byte, replaceLegacy bool) (int64, error)
}

// LoadTally is one table's outcome (R416): lines = loaded + withheld.
type LoadTally struct {
	Lines    int   `json:"lines"`
	Loaded   int   `json:"loaded"`
	Withheld int   `json:"withheld,omitempty"`
	Affected int64 `json:"affected"`
}

// LoadReport is load_report.json.
type LoadReport struct {
	Tables   map[string]LoadTally `json:"tables"`
	Warnings []string             `json:"warnings,omitempty"`
}

// Load writes a transform directory into the database, table by table in
// dependency order; every stage is an upsert, so a rerun converges (R414–R416).
func Load(ctx context.Context, db Target, dir string) (LoadReport, error) {
	if _, err := os.Stat(filepath.Join(dir, "summary.json")); err != nil {
		return LoadReport{}, fmt.Errorf("legacy: %s is not a transform output (no summary.json): %w", dir, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return LoadReport{}, err
	}
	for _, e := range entries {
		if err := LoadKnows(e.Name()); err != nil {
			return LoadReport{}, err
		}
	}
	withheld, err := unconfirmedAnalyzers(filepath.Join(dir, "manual_multipliers.csv"))
	if err != nil {
		return LoadReport{}, err
	}
	rep := LoadReport{Tables: map[string]LoadTally{}}
	for _, table := range loadOrder {
		f, err := os.Open(filepath.Join(dir, table+".ndjson")) //nolint:gosec // the operator's staging directory
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return rep, err
		}
		tally, err := loadTable(ctx, db, table, f, withheld)
		_ = f.Close()
		if err != nil {
			return rep, err
		}
		if tally.Lines != tally.Loaded+tally.Withheld {
			return rep, fmt.Errorf("legacy: %s: %d lines, %d loaded + %d withheld", table, tally.Lines, tally.Loaded, tally.Withheld)
		}
		rep.Tables[table] = tally
	}
	if n := rep.Tables["meter_readings"].Withheld; n > 0 {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("%d readings of %d analyzers withheld until their multiplier is confirmed (manual_multipliers.csv, Q-J13)", n, len(withheld)))
	}
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return rep, err
	}
	return rep, os.WriteFile(filepath.Join(dir, "load_report.json"), append(b, '\n'), 0o600)
}

func loadTable(ctx context.Context, db Target, table string, r io.Reader, withheld map[string]bool) (LoadTally, error) {
	desc, err := db.Describe(ctx, table)
	if err != nil {
		return LoadTally{}, err
	}
	replace := table == "operational_messages" // Q-J15: one transaction, delete then insert
	var t LoadTally
	var docs [][]byte
	keys := map[string]bool{}
	flush := func() error {
		if len(docs) == 0 {
			return nil
		}
		names := make([]string, 0, len(keys))
		for k := range keys {
			names = append(names, k)
		}
		n, err := db.Upsert(ctx, table, desc, names, docs, replace)
		if err != nil {
			return err
		}
		t.Loaded += len(docs)
		t.Affected += n
		docs, keys = nil, map[string]bool{}
		return nil
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<28)
	for sc.Scan() {
		line := append([]byte(nil), sc.Bytes()...)
		if len(line) == 0 {
			continue
		}
		t.Lines++
		var m map[string]json.RawMessage
		if err := json.Unmarshal(line, &m); err != nil {
			return t, fmt.Errorf("legacy: %s line %d: %w", table, t.Lines, err)
		}
		if table == "meter_readings" {
			var id string
			_ = json.Unmarshal(m["analyzer_id"], &id)
			if withheld[id] {
				t.Withheld++
				continue
			}
		}
		for k := range m {
			keys[k] = true
		}
		docs = append(docs, line)
		if !replace && len(docs) >= loadChunk {
			if err := flush(); err != nil {
				return t, err
			}
		}
	}
	if err := sc.Err(); err != nil {
		return t, err
	}
	return t, flush()
}

// unconfirmedAnalyzers is the set of analyzer ids still awaiting a multiplier answer.
func unconfirmedAnalyzers(path string) (map[string]bool, error) {
	out := map[string]bool{}
	f, err := os.Open(path) //nolint:gosec // staging directory
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	for i, r := range records {
		if i == 0 || len(r) < 2 {
			continue
		}
		out[r[1]] = true
	}
	return out, nil
}

// sideFiles are transform/artifacts outputs that are not tables.
var sideFiles = map[string]bool{"rejects.ndjson": true, "notes.ndjson": true, "artifact_refs.ndjson": true}

// LoadKnows refuses an NDJSON file that is neither a loaded table nor a known
// side file: load would otherwise skip it silently (R416).
func LoadKnows(name string) error {
	table, ok := strings.CutSuffix(name, ".ndjson")
	if !ok || sideFiles[name] {
		return nil
	}
	for _, t := range loadOrder {
		if t == table {
			return nil
		}
	}
	return fmt.Errorf("legacy: %s is not a table load knows: refusing to skip it silently", name)
}
