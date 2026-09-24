package legacy

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// AnalyzerInventory is one analyzer's risk profile (R407).
type AnalyzerInventory struct {
	LegacyID             string               `json:"legacy_id"`
	Provider             string               `json:"provider"`
	Readings             map[string]int       `json:"readings"`
	First                map[string]time.Time `json:"first"`
	Last                 map[string]time.Time `json:"last"`
	GapsOver24h          int                  `json:"gaps_over_24h"`
	IndexDrops           int                  `json:"index_drops"`
	Duplicates           int                  `json:"duplicates"`
	Rejected             int                  `json:"rejected"`
	MultiplierMissing    bool                 `json:"multiplier_missing"`
	MultiplierDetermined bool                 `json:"multiplier_determined"`
	MultiplierReason     string               `json:"multiplier_reason"`
}

// TariffInventory covers embedded and standalone tariffs.
type TariffInventory struct {
	Embedded             int            `json:"embedded"`
	Standalone           int            `json:"standalone"`
	EffectiveFromLayouts map[string]int `json:"effective_from_layouts"`
	MissingVAT           int            `json:"missing_vat"`
	PTFWithoutEnergyKBK  int            `json:"ptf_without_energy_kbk"`
}

// InvoicePeriod is the reconciliation baseline for one month.
type InvoicePeriod struct {
	Period string          `json:"period"`
	Count  int             `json:"count"`
	Total  decimal.Decimal `json:"total"`
}

// ArtifactDir is one artifact directory's size.
type ArtifactDir struct {
	Path  string `json:"path"`
	Files int    `json:"files"`
	Bytes int64  `json:"bytes"`
}

// Inventory is 08 §3's read-only survey; it is the migration's specification.
type Inventory struct {
	Collections         map[string]int      `json:"collections"`
	UsersByRole         map[string]int      `json:"users_by_role"`
	AnalyzersByProvider map[string]int      `json:"analyzers_by_provider"`
	Analyzers           []AnalyzerInventory `json:"analyzers"`
	Tariffs             TariffInventory     `json:"tariffs"`
	Invoices            []InvoicePeriod     `json:"invoices"`
	Artifacts           []ArtifactDir       `json:"artifacts"`
	MissingOnDisk       []string            `json:"missing_on_disk"`
	Unreferenced        []string            `json:"unreferenced"`
}

// Providers resolves an analyzer's provider: analyzer.building → building.company_id →
// company.integrations[subType == analyzer.subIntegration].type.
type Providers struct {
	buildingCompany map[string]string
	companySubtypes map[string]map[string]string
}

// Of names an analyzer's provider in upper case, or UNKNOWN.
func (p Providers) Of(analyzer bson.M) string {
	company := p.buildingCompany[hexID(analyzer["building"])]
	if t, ok := p.companySubtypes[company][str(analyzer, "subIntegration")]; ok {
		return strings.ToUpper(t)
	}
	return "UNKNOWN"
}

func each(ctx context.Context, src Source, collection string, fn func(bson.M) error) (int, error) {
	n := 0
	err := src.Iterate(ctx, collection, func(raw bson.Raw) error {
		n++
		doc, err := rawToM(raw)
		if err != nil {
			return fmt.Errorf("%s document %d: %w", collection, n, err)
		}
		return fn(doc)
	})
	return n, err
}

// LoadProviders reads companies and buildings once.
func LoadProviders(ctx context.Context, src Source) (Providers, error) {
	p := Providers{buildingCompany: map[string]string{}, companySubtypes: map[string]map[string]string{}}
	if _, err := each(ctx, src, "companies", func(c bson.M) error {
		subs := map[string]string{}
		for _, i := range asArray(c["integrations"]) {
			in := asDoc(i)
			subs[str(in, "subType")] = str(in, "type")
		}
		p.companySubtypes[hexID(c["_id"])] = subs
		return nil
	}); err != nil {
		return p, err
	}
	_, err := each(ctx, src, "buildings", func(b bson.M) error {
		p.buildingCompany[hexID(b["_id"])] = hexID(b["company_id"])
		return nil
	})
	return p, err
}

