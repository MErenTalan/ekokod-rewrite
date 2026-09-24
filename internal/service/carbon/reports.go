package carbon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/carbon"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/render/carbonview"
	"github.com/MErenTalan/ekokod-rewrite/internal/render/reportpdf"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// ReportInput is R312's body.
type ReportInput struct {
	BuildingID uuid.UUID
	Type       string // ghg | iso
	From, To   time.Time
	Name       *string
}

const maxReportName = 120

var reportTitles = map[string]string{"ghg": "GHG Protocol", "iso": "ISO 14064"}

// CreateReport computes and stores R312's snapshot.
func (s *Service) CreateReport(ctx context.Context, sc store.Scope, in ReportInput) (model.CarbonReport, error) {
	title, ok := reportTitles[in.Type]
	if !ok {
		return model.CarbonReport{}, validation("report_type", "invalid")
	}
	from, to := civil(in.From), civil(in.To)
	switch {
	case to.Before(from):
		return model.CarbonReport{}, validation("to", "invalid_range")
	case int(to.Sub(from).Hours()/24)+1 > maxSpanDays:
		return model.CarbonReport{}, validation("to", "range_too_long")
	case to.After(s.today()):
		return model.CarbonReport{}, validation("to", "future")
	}
	period := from.Format(time.DateOnly) + " – " + to.Format(time.DateOnly)
	name := title + " " + period
	if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
		name = strings.TrimSpace(*in.Name)
	}
	if len([]rune(name)) > maxReportName {
		return model.CarbonReport{}, validation("name", "too_long")
	}
	building, err := s.d.Buildings.Get(ctx, sc, in.BuildingID)
	if err != nil {
		return model.CarbonReport{}, err
	}
	company, err := s.d.Companies.Get(ctx, sc, sc.CompanyID)
	if err != nil {
		return model.CarbonReport{}, err
	}
	acts, err := s.allActivities(ctx, sc, in.BuildingID, from, to, nil)
	if err != nil {
		return model.CarbonReport{}, err
	}
	records := make([]domain.Record, len(acts))
	for i, a := range acts {
		records[i] = record(a)
	}
	groups, total, pending := domain.Groups(records, in.Type, from, to)
	p := domain.ReportPayload{ReportType: in.Type, Company: company.Name, Building: building.Name, Address: building.Address,
		From: from.Format(time.DateOnly), To: to.Format(time.DateOnly), GeneratedAt: s.d.Clock.Now().UTC(),
		TotalKgCO2e: total.Round(6), Pending: pending}
	for _, g := range groups {
		pg := domain.PayloadGroup{Key: g.Key, TotalKgCO2e: g.Total.Round(6), Lines: []domain.PayloadLine{}}
		for _, l := range g.Lines {
			pg.Lines = append(pg.Lines, domain.PayloadLine{Sub: l.Sub, KgCO2e: l.KgCO2e.Round(6)})
		}
		p.Groups = append(p.Groups, pg)
	}
	grid, err := s.GridFactor(ctx, sc)
	if err != nil {
		return model.CarbonReport{}, err
	}
	if grid != nil {
		if f := domain.Report94(records, from, to, grid.BaseFactor); f != nil {
			p.Figures94 = &domain.Payload94{ConsumptionKwh: f.ConsumptionKwh.Round(6), GenerationKwh: f.GenerationKwh.Round(6),
				GridFactor: grid.BaseFactor, GridFactorUnit: "kg CO2e/" + grid.BaseUnit, GridFactorSource: grid.Source,
				GridFactorYear: grid.SourceYear, ConsumptionT: f.ConsumptionT.Round(6), ProductionReductT: f.ReductionT.Round(6),
				NetT: f.NetT.Round(6)}
		}
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return model.CarbonReport{}, err
	}
	return s.d.Carbon.CreateReport(ctx, sc, model.CarbonReport{CompanyID: sc.CompanyID, BuildingID: in.BuildingID, Name: name,
		ReportType: in.Type, Period: from.Format(time.DateOnly) + "/" + to.Format(time.DateOnly), Payload: raw})
}

// Reports is the history; a named building outside the scope is 404.
func (s *Service) Reports(ctx context.Context, sc store.Scope, buildingID *uuid.UUID, p store.Page) ([]model.CarbonReport, error) {
	if buildingID != nil {
		if _, err := s.d.Buildings.Get(ctx, sc, *buildingID); err != nil {
			return nil, err
		}
	}
	return s.d.Carbon.ListReports(ctx, sc, buildingID, p)
}

// ReportPDF renders the stored snapshot (Q-F6); the figures never change.
func (s *Service) ReportPDF(ctx context.Context, sc store.Scope, id uuid.UUID, locale string) ([]byte, string, error) {
	r, err := s.d.Carbon.Report(ctx, sc, id)
	if err != nil {
		return nil, "", err
	}
	var p domain.ReportPayload
	if err := json.Unmarshal(r.Payload, &p); err != nil {
		return nil, "", fmt.Errorf("carbon report %s payload: %w", id, err)
	}
	view := carbonview.Build(p, locale)
	body, err := reportpdf.RenderView(view, view.Title+" "+p.From+" – "+p.To, p.GeneratedAt)
	if err != nil {
		return nil, "", err
	}
	return body, fmt.Sprintf("carbon-%s-%s-%s.pdf", p.ReportType, p.From, p.To), nil
}
