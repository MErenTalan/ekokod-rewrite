package billing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// GenerateRequest asks for one bill.
type GenerateRequest struct {
	Scope            model.BillScope
	SubjectID        uuid.UUID // analyzer, building, or company id (= sc.CompanyID)
	PeriodKey        string
	Force            bool
	RecomputeFlagged bool // I-8: supersede a flagged live bill, never an issued or draft one
}

// GenerateResult is the live bill after Generate.
type GenerateResult struct {
	Bill       model.Bill
	Created    bool
	Superseded *uuid.UUID
}

const anomalyLookback = 367 * 24 * time.Hour // I-5: widest anomaly period starts a year back

// subject is one computed invoice subject plus what persistence needs.
type subject struct {
	invoice     domain.Invoice
	members     []uuid.UUID
	building    *model.Building
	analyzer    *model.Analyzer
	missingDays []string // days the consumption could not cover (partial bill)
}

// Generate computes and persists a bill (R113, R114, R115).
func (s *Service) Generate(ctx context.Context, sc store.Scope, req GenerateRequest) (GenerateResult, error) {
	if !sc.Valid() || !req.Scope.Valid() || req.SubjectID == uuid.Nil {
		return GenerateResult{}, ErrInvalidRequest
	}
	if _, err := domain.Period(req.PeriodKey, 1, s.loc); err != nil {
		return GenerateResult{}, fmt.Errorf("%w: %w", ErrInvalidRequest, err)
	}
	if req.Scope == model.BillScopeCompany {
		// I-9: a building-restricted Scope must never persist a partial company bill.
		if req.SubjectID != sc.CompanyID || !sc.AllBuildings {
			return GenerateResult{}, ErrInvalidRequest
		}
	}

	var building *model.Building
	var members []model.Analyzer
	switch req.Scope {
	case model.BillScopeAnalyzer:
		a, err := s.deps.Analyzers.Get(ctx, sc, req.SubjectID)
		if err != nil {
			return GenerateResult{}, err
		}
		if a.BuildingID == nil {
			return GenerateResult{}, fmt.Errorf("%w: analyzer has no building", ErrInvalidRequest)
		}
		b, err := s.deps.Buildings.Get(ctx, sc, *a.BuildingID)
		if err != nil {
			return GenerateResult{}, err
		}
		building, members = &b, []model.Analyzer{a}
	case model.BillScopeBuilding:
		b, err := s.deps.Buildings.Get(ctx, sc, req.SubjectID)
		if err != nil {
			return GenerateResult{}, err
		}
		building = &b
		if members, err = s.buildingAnalyzers(ctx, sc, b.ID); err != nil {
			return GenerateResult{}, err
		}
		if len(members) == 0 {
			return GenerateResult{}, computeErr(CodeNoConsumptionData, "building_id", b.ID.String())
		}
	}

	existing, err := s.deps.Bills.Current(ctx, sc, req.Scope, req.SubjectID, req.PeriodKey)
	hasLive := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return GenerateResult{}, err
	}
	supersede := hasLive && (req.Force || (req.RecomputeFlagged && existing.Status == model.BillStatusFlagged))
	if hasLive && !supersede {
		return GenerateResult{Bill: existing}, s.repairHourlyDetail(ctx, sc, existing, building, members, req)
	}

	var subj subject
	if req.Scope == model.BillScopeCompany {
		subj, err = s.computeCompany(ctx, sc, req.PeriodKey)
	} else {
		subj, err = s.computeSubject(ctx, sc, *building, members, req.PeriodKey, req.Scope, req.SubjectID, subjectOpts{})
		if req.Scope == model.BillScopeAnalyzer {
			subj.analyzer = &members[0]
		}
	}
	if err != nil {
		return GenerateResult{}, err
	}
	return s.persist(ctx, sc, req, subj, existing, supersede)
}

