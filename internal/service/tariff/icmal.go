package tariff

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff/icmal"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Import statuses (icmal_imports.status).
const (
	ImportStatusPending  = "pending"
	ImportStatusAnalysed = "analysed"
	ImportStatusApplied  = "applied"
)

// ImportResult is an analysed icmal import.
type ImportResult struct {
	Import    model.IcmalImport
	Analyses  []icmal.Analysis
	Unmatched []string // ETSO codes with no (or more than one) building
	Warnings  []icmal.Warning
}

type storedResult struct {
	Analyses  []icmal.Analysis `json:"analyses"`
	Unmatched []string         `json:"unmatched"`
	Warnings  []icmal.Warning  `json:"warnings"`
}

var thousand = decimal.NewFromInt(1000)

// Import parses and analyses an icmal and stores its rows and result. It
// writes no tariff (02 §8.4).
func (s *Service) Import(ctx context.Context, sc store.Scope, uploadedBy uuid.UUID, fileName string, content []byte) (ImportResult, error) {
	table, err := ReadSheet(fileName, content)
	if err != nil {
		return ImportResult{}, err
	}
	parsed, err := icmal.Parse(table)
	if err != nil {
		return ImportResult{}, fmt.Errorf("%w: %w", ErrInvalidRequest, err)
	}

	analyzers, err := listAll(func(p store.Page) ([]model.Analyzer, error) {
		return s.deps.Analyzers.List(ctx, sc, store.AnalyzerFilter{Page: p})
	})
	if err != nil {
		return ImportResult{}, err
	}
	buildingsByEtso := map[string][]uuid.UUID{}
	for _, a := range analyzers {
		if a.EtsoCode != nil && a.BuildingID != nil && !slices.Contains(buildingsByEtso[*a.EtsoCode], *a.BuildingID) {
			buildingsByEtso[*a.EtsoCode] = append(buildingsByEtso[*a.EtsoCode], *a.BuildingID)
		}
	}
	res := ImportResult{Warnings: parsed.Warnings}
	latest := map[string]string{}
	unmatched := map[string]bool{}
	for i, r := range parsed.Rows {
		if r.EtsoCode == nil {
			continue
		}
		etso := *r.EtsoCode
		if r.Period > latest[etso] {
			latest[etso] = r.Period
		}
		switch b := buildingsByEtso[etso]; len(b) {
		case 1:
			parsed.Rows[i].BuildingID = &b[0]
		case 0:
			unmatched[etso] = true
		default:
			unmatched[etso] = true
			res.Warnings = append(res.Warnings, icmal.Warning{Row: i + 1, Code: "etso_ambiguous", Text: "ETSO code belongs to several buildings"})
		}
	}
	for etso := range unmatched {
		res.Unmatched = append(res.Unmatched, etso)
	}
	slices.Sort(res.Unmatched)

	base, err := s.basePrices(ctx, sc, parsed.Rows)
	if err != nil {
		return ImportResult{}, err
	}
	contracted := map[string]decimal.Decimal{}
	for etso, period := range latest {
		b := buildingsByEtso[etso]
		if len(b) != 1 {
			continue
		}
		t, err := s.deps.Tariffs.Effective(ctx, sc, b[0], s.periodStart(period))
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return ImportResult{}, err
		}
		if t.ContractedPowerKw != nil {
			contracted[etso] = *t.ContractedPowerKw
		}
	}
	params, err := s.deps.Params.Effective(ctx, sc, s.deps.Clock.Now())
	if err != nil {
		return ImportResult{}, err
	}
	res.Analyses = icmal.Analyse(parsed.Rows, base, contracted, params.DemandOverrunMultiplier)

	imp, err := s.deps.Icmal.CreateImport(ctx, sc, model.IcmalImport{CompanyID: sc.CompanyID, UploadedBy: &uploadedBy, FileName: fileName,
		RowCount: int32(len(parsed.Rows)), Status: ImportStatusPending, CreatedAt: s.deps.Clock.Now().UTC()})
	if err != nil {
		return ImportResult{}, err
	}
	if len(parsed.Rows) > 0 {
		for i := range parsed.Rows {
			parsed.Rows[i].ImportID = imp.ID
		}
		if _, err := s.deps.Icmal.InsertRows(ctx, sc, imp.ID, parsed.Rows); err != nil {
			return ImportResult{}, err
		}
	}
	raw, err := json.Marshal(storedResult{Analyses: res.Analyses, Unmatched: res.Unmatched, Warnings: res.Warnings})
	if err != nil {
		return ImportResult{}, err
	}
	if res.Import, err = s.deps.Icmal.UpdateImportResult(ctx, sc, imp.ID, ImportStatusAnalysed, raw); err != nil {
		return ImportResult{}, err
	}
	return res, nil
}

