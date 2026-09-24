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
}

// TransformResult summarises one run.
type TransformResult struct {
	Summary map[string]Tally `json:"summary"`
	Manual  int              `json:"manual_multipliers"`
}

// tables is the output order; it is also load's (F14b) dependency order.
var tables = []string{
	"integration_definitions", "companies", "company_weekend_days", "company_vacations", "calendar_events",
	"integration_credentials", "users", "user_password_history", "buildings", "building_contacts",
	"analyzers", "meter_readings", "consumption_anomalies", "legacy_ids",
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
	t := &transformer{dir: extractDir, w: w, rj: rj, opt: opt, resealer: Resealer{Keys: opt.Keys, Cipher: opt.Cipher}}
	runErr := t.run()
	if err := errors.Join(runErr, w.close()); err != nil {
		return TransformResult{}, err
	}
	if err := t.writeManual(filepath.Join(outDir, "manual_multipliers.csv")); err != nil {
		return TransformResult{}, err
	}
	for collection, read := range t.read {
		if err := rj.Check(collection, read); err != nil {
			return TransformResult{}, err
		}
	}
	res := TransformResult{Summary: rj.Summary(), Manual: len(t.manual)}
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

	definitions     map[string]uuid.UUID // "OSOS:Baskent" → definition id
	companies       map[string]bool
	users           map[string]bool
	buildingCompany map[string]string
	companySubtypes map[string]map[string]string
}

func (t *transformer) count(collection string) { t.read[collection]++ }

func (t *transformer) run() error {
	t.read = map[string]int{}
	t.definitions, t.companies, t.users = map[string]uuid.UUID{}, map[string]bool{}, map[string]bool{}
	t.buildingCompany, t.companySubtypes = map[string]string{}, map[string]map[string]string{}
	for _, step := range []func() error{t.integrations, t.companiesStep, t.usersStep, t.buildingsStep, t.analyzersStep} {
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