func (s *Service) persist(ctx context.Context, sc store.Scope, req GenerateRequest, subj subject, existing model.Bill, supersede bool) (GenerateResult, error) {
	now := s.deps.Clock.Now().UTC()
	bill, lines, hourly := domain.ToModel(subj.invoice, req.Scope)
	bill.CompanyID = sc.CompanyID
	if subj.building != nil {
		bill.BuildingID = &subj.building.ID
	}
	if subj.analyzer != nil {
		bill.AnalyzerID = &subj.analyzer.ID
	}
	bill.ComputedAt, bill.CreatedAt, bill.UpdatedAt = now, now, now
	bill.MissingDays = subj.missingDays

	var saved model.Bill
	var err error
	res := GenerateResult{Created: true}
	if supersede {
		saved, err = s.deps.Bills.Supersede(ctx, sc, existing.ID, bill, lines, subj.members, now)
		res.Superseded = &existing.ID
	} else {
		saved, err = s.deps.Bills.Create(ctx, sc, bill, lines, subj.members)
		if errors.Is(err, store.ErrConflict) { // M-11: a concurrent non-force Generate won
			current, cerr := s.deps.Bills.Current(ctx, sc, req.Scope, req.SubjectID, req.PeriodKey)
			if cerr != nil {
				return GenerateResult{}, cerr
			}
			return GenerateResult{Bill: current}, nil
		}
	}
	if err != nil {
		return GenerateResult{}, err
	}
	res.Bill = saved
	if len(hourly) > 0 {
		for i := range hourly {
			hourly[i].BillID = saved.ID
		}
		if _, err := s.deps.Bills.ReplaceHourlyDetail(ctx, sc, saved.ID, hourly); err != nil {
			return GenerateResult{}, err
		}
	}
	if err := s.notify(ctx, sc, saved, subj); err != nil {
		return GenerateResult{}, err
	}
	return res, nil
}

// notify appends the operator messages for a flagged bill and for a
// multi-analyzer binomial building bill without a coincident peak (M-12).
func (s *Service) notify(ctx context.Context, sc store.Scope, b model.Bill, subj subject) error {
	msg := func(status, text string) error {
		related := "bill"
		_, err := s.deps.Ops.AppendMessage(ctx, sc, model.OperationalMessage{
			CompanyID: &sc.CompanyID, Kind: "system", Category: "bill-generation", Status: status,
			Message: text, RelatedType: &related, RelatedID: &b.ID,
		})
		return err
	}
	if b.Status == model.BillStatusFlagged && b.FlagReason != nil {
		if err := msg("warning", fmt.Sprintf("bill %s for %s %s flagged: %s", b.ID, b.Scope, b.PeriodKey, *b.FlagReason)); err != nil {
			return err
		}
	}
	if b.Scope == model.BillScopeBuilding && len(subj.members) > 1 && b.PowerCost.IsPositive() && !b.DemandDataAvailable {
		return msg("info", fmt.Sprintf("bill %s: no demand overrun billed — a building's coincident peak cannot be built from per-meter maxima (R111)", b.ID))
	}
	return nil
}

// repairHourlyDetail re-writes a live PTF bill's missing hourly rows (M-9).
func (s *Service) repairHourlyDetail(ctx context.Context, sc store.Scope, live model.Bill, building *model.Building, members []model.Analyzer, req GenerateRequest) error {
	if req.Scope == model.BillScopeCompany || !live.PtfYekdemUsed || live.PtfHoursMatched == nil || *live.PtfHoursMatched == 0 {
		return nil
	}
	have, err := s.deps.Bills.HourlyDetail(ctx, sc, live.ID)
	if err != nil || len(have) > 0 {
		return err
	}
	subj, err := s.computeSubject(ctx, sc, *building, members, req.PeriodKey, req.Scope, req.SubjectID, subjectOpts{})
	if err != nil {
		return err
	}
	_, _, hourly := domain.ToModel(subj.invoice, req.Scope)
	if len(hourly) == 0 {
		return nil
	}
	for i := range hourly {
		hourly[i].BillID = live.ID
	}
	_, err = s.deps.Bills.ReplaceHourlyDetail(ctx, sc, live.ID, hourly)
	return err
}

