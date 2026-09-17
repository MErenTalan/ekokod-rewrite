package analysis

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/reactive"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// ReactiveAnalyzer is one analyzer's reactive position for the month (R166).
type ReactiveAnalyzer struct {
	AnalyzerID                      uuid.UUID
	BuildingID                      *uuid.UUID
	HasData                         bool
	InductiveRatio, CapacitiveRatio *decimal.Decimal
	InductiveLimit, CapacitiveLimit *decimal.Decimal
	InstalledPowerKw                *decimal.Decimal
	ExemptReason                    string
	PenaltyApplies                  bool
}

// ReactiveExtreme names the analyzer with the highest ratio of one kind.
type ReactiveExtreme struct {
	AnalyzerID uuid.UUID
	BuildingID *uuid.UUID
	Ratio      decimal.Decimal
}

// ReactiveStatus is GET /consumption/reactive-status.
type ReactiveStatus struct {
	Month                               string
	Analyzers                           []ReactiveAnalyzer
	HighestInductive, HighestCapacitive *ReactiveExtreme
}

// ReactiveStatus evaluates every analyzer in scope (or in one building) for a
// month with 02 §6.6's rules; month zero means the current Istanbul month.
func (s *Service) ReactiveStatus(ctx context.Context, sc store.Scope, month time.Time, buildingID *uuid.UUID) (ReactiveStatus, error) {
	loc := s.d.Location
	if month.IsZero() {
		now := s.d.Clock.Now().In(loc)
		month = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	}
	start := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 1, 0)
	out := ReactiveStatus{Month: start.Format("2006-01"), Analyzers: []ReactiveAnalyzer{}}

	if buildingID != nil {
		if _, err := s.d.Buildings.Get(ctx, sc, *buildingID); err != nil {
			return ReactiveStatus{}, err
		}
	}
	var analyzers []model.Analyzer
	for offset := int32(0); ; offset += 500 {
		page, err := s.d.Analyzers.List(ctx, sc, store.AnalyzerFilter{BuildingID: buildingID, Page: store.Page{Limit: 500, Offset: offset}})
		if err != nil {
			return ReactiveStatus{}, err
		}
		analyzers = append(analyzers, page...)
		if len(page) < 500 {
			break
		}
	}
	if len(analyzers) == 0 {
		return out, nil
	}
	params, err := s.d.Params.Effective(ctx, sc, start)
	if errors.Is(err, store.ErrNotFound) {
		return ReactiveStatus{}, ErrBillingParametersAbsent
	}
	if err != nil {
		return ReactiveStatus{}, err
	}
	rows := map[uuid.UUID]consumption.Row{}
	for i := 0; i < len(analyzers); i += consumption.MaxAnalyzersPerRequest {
		chunk := analyzers[i:min(i+consumption.MaxAnalyzersPerRequest, len(analyzers))]
		ids := make([]uuid.UUID, len(chunk))
		for j, a := range chunk {
			ids[j] = a.ID
		}
		got, err := s.d.Series.Consumption(ctx, sc, consumption.SeriesRequest{AnalyzerIDs: ids, Level: energy.Monthly, Range: store.TimeRange{From: start, To: end}})
		if err != nil {
			return ReactiveStatus{}, mapErr(err)
		}
		for _, r := range got {
			rows[r.AnalyzerID] = r
		}
	}
	for _, a := range analyzers {
		view := ReactiveAnalyzer{AnalyzerID: a.ID, BuildingID: a.BuildingID, InstalledPowerKw: a.InstalledPowerKw}
		row, ok := rows[a.ID]
		active := sound(row, energy.ActiveImport)
		if !ok || active == nil {
			out.Analyzers = append(out.Analyzers, view)
			continue
		}
		view.HasData = true
		net := *active
		export := sound(row, energy.ActiveExport)
		known := decimal.Zero
		if export != nil {
			net = decimal.Max(net.Sub(*export), decimal.Zero)
			known = *export
		}
		in := reactive.Input{
			NetConsumption: net, Inductive: sound(row, energy.ReactiveInductiveImport), Capacitive: sound(row, energy.ReactiveCapacitiveImport),
			ActiveExport: export, ActiveExportKnownSum: known, InstalledPowerKw: a.InstalledPowerKw, Params: params,
		}
		if a.BuildingID != nil {
			if t, err := s.d.Tariffs.Effective(ctx, sc, *a.BuildingID, start); err == nil {
				in.Term, in.UserGroup = t.Term, t.UserGroup
			} else if !errors.Is(err, store.ErrNotFound) {
				return ReactiveStatus{}, err
			}
		}
		res := reactive.Evaluate(in)
		view.InductiveRatio, view.CapacitiveRatio = res.InductiveRatio, res.CapacitiveRatio
		view.ExemptReason, view.PenaltyApplies = res.ExemptReason, res.Applied
		if res.Exempt {
			if band, ok := bandFor(params.ReactiveBands, a.InstalledPowerKw); ok {
				view.InductiveLimit, view.CapacitiveLimit = &band.Inductive, &band.Capacitive
			}
		} else {
			view.InductiveLimit, view.CapacitiveLimit = res.InductiveLimit, res.CapacitiveLimit
		}
		out.Analyzers = append(out.Analyzers, view)
		out.HighestInductive = higher(out.HighestInductive, view, view.InductiveRatio)
		out.HighestCapacitive = higher(out.HighestCapacitive, view, view.CapacitiveRatio)
	}
	return out, nil
}

func sound(r consumption.Row, reg energy.Register) *decimal.Decimal {
	if _, suspect := r.Suspect[reg]; suspect {
		return nil
	}
	return r.Values[reg]
}

func bandFor(bands []model.ReactiveBand, kw *decimal.Decimal) (model.ReactiveBand, bool) {
	if kw == nil {
		return model.ReactiveBand{}, false
	}
	return reactive.FindBand(bands, *kw)
}

func higher(current *ReactiveExtreme, v ReactiveAnalyzer, ratio *decimal.Decimal) *ReactiveExtreme {
	if ratio == nil || (current != nil && !ratio.GreaterThan(current.Ratio)) {
		return current
	}
	return &ReactiveExtreme{AnalyzerID: v.AnalyzerID, BuildingID: v.BuildingID, Ratio: *ratio}
}
