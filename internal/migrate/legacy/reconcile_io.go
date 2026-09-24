package legacy

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
)

// ReconcileSource is the database side (admin.ReconcileRepository).
type ReconcileSource interface {
	LegacyBills(ctx context.Context) ([]admin.LegacyBillRow, error)
	LiveBills(ctx context.Context) ([]admin.LiveBillRow, error)
	UnresolvedAnomalies(ctx context.Context) ([]admin.Anomaly, error)
	ReadingCounts(ctx context.Context) (map[uuid.UUID]map[string]int, error)
	CarbonByBuildingYear(ctx context.Context) (map[uuid.UUID]map[int]decimal.Decimal, error)
	StoredFiles(ctx context.Context, ids []uuid.UUID) (int, int64, error)
	Names(ctx context.Context) (admin.Names, error)
}

// discarded are the legacy collections 08 §4 recomputes instead of migrating.
var discarded = map[string]bool{"consumptions": true, "aipredicts": true, "epiasdailyaverages": true}

// perDocument maps a collection to the tally that counts its documents; the
// others are tallied per sub-record (activities, reports, dates, factors).
var perDocument = map[string]string{
	"integrations": "integrations", "companies": "companies", "users": "users", "buildings": "buildings", "analyzers": "analyzers",
	"tariffs": "tariffs", "tarifftemplates": "tarifftemplates", "smtpsettings": "smtpsettings", "epiashistories": "epiashistories",
	"powerplants": "powerplants", "alarms": "alarms", "reports": "reports", "logs": "logs", "notes": "iso_notes", "files": "iso_files",
}

var perSubRecord = map[string]bool{"carbonfootprint": true, "companyemissionfactors": true, "carbonreporthistories": true, "projectDates": true}

// GatherReconcile reads the extract, the transform directory and the database (R437–R441).
func GatherReconcile(ctx context.Context, db ReconcileSource, transformDir, extractDir string, now time.Time) (Inputs, error) {
	in := Inputs{Now: now}
	var err error
	if in.LegacyBills, err = db.LegacyBills(ctx); err != nil {
		return in, err
	}
	if in.LiveBills, err = db.LiveBills(ctx); err != nil {
		return in, err
	}
	if in.Anomalies, err = db.UnresolvedAnomalies(ctx); err != nil {
		return in, err
	}
	if in.Readings, err = db.ReadingCounts(ctx); err != nil {
		return in, err
	}
	if in.CarbonNew, err = db.CarbonByBuildingYear(ctx); err != nil {
		return in, err
	}
	if in.Names, err = db.Names(ctx); err != nil {
		return in, err
	}
	if in.Global, err = rowCounts(transformDir, extractDir); err != nil {
		return in, err
	}
	artifacts, err := artifactRows(ctx, db, transformDir)
	if err != nil {
		return in, err
	}
	in.Global = append(in.Global, artifacts...)
	if in.Expected, err = expectedReadings(filepath.Join(transformDir, "meter_readings.ndjson")); err != nil {
		return in, err
	}
	withheld, err := unconfirmedAnalyzers(filepath.Join(transformDir, "manual_multipliers.csv"))
	if err != nil {
		return in, err
	}
	in.Withheld = map[uuid.UUID]bool{}
	for id := range withheld {
		if u, err := uuid.Parse(id); err == nil {
			in.Withheld[u] = true
		}
	}
	in.CarbonLegacy, in.CarbonAuto, err = legacyCarbon(extractDir)
	return in, err
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path) //nolint:gosec // staging directory
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func rowCounts(transformDir, extractDir string) ([]Row, error) {
	m, err := ReadManifest(extractDir)
	if err != nil {
		return nil, err
	}
	var summary TransformResult
	if err := readJSON(filepath.Join(transformDir, "summary.json"), &summary); err != nil {
		return nil, err
	}
	var rows []Row
	for _, e := range m.Collections {
		r := Row{Check: CheckRowCounts, Subject: e.Collection, Legacy: dptr(decimal.NewFromInt(int64(e.Count))), Reasons: []Reason{}}
		switch tally, ok := perDocument[e.Collection]; {
		case discarded[e.Collection]:
			r.Reasons = []Reason{ReasonDiscarded}
		case ok:
			t := summary.Summary[tally]
			r.New = dptr(decimal.NewFromInt(int64(t.Accepted + t.Rejected)))
			if t.Accepted+t.Rejected != e.Count {
				r.Diff, r.Reasons = dptr(r.New.Sub(*r.Legacy)), []Reason{Unexplained}
			}
		case perSubRecord[e.Collection]:
			// Tallied per activity/report/date/factor; the invariant is enforced by the transform.
		default:
			if e.Count > 0 {
				r.Reasons = []Reason{Unexplained} // a collection the transform never reads
			}
		}
		rows = append(rows, r)
	}
	var load LoadReport
	if err := readJSON(filepath.Join(transformDir, "load_report.json"), &load); err != nil {
		return nil, fmt.Errorf("reconcile needs a loaded directory: %w", err)
	}
	tables := make([]string, 0, len(load.Tables))
	for t := range load.Tables {
		tables = append(tables, t)
	}
	sort.Strings(tables)
	for _, t := range tables {
		lt := load.Tables[t]
		if lt.Withheld == 0 {
			continue
		}
		rows = append(rows, Row{Check: CheckRowCounts, Subject: "load " + t, Legacy: dptr(decimal.NewFromInt(int64(lt.Lines))),
			New: dptr(decimal.NewFromInt(int64(lt.Loaded))), Diff: dptr(decimal.NewFromInt(int64(-lt.Withheld))), Reasons: []Reason{ReasonMultiplierPending}})
	}
	return rows, nil
}