// computeCompany computes every building in memory with its own tariff and
// window and combines them (R115); analyzer-less buildings are skipped (I-9).
func (s *Service) computeCompany(ctx context.Context, sc store.Scope, key string) (subject, error) {
	buildings, err := listAll(func(p store.Page) ([]model.Building, error) {
		return s.deps.Buildings.List(ctx, sc, store.BuildingFilter{Page: p})
	})
	if err != nil {
		return subject{}, err
	}
	var invoices []domain.Invoice
	var members []uuid.UUID
	missing := map[string]bool{}
	for _, b := range buildings {
		analyzers, err := s.buildingAnalyzers(ctx, sc, b.ID)
		if err != nil {
			return subject{}, err
		}
		if len(analyzers) == 0 {
			continue
		}
		subj, err := s.computeSubject(ctx, sc, b, analyzers, key, model.BillScopeBuilding, b.ID, subjectOpts{})
		var ce *ComputeError
		if errors.As(err, &ce) {
			ce.Detail["building_id"] = b.ID.String()
		}
		if err != nil {
			return subject{}, err
		}
		invoices = append(invoices, subj.invoice)
		members = append(members, subj.members...)
		for _, d := range subj.missingDays {
			missing[d] = true
		}
	}
	if len(invoices) == 0 {
		return subject{}, computeErr(CodeNoConsumptionData, "company_id", sc.CompanyID.String())
	}
	combined, err := domain.CombineCompany(key, invoices)
	if err != nil {
		return subject{}, computeErr(CodeNoConsumptionData, "reason", err.Error())
	}
	return subject{invoice: combined, members: members, missingDays: sortedKeys(missing)}, nil
}

// computeSubject prices one analyzer or building for key (steps 2–8);
// billScope and subjectID name whose bill history decides R529's waiver.
func (s *Service) computeSubject(ctx context.Context, sc store.Scope, building model.Building, analyzers []model.Analyzer, key string, billScope model.BillScope, subjectID uuid.UUID, opt subjectOpts) (subject, error) {
	window, err := s.windowFor(ctx, sc, building, key, opt)
	if err != nil {
		return subject{}, err
	}
	now := s.deps.Clock.Now()
	if !now.After(window.From) {
		return subject{}, computeErr(CodePeriodNotClosed, "period_end", window.To.UTC().Format(time.RFC3339))
	}
	ids := make([]uuid.UUID, len(analyzers))
	for i, a := range analyzers {
		ids[i] = a.ID
	}

	t, err := s.deps.Tariffs.Effective(ctx, sc, building.ID, window.From)
	if errors.Is(err, store.ErrNotFound) {
		return subject{}, computeErr(CodeTariffNotFound, "building_id", building.ID.String())
	}
	if err != nil {
		return subject{}, err
	}
	params, err := s.deps.Params.Effective(ctx, sc, window.From)
	if errors.Is(err, store.ErrNotFound) {
		return subject{}, computeErr(CodeBillingParametersMissing)
	}
	if err != nil {
		return subject{}, err
	}
	if err := tariff.ValidateParams(params); err != nil {
		return subject{}, computeErr(CodeBillingParametersInvalid, "reason", err.Error())
	}

	rows := map[uuid.UUID]consumption.Row{}
	if opt.preview || !now.Before(window.To.Add(consumption.SettleDelayMonthly)) {
		for _, chunk := range chunks(ids, maxChunk) {
			req := consumption.PeriodRequest{AnalyzerIDs: chunk, Windows: []energy.Window{window}}
			read := s.deps.Consumption.PeriodConsumptionAndRecord
			if opt.preview { // R559: a preview records nothing
				req.Open, read = true, s.deps.Consumption.PeriodConsumption
			}
			got, err := read(ctx, sc, req)
			if err != nil {
				return subject{}, err
			}
			for _, r := range got {
				if billable(r) {
					rows[r.AnalyzerID] = r
				}
			}
		}
	}
	// An open period or a missing boundary no longer blocks the bill: it
	// covers the days that have data and lists the rest.
	billed, partial := window, len(rows) < len(ids)
	var missingDays []string
	if partial {
		if billed, missingDays, rows, err = s.partialRows(ctx, sc, ids, window, now); err != nil {
			return subject{}, err
		}
	}
	var quantities []domain.Quantities
	var suspect []string
	for _, id := range ids {
		r, ok := rows[id]
		if !ok {
			return subject{}, computeErr(CodeNoConsumptionData, "analyzer_id", id.String())
		}
		if len(r.Suspect) > 0 {
			suspect = append(suspect, id.String())
		}
		quantities = append(quantities, quantitiesFromRow(r))
	}
	if len(suspect) > 0 {
		return subject{}, computeErr(CodeUnresolvedAnomaly, "analyzer_ids", strings.Join(suspect, ","))
	}
	if blocking, err := s.blockingAnomalies(ctx, sc, ids, billed, partial); err != nil {
		return subject{}, err
	} else if len(blocking) > 0 {
		return subject{}, computeErr(CodeUnresolvedAnomaly, "anomaly_ids", strings.Join(blocking, ","))
	}

	taxes, err := s.deps.Tariffs.Taxes(ctx, sc, t.ID)
	if err != nil {
		return subject{}, err
	}
	extras, err := s.deps.Tariffs.ExtraCharges(ctx, sc, t.ID)
	if err != nil {
		return subject{}, err
	}
	installed := make([]*decimal.Decimal, len(analyzers))
	for i, a := range analyzers {
		installed[i] = a.InstalledPowerKw
	}
	in := domain.Input{
		PeriodKey: key, Period: window, Days: domain.DaysInPeriod(window, s.loc),
		Tariff: t, Taxes: taxes, ExtraCharges: extras, Params: params,
		Quantities: domain.AggregateQuantities(quantities), InstalledPowerKw: domain.SumInstalledPower(installed),
	}
	if params.ReactiveFirstViolationWaived && !t.ReactiveWaiverDisabled {
		if in.ReactivePriorViolation, err = s.deps.Bills.ReactiveViolationBefore(ctx, sc, billScope, subjectID, key); err != nil {
			return subject{}, err
		}
	}
	if t.UsePtfYekdem {
		pricing, err := s.price(ctx, sc, t, ids, billed, params)
		if err != nil {
			return subject{}, err
		}
		in.Pricing = &pricing
	}
	inv, err := domain.Compute(in)
	if errors.Is(err, domain.ErrInvalidInput) { // R113: a nil required register
		return subject{}, computeErr(CodeNoConsumptionData, "reason", err.Error())
	}
	if err != nil {
		return subject{}, err
	}
	return subject{invoice: inv, members: ids, building: &building, missingDays: missingDays}, nil
}

