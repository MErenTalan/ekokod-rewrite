package legacy

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
)

// TransformOptions are the transform's inputs besides the extract (R410, Q-J5).
type TransformOptions struct {
	Keys   Keys
	Cipher *crypto.Cipher
	// Confirmations answers undetermined multipliers: legacy analyzer id → "raw" | "multiplied".
	Confirmations map[string]string
	// Now bounds plausible reading timestamps; a fixed value keeps reruns identical.
	Now time.Time
	// Answers are the operator's answers.csv (kind → legacy id → answer).
	Answers map[string]map[string]string
	// LogsDays is Q-J10's logs retention window (days before Now); 0 means 180.
	LogsDays int
}

// TransformResult summarises one run.
type TransformResult struct {
	Summary map[string]Tally `json:"summary"`
	Manual  int              `json:"manual_multipliers"`
	// Asked counts the other facts waiting for an answer, per kind (manual_<kind>.csv).
	Asked map[string]int `json:"asked,omitempty"`
	// Notes counts deliberate changes (notes.ndjson), e.g. monomial_power_dropped.
	Notes map[string]int `json:"notes,omitempty"`
}

// tables are the transform's output files (load orders them itself, R415).
var tables = []string{
	"integration_definitions", "companies", "company_weekend_days", "company_vacations", "calendar_events",
	"integration_credentials", "users", "user_password_history", "buildings", "building_contacts",
	"analyzers", "meter_readings", "consumption_anomalies",
	"tariffs", "tariff_taxes", "tariff_manual_yekdem", "tariff_templates", "smtp_settings", "market_prices_hourly", "yekdem_monthly",
	"legacy_bills", "legacy_reports", "operational_messages",
	"power_plants", "power_plant_monthly_targets", "power_plant_devices", "power_plant_alarm_recipients", "solar_tariffs", "plant_production_totals",
	"alarms", "alarm_analyzers", "alarm_channels", "alarm_events",
	"emission_factors", "emission_factor_conversions", "carbon_selected_activities", "carbon_activities", "carbon_reports",
	"iso50001_projects", "iso50001_clause_dates", "iso50001_notes",
	"legacy_ids",
}

type writer struct {
	dir   string
	files map[string]*os.File
	bufs  map[string]*bufio.Writer
}

func newWriter(dir string) (*writer, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	w := &writer{dir: dir, files: map[string]*os.File{}, bufs: map[string]*bufio.Writer{}}
	for _, t := range tables {
		f, err := os.Create(filepath.Join(dir, t+".ndjson")) //nolint:gosec // the operator's staging directory
		if err != nil {
			return nil, err
		}
		w.files[t], w.bufs[t] = f, bufio.NewWriter(f)
	}
	return w, nil
}

// row writes one COPY-ready record; encoding/json sorts map keys, so output is stable.
func (w *writer) row(table string, r map[string]any) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if _, err := w.bufs[table].Write(b); err != nil {
		return err
	}
	return w.bufs[table].WriteByte('\n')
}

func (w *writer) legacyID(collection, legacyID, table string, id uuid.UUID) error {
	return w.row("legacy_ids", map[string]any{"collection": collection, "legacy_id": legacyID, "table_name": table, "new_id": id.String()})
}

func (w *writer) close() error {
	var errs []error
	for _, t := range tables {
		errs = append(errs, w.bufs[t].Flush(), w.files[t].Close())
	}
	return errors.Join(errs...)
}

func dec(d *decimal.Decimal) any {
	if d == nil {
		return nil
	}
	return d.String()
}