func artifactRows(ctx context.Context, db ReconcileSource, dir string) ([]Row, error) {
	var rep ArtifactsReport
	if err := readJSON(filepath.Join(dir, "artifacts_report.json"), &rep); errors.Is(err, os.ErrNotExist) {
		return []Row{{Check: CheckArtifacts, Subject: "artifacts not copied", Reasons: []Reason{Unexplained}}}, nil
	} else if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(dir, "stored_files.ndjson")) //nolint:gosec // staging directory
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var ids []uuid.UUID
	var bytes int64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r struct {
			ID   uuid.UUID `json:"id"`
			Size int64     `json:"size_bytes"`
		}
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, err
		}
		ids, bytes = append(ids, r.ID), bytes+r.Size
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	count, dbBytes, err := db.StoredFiles(ctx, ids)
	if err != nil {
		return nil, err
	}
	row := func(subject string, want, got int64) Row {
		r := Row{Check: CheckArtifacts, Subject: subject, Legacy: dptr(decimal.NewFromInt(want)), New: dptr(decimal.NewFromInt(got)), Reasons: []Reason{}}
		if want != got {
			r.Diff, r.Reasons = dptr(decimal.NewFromInt(got-want)), []Reason{Unexplained}
		}
		return r
	}
	rows := []Row{row("files", int64(rep.Copied+rep.AlreadyThere), int64(count)), row("bytes", bytes, dbBytes)}
	if len(rep.Missing) > 0 {
		rows = append(rows, Row{Check: CheckArtifacts, Subject: fmt.Sprintf("referenced but missing on disk: %d (artifacts_report.json)", len(rep.Missing)), Reasons: []Reason{}})
	}
	return rows, nil
}

func expectedReadings(path string) (map[uuid.UUID]map[string]int, error) {
	out := map[uuid.UUID]map[string]int{}
	f, err := os.Open(path) //nolint:gosec // staging directory
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var r struct {
			Analyzer uuid.UUID `json:"analyzer_id"`
			TS       time.Time `json:"ts"`
		}
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, err
		}
		if out[r.Analyzer] == nil {
			out[r.Analyzer] = map[string]int{}
		}
		out[r.Analyzer][r.TS.In(istanbul).Format("2006-01")]++
	}
	return out, sc.Err()
}

func legacyCarbon(extractDir string) (map[uuid.UUID]map[int]decimal.Decimal, map[uuid.UUID]map[int]bool, error) {
	sums, auto := map[uuid.UUID]map[int]decimal.Decimal{}, map[uuid.UUID]map[int]bool{}
	err := ReadExtract(extractDir, "carbonfootprint", func(c bson.M) error {
		b := ID("buildings", hexID(c["building_id"]))
		for _, v := range asArray(c["activities"]) {
			a := asDoc(v)
			d, err := ParseDate(str(a, "date"))
			e, _ := num(a["emissionCo2e"])
			if err != nil || e == nil {
				continue
			}
			y := d.In(istanbul).Year()
			if sums[b] == nil {
				sums[b], auto[b] = map[int]decimal.Decimal{}, map[int]bool{}
			}
			sums[b][y] = sums[b][y].Add(*e)
			if str(a, "description") == automatedCarbon {
				auto[b][y] = true
			}
		}
		return nil
	})
	return sums, auto, err
}