// subjectOpts: a preview prices an explicit window and records nothing (R559).
type subjectOpts struct {
	window  *energy.Window
	preview bool
}

// windowFor is opt's explicit window, else key's resolved window (R556).
func (s *Service) windowFor(ctx context.Context, sc store.Scope, building model.Building, key string, opt subjectOpts) (energy.Window, error) {
	if opt.window != nil {
		return *opt.window, nil
	}
	saved, err := s.savedWindows(ctx, sc, building.ID, key)
	if err != nil {
		return energy.Window{}, err
	}
	prev, next := domain.ShiftKey(key, -1), domain.ShiftKey(key, 1)
	w, err := domain.ResolvePeriod(key, int(building.BillCutoffDay), saved[prev], saved[key], saved[next], s.loc)
	switch {
	case errors.Is(err, domain.ErrPeriodConflict):
		return energy.Window{}, computeErr(CodePeriodConflict, "period_key", key)
	case err != nil:
		return energy.Window{}, fmt.Errorf("%w: %w", ErrInvalidRequest, err)
	}
	return w, nil
}

// savedWindows maps key and its neighbours to the building's saved windows.
func (s *Service) savedWindows(ctx context.Context, sc store.Scope, buildingID uuid.UUID, key string) (map[string]*energy.Window, error) {
	periods, err := s.deps.Bills.BillPeriods(ctx, sc, buildingID, []string{domain.ShiftKey(key, -1), key, domain.ShiftKey(key, 1)})
	if err != nil {
		return nil, err
	}
	out := make(map[string]*energy.Window, len(periods))
	for _, p := range periods {
		out[p.PeriodKey] = &energy.Window{From: p.PeriodStart.In(s.loc), To: p.PeriodEnd.In(s.loc)}
	}
	return out, nil
}