// basePrices is (month-average stored PTF + YEKDEM) / 1000 per period that has both.
func (s *Service) basePrices(ctx context.Context, sc store.Scope, rows []model.IcmalRow) (map[string]decimal.Decimal, error) {
	out := map[string]decimal.Decimal{}
	for _, r := range rows {
		if _, done := out[r.Period]; done {
			continue
		}
		from := s.periodStart(r.Period)
		if from.IsZero() {
			continue
		}
		prices, err := s.deps.Prices.HourlyRange(ctx, sc, store.TimeRange{From: from, To: from.AddDate(0, 1, 0)})
		if err != nil {
			return nil, err
		}
		y, err := s.deps.Prices.Yekdem(ctx, sc, int16(from.Year()), int16(from.Month()))
		if errors.Is(err, store.ErrNotFound) || len(prices) == 0 {
			continue // Analyse warns base_price_missing
		}
		if err != nil {
			return nil, err
		}
		sum := decimal.Zero
		for _, p := range prices {
			sum = sum.Add(p.PTF)
		}
		avg := sum.DivRound(decimal.NewFromInt(int64(len(prices))), 20)
		out[r.Period] = avg.Add(y.Value).DivRound(thousand, 20)
	}
	return out, nil
}

// periodStart is the Istanbul start of "YYYYMM", zero when malformed.
func (s *Service) periodStart(period string) time.Time {
	t, err := time.ParseInLocation("200601", period, s.loc)
	if err != nil {
		return time.Time{}
	}
	return t
}

// GetImport re-reads a stored import and its analysis.
func (s *Service) GetImport(ctx context.Context, sc store.Scope, id uuid.UUID) (ImportResult, error) {
	imp, err := s.deps.Icmal.GetImport(ctx, sc, id)
	if err != nil {
		return ImportResult{}, err
	}
	res := ImportResult{Import: imp}
	if len(imp.Result) > 0 {
		var stored storedResult
		if err := json.Unmarshal(imp.Result, &stored); err != nil {
			return ImportResult{}, fmt.Errorf("icmal import result: %w", err)
		}
		res.Analyses, res.Unmatched, res.Warnings = stored.Analyses, stored.Unmatched, stored.Warnings
	}
	return res, nil
}

// ApplyConfirmation is one operator-confirmed building for an import (R134).
type ApplyConfirmation struct {
	BuildingID    uuid.UUID
	EtsoCode      string
	EffectiveFrom time.Time
	Base          *Input // required when the building has no applicable tariff at EffectiveFrom
}