func fmtDec(d *decimal.Decimal) string {
	if d == nil {
		return ""
	}
	return d.String()
}

func joinReasons(rs []Reason) string {
	s := make([]string, len(rs))
	for i, r := range rs {
		s[i] = string(r)
	}
	return strings.Join(s, ";")
}

// RenderCSV writes one line per row (R442).
func RenderCSV(w io.Writer, rep Report) error {
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"company_id", "company", "check", "subject", "period", "legacy", "new", "diff", "pct", "reasons"})
	for _, r := range rep.Global {
		_ = cw.Write([]string{"", "(global)", r.Check, r.Subject, r.Period, fmtDec(r.Legacy), fmtDec(r.New), fmtDec(r.Diff), fmtDec(r.Pct), joinReasons(r.Reasons)})
	}
	for _, c := range rep.Companies {
		for _, r := range c.Rows {
			_ = cw.Write([]string{c.ID, c.Name, r.Check, r.Subject, r.Period, fmtDec(r.Legacy), fmtDec(r.New), fmtDec(r.Diff), fmtDec(r.Pct), joinReasons(r.Reasons)})
		}
	}
	cw.Flush()
	return cw.Error()
}

var reportHTML = template.Must(template.New("r").Funcs(template.FuncMap{"dec": fmtDec, "reasons": joinReasons,
	"blocks": func(r Row) bool { return r.Blocks() }}).Parse(`<!doctype html>
<html lang="tr"><head><meta charset="utf-8"><title>Mutabakat raporu</title>
<style>
body{font:14px/1.45 system-ui,sans-serif;margin:24px;color:#1b1f24;background:#fff}
h1{font-size:22px}h2{font-size:18px;margin-top:32px}table{border-collapse:collapse;width:100%;margin:8px 0}
th,td{border:1px solid #d0d7de;padding:4px 8px;text-align:left;vertical-align:top}th{background:#f3f4f6}
td.n{text-align:right;font-variant-numeric:tabular-nums}.pass{color:#116329;font-weight:600}.fail{color:#a40e26;font-weight:600}
tr.block td{background:#fff1f1}
</style></head><body>
<h1>Geçiş mutabakat raporu</h1>
<p>Oluşturulma: {{.GeneratedAt.Format "2006-01-02 15:04 MST"}} — Genel sonuç:
{{if .Pass}}<span class="pass">GEÇTİ</span>{{else}}<span class="fail">BAŞARISIZ: açıklanamayan fark var (geçişi engeller)</span>{{end}}</p>
<h2>Genel kontroller</h2>
<table><thead><tr><th>Kontrol</th><th>Konu</th><th>Eski</th><th>Yeni</th><th>Fark</th><th>Neden</th></tr></thead><tbody>
{{range .Global}}<tr{{if blocks .}} class="block"{{end}}><td>{{.Check}}</td><td>{{.Subject}}</td><td class="n">{{dec .Legacy}}</td><td class="n">{{dec .New}}</td><td class="n">{{dec .Diff}}</td><td>{{reasons .Reasons}}</td></tr>
{{end}}</tbody></table>
{{range .Companies}}<h2>{{.Name}} — {{if .Pass}}<span class="pass">GEÇTİ</span>{{else}}<span class="fail">BAŞARISIZ</span>{{end}}</h2>
{{if .Rows}}<p>{{range $k, $v := .Reasons}}{{$k}}: {{$v}} · {{end}}</p>
<table><thead><tr><th>Kontrol</th><th>Konu</th><th>Dönem</th><th>Eski</th><th>Yeni</th><th>Fark</th><th>Fark %</th><th>Neden</th></tr></thead><tbody>
{{range .Rows}}<tr{{if blocks .}} class="block"{{end}}><td>{{.Check}}</td><td>{{.Subject}}</td><td>{{.Period}}</td><td class="n">{{dec .Legacy}}</td><td class="n">{{dec .New}}</td><td class="n">{{dec .Diff}}</td><td class="n">{{dec .Pct}}</td><td>{{reasons .Reasons}}</td></tr>
{{end}}</tbody></table>{{else}}<p>Tolerans dışında fark yok.</p>{{end}}
{{end}}</body></html>
`))

// RenderHTML writes the self-contained report (R442).
func RenderHTML(w io.Writer, rep Report) error { return reportHTML.Execute(w, rep) }