// partialRows bills the span from the first to the last day every analyzer
// can difference; days not covered (future ones included) are returned as
// missing, in order.
func (s *Service) partialRows(ctx context.Context, sc store.Scope, ids []uuid.UUID, window energy.Window, now time.Time) (energy.Window, []string, map[uuid.UUID]consumption.Row, error) {
	var days []energy.Window
	for d := window.From.In(s.loc); d.Before(window.To); d = d.AddDate(0, 0, 1) {
		end := d.AddDate(0, 0, 1)
		if end.After(window.To) { // R558: an hour-bound period ends inside its last day
			end = window.To
		}
		days = append(days, energy.Window{From: d, To: end})
	}
	index := make(map[int64]int, len(days))
	var probe []energy.Window
	for i, d := range days {
		index[d.From.UnixNano()] = i
		if !d.To.After(now) { // a day still running is never "covered"
			probe = append(probe, d)
		}
	}
	covered := make([]int, len(days))
	perChunk := max(1, min(maxChunk, consumption.MaxCells/consumption.MaxPeriodWindows))
	for i := 0; i < len(probe); i += consumption.MaxPeriodWindows {
		batch := probe[i:min(len(probe), i+consumption.MaxPeriodWindows)]
		for _, chunk := range chunks(ids, perChunk) {
			got, err := s.deps.Consumption.PeriodConsumption(ctx, sc, consumption.PeriodRequest{AnalyzerIDs: chunk, Windows: batch, Open: true})
			if err != nil {
				return energy.Window{}, nil, nil, err
			}
			for _, r := range got {
				if j, ok := index[r.Window.From.UnixNano()]; ok && billable(r) {
					covered[j]++
				}
			}
		}
	}
	first, last := -1, -1
	var missing []string
	for i, d := range days {
		if covered[i] == len(ids) {
			if first < 0 {
				first = i
			}
			last = i
			continue
		}
		missing = append(missing, d.From.Format(time.DateOnly))
	}
	if first < 0 {
		return energy.Window{}, nil, nil, computeErr(CodeNoConsumptionData, "reason", "no day of the period has readings")
	}
	billed := energy.Window{From: days[first].From, To: days[last].To}
	rows := map[uuid.UUID]consumption.Row{}
	for _, chunk := range chunks(ids, maxChunk) {
		got, err := s.deps.Consumption.PeriodConsumption(ctx, sc, consumption.PeriodRequest{AnalyzerIDs: chunk, Windows: []energy.Window{billed}, Open: true})
		if err != nil {
			return energy.Window{}, nil, nil, err
		}
		for _, r := range got {
			rows[r.AnalyzerID] = r
		}
	}
	return billed, missing, rows, nil
}

// billable reports whether a row can be invoiced: its boundary readings both
// carry the active index (R501). A reading the gateway delivered without the
// meter's registers differences to nil, and that day is missing, not billed.
// A suspect row stays: it must block the bill, not shrink it.
func billable(r consumption.Row) bool {
	return r.Values[energy.ActiveImport] != nil || len(r.Suspect) > 0
}

// blockingAnomalies pages every unresolved anomaly that overlaps window (I-5).
// A partial bill already lists its missing days, so missing_readings does not block it.
func (s *Service) blockingAnomalies(ctx context.Context, sc store.Scope, ids []uuid.UUID, window energy.Window, partial bool) ([]string, error) {
	var out []string
	for _, chunk := range chunks(ids, maxChunk) {
		anomalies, err := listAll(func(p store.Page) ([]model.ConsumptionAnomaly, error) {
			return s.deps.Anomalies.List(ctx, sc, store.AnomalyFilter{
				AnalyzerIDs: chunk, Unresolved: true, Page: p,
				Range: &store.TimeRange{From: window.From.Add(-anomalyLookback), To: window.To},
			})
		})
		if err != nil {
			return nil, err
		}
		for _, a := range anomalies {
			if partial && a.Reason == string(energy.ReasonMissingReadings) {
				continue
			}
			if overlaps(a, window) {
				out = append(out, a.ID.String())
			}
		}
	}
	return out, nil
}