// TakeInventory is R407. It only reads: src is a Source, and the artifact walk only stats.
func TakeInventory(ctx context.Context, src Source, now time.Time, artifactDirs []string) (Inventory, error) {
	inv := Inventory{Collections: map[string]int{}, UsersByRole: map[string]int{}, AnalyzersByProvider: map[string]int{},
		Tariffs: TariffInventory{EffectiveFromLayouts: map[string]int{}}}
	names, err := src.Collections(ctx)
	if err != nil {
		return inv, err
	}
	for _, name := range names {
		n, err := each(ctx, src, name, func(bson.M) error { return nil })
		if err != nil {
			return inv, err
		}
		inv.Collections[name] = n
	}
	providers, err := LoadProviders(ctx, src)
	if err != nil {
		return inv, err
	}
	referenced := map[string]bool{}
	invoices := map[string]*InvoicePeriod{}
	collect := func(doc bson.M) { findPaths(doc, artifactDirs, referenced) }

	if _, err := each(ctx, src, "users", func(u bson.M) error {
		inv.UsersByRole[str(u, "userType")]++
		return nil
	}); err != nil {
		return inv, err
	}
	tariff := func(t bson.M) {
		inv.Tariffs.EffectiveFromLayouts[DateLayout(str(t, "effectiveFrom"))]++
		if v, _ := num(asDoc(t["price"])["vat_rate"]); v == nil {
			inv.Tariffs.MissingVAT++
		}
		if ptf, _ := t["usePtfYekdem"].(bool); ptf {
			if v, _ := num(asDoc(t["kbk"])["energyKbk"]); v == nil {
				inv.Tariffs.PTFWithoutEnergyKBK++
			}
		}
	}
	bills := func(doc bson.M) {
		for period, v := range asDoc(doc["billHistory"]) {
			b := asDoc(v)
			total, _ := num(b["totalCost"])
			p := invoices[period]
			if p == nil {
				p = &InvoicePeriod{Period: period}
				invoices[period] = p
			}
			p.Count++
			if total != nil {
				p.Total = p.Total.Add(*total)
			}
		}
	}
	if _, err := each(ctx, src, "buildings", func(b bson.M) error {
		for _, t := range asArray(b["tariffs"]) {
			inv.Tariffs.Embedded++
			tariff(asDoc(t))
		}
		bills(b)
		collect(b)
		return nil
	}); err != nil {
		return inv, err
	}
	if inv.Tariffs.Standalone, err = each(ctx, src, "tariffs", func(t bson.M) error {
		tariff(t)
		return nil
	}); err != nil {
		return inv, err
	}
	if _, err := each(ctx, src, "analyzers", func(a bson.M) error {
		provider := providers.Of(a)
		inv.AnalyzersByProvider[provider]++
		inv.Analyzers = append(inv.Analyzers, analyzerInventory(a, provider, now))
		bills(a)
		collect(a)
		return nil
	}); err != nil {
		return inv, err
	}
	for _, name := range []string{"reports", "carbonreporthistories", "companies"} {
		if _, err := each(ctx, src, name, func(d bson.M) error { collect(d); return nil }); err != nil {
			return inv, err
		}
	}
	for _, p := range invoices {
		inv.Invoices = append(inv.Invoices, *p)
	}
	sort.Slice(inv.Invoices, func(i, j int) bool { return inv.Invoices[i].Period < inv.Invoices[j].Period })
	return inv, inv.artifacts(artifactDirs, referenced)
}

func analyzerInventory(a bson.M, provider string, now time.Time) AnalyzerInventory {
	dec := DecideMultiplier(provider, str(a, "meterMultiplier"), "")
	res := AnalyzerReadings(a, provider, dec, now)
	ai := AnalyzerInventory{LegacyID: hexID(a["_id"]), Provider: provider, Readings: map[string]int{}, First: map[string]time.Time{},
		Last: map[string]time.Time{}, IndexDrops: len(res.Drops), Duplicates: res.Duplicates, Rejected: len(res.Rejects),
		MultiplierMissing: strings.TrimSpace(str(a, "meterMultiplier")) == "", MultiplierDetermined: dec.Determined, MultiplierReason: dec.Reason}
	prev := map[model.ReadingKind]time.Time{}
	for _, r := range res.Readings { // sorted by ts within each kind
		k := string(r.Kind)
		ai.Readings[k]++
		if f, ok := ai.First[k]; !ok || r.Ts.Before(f) {
			ai.First[k] = r.Ts
		}
		if r.Ts.After(ai.Last[k]) {
			ai.Last[k] = r.Ts
		}
		if p, ok := prev[r.Kind]; ok && r.Kind != model.ReadingKindBilling && r.Ts.Sub(p) > 24*time.Hour {
			ai.GapsOver24h++
		}
		prev[r.Kind] = r.Ts
	}
	return ai
}