// ApplyImport writes one new tariff version per confirmation (R134): a copy of
// the building's applicable tariff (or Base) with the derived PTF coefficients,
// power and reactive prices as fixed sources (R119, R132) and the derived named
// taxes. Every confirmation is validated before anything is written.
func (s *Service) ApplyImport(ctx context.Context, sc store.Scope, importID uuid.UUID, confirmations []ApplyConfirmation) ([]Definition, error) {
	if len(confirmations) == 0 {
		return nil, ErrInvalidRequest
	}
	res, err := s.GetImport(ctx, sc, importID)
	if err != nil {
		return nil, err
	}
	if res.Import.Status != ImportStatusAnalysed {
		return nil, fmt.Errorf("%w: import is %s", ErrInvalidRequest, res.Import.Status)
	}
	inputs := make([]Input, len(confirmations))
	for i, c := range confirmations {
		if c.BuildingID == uuid.Nil || c.EffectiveFrom.IsZero() {
			return nil, fmt.Errorf("%w: confirmation %d needs a building and effective_from", ErrInvalidRequest, i)
		}
		if _, err := s.deps.Buildings.Get(ctx, sc, c.BuildingID); err != nil {
			return nil, err
		}
		idx := slices.IndexFunc(res.Analyses, func(a icmal.Analysis) bool { return a.EtsoCode == c.EtsoCode })
		if idx < 0 {
			return nil, fmt.Errorf("%w: no analysis for ETSO %s", ErrInvalidRequest, c.EtsoCode)
		}
		base, err := s.baseInput(ctx, sc, c)
		if err != nil {
			return nil, err
		}
		in := applyAnalysis(base, res.Analyses[idx])
		in.Tariff.BuildingID = &confirmations[i].BuildingID // I-15: never the copied company-wide row
		in.Tariff.EffectiveFrom = c.EffectiveFrom
		if _, err := validate(in); err != nil {
			return nil, err
		}
		inputs[i] = in
	}
	out := make([]Definition, 0, len(inputs))
	for _, in := range inputs {
		def, err := s.Create(ctx, sc, in)
		if err != nil {
			return nil, err
		}
		out = append(out, def)
	}
	if _, err := s.deps.Icmal.UpdateImportResult(ctx, sc, importID, ImportStatusApplied, res.Import.Result); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) baseInput(ctx context.Context, sc store.Scope, c ApplyConfirmation) (Input, error) {
	def, err := s.Applicable(ctx, sc, c.BuildingID, c.EffectiveFrom)
	if errors.Is(err, store.ErrNotFound) {
		if c.Base == nil {
			return Input{}, fmt.Errorf("%w: building %s has no applicable tariff and no base definition", ErrInvalidRequest, c.BuildingID)
		}
		return *c.Base, nil
	}
	if err != nil {
		return Input{}, err
	}
	vat := def.Tariff.VatRate
	return Input{Tariff: def.Tariff, VatRate: &vat, Taxes: def.Taxes, ExtraCharges: def.ExtraCharges, ManualYekdem: def.ManualYekdem}, nil
}

// applyAnalysis overlays derived values on a base definition.
func applyAnalysis(base Input, a icmal.Analysis) Input {
	in := base
	t := base.Tariff
	t.ID, t.CreatedAt, t.UpdatedAt, t.DeletedAt = uuid.Nil, time.Time{}, time.Time{}, nil
	t.UsePtfYekdem, t.Currency = true, model.CurrencyTRY
	if v := a.EnergyKbk.Value; v != nil {
		t.KbkEnergy = v
	}
	if v := a.DistributionTlPerKwh.Value; v != nil {
		t.DistributionPriceSource, t.KbkDistributionCostTlPerKwh = model.PriceSourceKbk, v
	}
	if v := a.PowerUnitPrice.Value; v != nil {
		t.PowerPriceSource, t.PowerUnitPrice = model.PriceSourceFixed, v
	}
	if v := a.ReactiveUnitPrice.Value; v != nil {
		t.ReactivePriceSource, t.ReactivePowerPrice = model.PriceSourceFixed, *v
	}
	// A price the icmal did not derive stays the base tariff's own column
	// unless the base already carries its KBK coefficient.
	if t.PowerPriceSource != model.PriceSourceFixed && t.KbkPowerPrice == nil {
		t.PowerPriceSource = model.PriceSourceFixed
	}
	if t.ReactivePriceSource != model.PriceSourceFixed && t.KbkReactivePower == nil {
		t.ReactivePriceSource = model.PriceSourceFixed
	}
	if t.DistributionPriceSource != model.PriceSourceFixed && t.KbkDistributionCostTlPerKwh == nil {
		t.DistributionPriceSource = model.PriceSourceFixed
	}
	if v := a.VatRate.Value; v != nil {
		in.VatRate = v
	}
	if len(a.Taxes) > 0 {
		names := make([]string, 0, len(a.Taxes))
		for name := range a.Taxes {
			names = append(names, name)
		}
		slices.Sort(names)
		in.Taxes = nil
		for i, name := range names {
			if v := a.Taxes[name].Value; v != nil {
				in.Taxes = append(in.Taxes, model.TariffTax{Name: name, Rate: *v, SortOrder: int16(i)})
			}
		}
	}
	in.Tariff = t
	return in
}