// price loads hourly consumption and market data and runs tariff.Price (R108, R124).
func (s *Service) price(ctx context.Context, sc store.Scope, t model.Tariff, ids []uuid.UUID, window energy.Window, params model.BillingParameters) (tariff.Pricing, error) {
	hoursInWindow := int(window.To.Sub(window.From) / time.Hour)
	var series [][]tariff.HourConsumption
	hourlyAvailable := false
	if t.PriceType == model.PriceTypeSingleTime {
		perChunk := max(1, min(maxChunk, consumption.MaxCells/max(1, hoursInWindow)))
		byAnalyzer := map[uuid.UUID][]consumption.Row{}
		for _, chunk := range chunks(ids, perChunk) {
			rows, err := s.deps.Consumption.Consumption(ctx, sc, consumption.SeriesRequest{
				AnalyzerIDs: chunk, Level: energy.Hourly, Range: store.TimeRange{From: window.From, To: window.To},
			})
			if err != nil {
				return tariff.Pricing{}, err
			}
			for _, r := range rows {
				byAnalyzer[r.AnalyzerID] = append(byAnalyzer[r.AnalyzerID], r)
			}
		}
		hourlyAvailable = len(byAnalyzer) > 0
		for _, id := range ids {
			series = append(series, soundHours(byAnalyzer[id]))
		}
	}
	hours, _ := domain.AggregateHours(series)

	market := tariff.Market{PTF: map[time.Time]decimal.Decimal{}, Yekdem: map[tariff.YearMonth]decimal.Decimal{}, ManualYekdem: map[tariff.YearMonth]decimal.Decimal{},
		Quantity: map[time.Time]decimal.Decimal{}, YekdemForecast: map[tariff.YearMonth]decimal.Decimal{}}
	// R567: the SKTT method weighs whole read dates, so load both end days fully.
	priceRange := store.TimeRange{From: dayStart(window.From, s.loc), To: dayStart(window.To, s.loc).AddDate(0, 0, 1)}
	prices, err := s.deps.Prices.HourlyRange(ctx, sc, priceRange)
	if err != nil {
		return tariff.Pricing{}, err
	}
	for _, p := range prices {
		market.PTF[p.Ts.UTC()] = p.PTF
		if p.MatchedQuantity != nil {
			market.Quantity[p.Ts.UTC()] = *p.MatchedQuantity
		}
	}
	for _, ym := range monthsTouched(energy.Window{From: priceRange.From, To: priceRange.To}, s.loc) {
		f, err := s.deps.Prices.YekdemForecast(ctx, sc, int16(ym.Year), int16(ym.Month))
		switch {
		case err == nil:
			market.YekdemForecast[ym] = f.Value
		case !errors.Is(err, store.ErrNotFound):
			return tariff.Pricing{}, err
		}
	}
	for _, ym := range monthsTouched(energy.Window{From: priceRange.From, To: priceRange.To}, s.loc) {
		y, err := s.deps.Prices.Yekdem(ctx, sc, int16(ym.Year), int16(ym.Month))
		if errors.Is(err, store.ErrNotFound) {
			continue // R109: flagged by Price, never substituted
		}
		if err != nil {
			return tariff.Pricing{}, err
		}
		market.Yekdem[ym] = y.Value
	}
	if t.UseManualYekdem {
		manual, err := s.deps.Tariffs.ManualYekdem(ctx, sc, t.ID)
		if err != nil {
			return tariff.Pricing{}, err
		}
		for _, m := range manual {
			market.ManualYekdem[tariff.YearMonth{Year: int(m.Year), Month: time.Month(m.Month)}] = m.Value
		}
	}
	return tariff.Price(t, window, hours, hourlyAvailable, market, params, s.loc)
}

func (s *Service) buildingAnalyzers(ctx context.Context, sc store.Scope, buildingID uuid.UUID) ([]model.Analyzer, error) {
	return listAll(func(p store.Page) ([]model.Analyzer, error) {
		return s.deps.Analyzers.List(ctx, sc, store.AnalyzerFilter{BuildingID: &buildingID, Page: p})
	})
}

const pageSize = 500

// listAll pages a repository listing to exhaustion.
func listAll[T any](list func(store.Page) ([]T, error)) ([]T, error) {
	var out []T
	for offset := int32(0); ; offset += pageSize {
		page, err := list(store.Page{Limit: pageSize, Offset: offset})
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(page) < pageSize {
			return out, nil
		}
	}
}

// dayStart is t's Istanbul midnight.
func dayStart(t time.Time, loc *time.Location) time.Time {
	lt := t.In(loc)
	return time.Date(lt.Year(), lt.Month(), lt.Day(), 0, 0, 0, 0, loc)
}
