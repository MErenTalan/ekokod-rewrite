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
	invoice  domain.Invoice
	members  []uuid.UUID
	building *model.Building
	analyzer *model.Analyzer
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
		subj, err = s.computeSubject(ctx, sc, *building, members, req.PeriodKey)
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
	subj, err := s.computeSubject(ctx, sc, *building, members, req.PeriodKey)
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
	for _, b := range buildings {
		analyzers, err := s.buildingAnalyzers(ctx, sc, b.ID)
		if err != nil {
			return subject{}, err
		}
		if len(analyzers) == 0 {
			continue
		}
		subj, err := s.computeSubject(ctx, sc, b, analyzers, key)
		var ce *ComputeError
		if errors.As(err, &ce) {
			ce.Detail["building_id"] = b.ID.String()
		}
		if err != nil {
			return subject{}, err
		}
		invoices = append(invoices, subj.invoice)
		members = append(members, subj.members...)
	}
	if len(invoices) == 0 {
		return subject{}, computeErr(CodeNoConsumptionData, "company_id", sc.CompanyID.String())
	}
	combined, err := domain.CombineCompany(key, invoices)
	if err != nil {
		return subject{}, computeErr(CodeNoConsumptionData, "reason", err.Error())
	}
	return subject{invoice: combined, members: members}, nil
}

// computeSubject prices one analyzer or building for key (steps 2–8).
func (s *Service) computeSubject(ctx context.Context, sc store.Scope, building model.Building, analyzers []model.Analyzer, key string) (subject, error) {
	window, err := domain.Period(key, int(building.BillCutoffDay), s.loc)
	if err != nil {
		return subject{}, fmt.Errorf("%w: %w", ErrInvalidRequest, err)
	}
	if s.deps.Clock.Now().Before(window.To.Add(consumption.SettleDelayMonthly)) {
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
	for _, chunk := range chunks(ids, maxChunk) {
		got, err := s.deps.Consumption.PeriodConsumptionAndRecord(ctx, sc, consumption.PeriodRequest{AnalyzerIDs: chunk, Windows: []energy.Window{window}})
		if err != nil {
			return subject{}, err
		}
		for _, r := range got {
			rows[r.AnalyzerID] = r
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
	if blocking, err := s.blockingAnomalies(ctx, sc, ids, window); err != nil {
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
	if t.UsePtfYekdem {
		pricing, err := s.price(ctx, sc, t, ids, window, params)
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
	return subject{invoice: inv, members: ids, building: &building}, nil
}

// blockingAnomalies pages every unresolved anomaly that overlaps window (I-5).
func (s *Service) blockingAnomalies(ctx context.Context, sc store.Scope, ids []uuid.UUID, window energy.Window) ([]string, error) {
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

	market := tariff.Market{PTF: map[time.Time]decimal.Decimal{}, Yekdem: map[tariff.YearMonth]decimal.Decimal{}, ManualYekdem: map[tariff.YearMonth]decimal.Decimal{}}
	prices, err := s.deps.Prices.HourlyRange(ctx, sc, store.TimeRange{From: window.From, To: window.To})
	if err != nil {
		return tariff.Pricing{}, err
	}
	for _, p := range prices {
		market.PTF[p.Ts.UTC()] = p.PTF
	}
	for _, ym := range monthsTouched(window, s.loc) {
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