func ts(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func strOrNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Transform is 08 §5: the verified extract → COPY-ready NDJSON per table,
// rejections with reasons, and the analyzers that need a multiplier decision.
func Transform(extractDir, outDir string, opt TransformOptions) (TransformResult, error) {
	if opt.Keys.Primary == "" {
		return TransformResult{}, errors.New("legacy: the legacy key (--legacy-key) is required: credentials are re-sealed during the transform (R410)")
	}
	if opt.Cipher == nil {
		return TransformResult{}, errors.New("legacy: the new encryption key is required")
	}
	if err := VerifyManifest(extractDir); err != nil {
		return TransformResult{}, err
	}
	w, err := newWriter(outDir)
	if err != nil {
		return TransformResult{}, err
	}
	rf, err := os.Create(filepath.Join(outDir, "rejects.ndjson")) //nolint:gosec // staging directory
	if err != nil {
		return TransformResult{}, err
	}
	defer func() { _ = rf.Close() }()
	rj := NewRejections(rf)
	nf, err := os.Create(filepath.Join(outDir, "notes.ndjson")) //nolint:gosec // staging directory
	if err != nil {
		return TransformResult{}, err
	}
	notesOut := bufio.NewWriter(nf)
	t := &transformer{dir: extractDir, w: w, rj: rj, opt: opt, resealer: Resealer{Keys: opt.Keys, Cipher: opt.Cipher},
		asked: map[string][]manualItem{}, notes: map[string]int{}, notesOut: notesOut}
	runErr := t.run()
	if err := errors.Join(runErr, w.close(), notesOut.Flush(), nf.Close()); err != nil {
		return TransformResult{}, err
	}
	if err := t.writeManual(filepath.Join(outDir, "manual_multipliers.csv")); err != nil {
		return TransformResult{}, err
	}
	if err := t.writeAsked(outDir); err != nil {
		return TransformResult{}, err
	}
	for collection, read := range t.read {
		if err := rj.Check(collection, read); err != nil {
			return TransformResult{}, err
		}
	}
	res := TransformResult{Summary: rj.Summary(), Manual: len(t.manual), Notes: t.notes, Asked: map[string]int{}}
	for kind, items := range t.asked {
		res.Asked[kind] = len(items)
	}
	summary, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return res, err
	}
	return res, os.WriteFile(filepath.Join(outDir, "summary.json"), append(summary, '\n'), 0o600)
}

type manualRow struct {
	legacyID, provider, stored, reason string
	id                                 uuid.UUID
	readings                           int
}

type transformer struct {
	dir      string
	w        *writer
	rj       *Rejections
	opt      TransformOptions
	resealer Resealer
	read     map[string]int
	manual   []manualRow
	asked    map[string][]manualItem
	notes    map[string]int
	notesOut *bufio.Writer

	definitions     map[string]uuid.UUID // "OSOS:Baskent" → definition id
	companies       map[string]bool
	users           map[string]bool
	buildingCompany map[string]string
	companySubtypes map[string]map[string]string
	solarTariffDocs []bson.M // standalone solar tariffs, written with the plants (R424)
}

func (t *transformer) count(collection string) { t.read[collection]++ }

func (t *transformer) run() error {
	t.read = map[string]int{}
	t.definitions, t.companies, t.users = map[string]uuid.UUID{}, map[string]bool{}, map[string]bool{}
	t.buildingCompany, t.companySubtypes = map[string]string{}, map[string]map[string]string{}
	for _, step := range []func() error{t.integrations, t.companiesStep, t.usersStep, t.buildingsStep, t.analyzersStep,
		t.tariffsStep, t.templatesStep, t.smtpStep, t.epiasStep, t.solarTariffsPending} {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

func (t *transformer) writeManual(path string) error {
	f, err := os.Create(path) //nolint:gosec // staging directory
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	cw := csv.NewWriter(f)
	sort.Slice(t.manual, func(i, j int) bool { return t.manual[i].legacyID < t.manual[j].legacyID })
	records := [][]string{{"legacy_id", "analyzer_id", "provider", "stored_multiplier", "reason", "readings", "answer (raw|multiplied)"}}
	for _, m := range t.manual {
		records = append(records, []string{m.legacyID, m.id.String(), m.provider, m.stored, m.reason, fmt.Sprint(m.readings), ""})
	}
	if err := cw.WriteAll(records); err != nil {
		return err
	}
	return f.Close()
}

// solarTariffsPending accounts standalone solar tariffs until plants are migrated.
func (t *transformer) solarTariffsPending() error {
	for _, d := range t.solarTariffDocs {
		t.rj.Reject("tariffs", hexID(d["_id"]), "energy_source", "solar_tariff", nil)
	}
	return nil
}