// findPaths collects every string value under one of the artifact directories.
func findPaths(v any, dirs []string, out map[string]bool) {
	switch x := v.(type) {
	case string:
		for _, d := range dirs {
			if strings.HasPrefix(x, strings.TrimRight(d, "/")+"/") {
				out[filepath.Clean(x)] = true
			}
		}
	case bson.M:
		for _, e := range x {
			findPaths(e, dirs, out)
		}
	case bson.D:
		for _, e := range x {
			findPaths(e.Value, dirs, out)
		}
	case bson.A:
		for _, e := range x {
			findPaths(e, dirs, out)
		}
	}
}

func (inv *Inventory) artifacts(dirs []string, referenced map[string]bool) error {
	onDisk := map[string]bool{}
	for _, dir := range dirs {
		ad := ArtifactDir{Path: dir}
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			ad.Files++
			ad.Bytes += info.Size()
			onDisk[filepath.Clean(path)] = true
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		inv.Artifacts = append(inv.Artifacts, ad)
	}
	for p := range referenced {
		if !onDisk[p] {
			inv.MissingOnDisk = append(inv.MissingOnDisk, p)
		}
	}
	for p := range onDisk {
		if !referenced[p] {
			inv.Unreferenced = append(inv.Unreferenced, p)
		}
	}
	sort.Strings(inv.MissingOnDisk)
	sort.Strings(inv.Unreferenced)
	return nil
}

// DateLayout names the layout of a legacy date string, for the inventory's format census.
func DateLayout(s string) string {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return "empty"
	case reYMDClock.MatchString(s):
		return "yyyy-MM-dd HH:mm"
	case reYMD.MatchString(s):
		return "yyyy-MM-dd"
	case reYM.MatchString(s):
		return "yyyy-MM"
	case reDMY.MatchString(s):
		return map[byte]string{'-': "dd-MM-yyyy", '.': "dd.MM.yyyy", '/': "dd/MM/yyyy"}[s[strings.IndexAny(s, "-./")]]
	case reDMY2.MatchString(s):
		return "two-digit year"
	case reDMonY.MatchString(strings.ToLower(s)):
		return "dd-MMM-yyyy"
	}
	if _, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return "iso8601"
	}
	return "unknown"
}

// Text renders the inventory for an operator's review.
func (inv Inventory) Text() string {
	var b strings.Builder
	line := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	sortedKeys := func(m map[string]int) []string {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys
	}
	line("== collections")
	for _, k := range sortedKeys(inv.Collections) {
		line("%s: %d", k, inv.Collections[k])
	}
	line("== users by role")
	for _, k := range sortedKeys(inv.UsersByRole) {
		line("%s: %d", k, inv.UsersByRole[k])
	}
	line("== analyzers by provider")
	for _, k := range sortedKeys(inv.AnalyzersByProvider) {
		line("%s: %d", k, inv.AnalyzersByProvider[k])
	}
	line("== analyzers")
	for _, a := range inv.Analyzers {
		line("%s %s readings %v · gaps > 24 h: %d · index drops: %d · duplicates: %d · rejected: %d · multiplier %s (determined %t)",
			a.LegacyID, a.Provider, a.Readings, a.GapsOver24h, a.IndexDrops, a.Duplicates, a.Rejected, a.MultiplierReason, a.MultiplierDetermined)
	}
	line("== tariffs: embedded %d, standalone %d, missing VAT: %d, PTF+YEKDEM without energy KBK: %d",
		inv.Tariffs.Embedded, inv.Tariffs.Standalone, inv.Tariffs.MissingVAT, inv.Tariffs.PTFWithoutEnergyKBK)
	for _, k := range sortedKeys(inv.Tariffs.EffectiveFromLayouts) {
		line("  effectiveFrom %s: %d", k, inv.Tariffs.EffectiveFromLayouts[k])
	}
	line("== invoices (reconciliation baseline)")
	for _, p := range inv.Invoices {
		line("%s: %d invoices, total %s", p.Period, p.Count, p.Total.StringFixed(2))
	}
	line("== artifacts")
	for _, d := range inv.Artifacts {
		line("%s: %d files, %d bytes", d.Path, d.Files, d.Bytes)
	}
	for _, p := range inv.MissingOnDisk {
		line("referenced but missing: %s", p)
	}
	for _, p := range inv.Unreferenced {
		line("on disk, referenced by nothing: %s", p)
	}
	return b.String()
}
