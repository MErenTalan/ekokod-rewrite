package v1

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/kit"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/carbon"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	carbonsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/carbon"
)

// carbonRoutes is 05 §12 (R300–R319): reads are every scope, writes A CA (R314).
func carbonRoutes() []Route {
	all, aca := auth.AllRoles, auth.Roles(roleA, roleCA)
	get := func(pattern, id, summary string, req, resp any, h func(*Handlers, http.ResponseWriter, *http.Request)) Route {
		return Route{Method: http.MethodGet, Pattern: pattern, OperationID: id, Tag: "carbon", Access: RoleGated, Roles: all,
			Summary: summary, Request: req, Response: resp, Status: http.StatusOK, Handler: h}
	}
	write := func(method, pattern, id, summary string, status int, req, resp any, h func(*Handlers, http.ResponseWriter, *http.Request)) Route {
		return Route{Method: method, Pattern: pattern, OperationID: id, Tag: "carbon", Access: RoleGated, Roles: aca,
			Entity: "carbon", Summary: summary, Request: req, Response: resp, Status: status, Handler: h}
	}
	return []Route{
		get("/carbon/overview", "carbon.overview", "A building's year: totals, categories, scopes, months against the year before, recent records.",
			dto.CarbonOverviewRequest{}, dto.CarbonOverview{}, (*Handlers).carbonOverview),
		get("/carbon/activity-catalogue", "carbon.activity_catalogue", "The main/sub-category tree with the derived GHG scope and ISO 14064 category.",
			nil, dto.CarbonCatalogue{}, (*Handlers).carbonCatalogue),
		get("/carbon/selected-activities", "carbon.selected_activities", "The building's declared sub-categories.",
			dto.CarbonBuildingQuery{}, dto.CarbonSelection{}, (*Handlers).carbonSelected),
		write(http.MethodPut, "/carbon/selected-activities", "carbon.selected_activities.put", "Replace the building's declared sub-categories.",
			http.StatusOK, dto.CarbonSelectionRequest{}, dto.CarbonSelection{}, (*Handlers).carbonSetSelected),
		get("/carbon/activities", "carbon.activities", "Recorded activities; from/to select overlap.",
			dto.CarbonActivitiesRequest{}, dto.Page[dto.CarbonActivity]{}, (*Handlers).carbonActivities),
		write(http.MethodPost, "/carbon/activities", "carbon.activities.create", "Record an activity; the emission is computed here.",
			http.StatusCreated, dto.CarbonActivityCreateRequest{}, dto.CarbonActivity{}, (*Handlers).carbonCreateActivity),
		write(http.MethodPatch, "/carbon/activities/{id}", "carbon.activities.update", "Edit a manual activity; recomputed and back to pending.",
			http.StatusOK, dto.CarbonActivityUpdateRequest{}, dto.CarbonActivity{}, (*Handlers).carbonUpdateActivity),
		write(http.MethodDelete, "/carbon/activities/{id}", "carbon.activities.delete", "Delete a manual activity.",
			http.StatusNoContent, dto.IDPath{}, nil, (*Handlers).carbonDeleteActivity),
		write(http.MethodPost, "/carbon/activities/{id}/status", "carbon.activities.status", "Approve or reject an activity.",
			http.StatusOK, dto.CarbonStatusRequest{}, dto.CarbonActivity{}, (*Handlers).carbonSetStatus),
		get("/carbon/emission-factors", "carbon.emission_factors", "The company's effective factor catalogue with conversions.",
			dto.EmissionFactorsRequest{}, dto.EmissionFactorList{}, (*Handlers).carbonFactors),
		write(http.MethodPatch, "/carbon/emission-factors/{id}", "carbon.emission_factors.override", "Override a factor for this company.",
			http.StatusOK, dto.EmissionFactorOverrideRequest{}, dto.EmissionFactorView{}, (*Handlers).carbonOverrideFactor),
		write(http.MethodPost, "/carbon/emission-factors/reset", "carbon.emission_factors.reset", "Drop every company override.",
			http.StatusNoContent, nil, nil, (*Handlers).carbonResetFactors),
		get("/carbon/reports", "carbon.reports", "Report history.",
			dto.CarbonReportsRequest{}, dto.Page[dto.CarbonReportSummary]{}, (*Handlers).carbonReports),
		write(http.MethodPost, "/carbon/reports", "carbon.reports.create", "Generate a GHG Protocol or ISO 14064 report for a period.",
			http.StatusCreated, dto.CarbonReportRequest{}, dto.CarbonReportSummary{}, (*Handlers).carbonCreateReport),
		{Method: http.MethodGet, Pattern: "/carbon/reports/{id}/pdf", OperationID: "carbon.reports.pdf", Tag: "carbon", Access: RoleGated,
			Roles: all, Summary: "The report as PDF.", Request: dto.IDPath{}, Status: http.StatusOK, RawContentType: "application/pdf",
			Handler: (*Handlers).carbonReportPDF},
	}
}

func carbonActivityDTO(a model.CarbonActivity) dto.CarbonActivity {
	details := map[string]string{}
	_ = json.Unmarshal(a.Details, &details) // a malformed or non-string detail is shown as absent
	return dto.CarbonActivity{ID: a.ID, BuildingID: a.BuildingID, MainCategory: a.MainCategory, SubCategory: a.SubCategory,
		ActivityType: a.ActivityType, PeriodStart: dto.Date{Time: a.PeriodStart}, PeriodEnd: dto.Date{Time: a.PeriodEnd},
		Quantity: dto.D(a.Quantity), Unit: a.Unit, FactorID: a.FactorID, FactorKey: a.FactorKey, FactorValue: dto.DP(a.FactorValue),
		ConversionMultiplier: dto.D(a.ConversionMultiplier), EmissionKgCO2e: dto.D(a.EmissionKgco2e), Scope: string(a.Scope),
		IsoCategory: a.IsoCategory, Description: a.Description, Details: details, Status: string(a.Status),
		IsAutomated: a.IsAutomated, CreatedBy: a.CreatedBy, CreatedAt: dto.T(a.CreatedAt), UpdatedAt: dto.T(a.UpdatedAt)}
}

func (h *Handlers) carbonOverview(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.CarbonOverviewRequest) (any, error) {
		year := time.Now().In(dto.Istanbul).Year()
		if req.Year != nil {
			year = *req.Year
		}
		o, err := h.Carbon.Overview(r.Context(), mw.ScopeFrom(r), req.BuildingID, year)
		if err != nil {
			return nil, err
		}
		out := dto.CarbonOverview{Year: o.Year, TotalKgCO2e: dto.D(o.Total.Round(6)), ActivityCount: o.ActivityCount,
			RegisteredCount: o.RegisteredCount, PendingCount: o.PendingCount, ByCategory: []dto.CarbonAmount{},
			ByScope: []dto.CarbonAmount{}, Monthly: make([]dto.CarbonMonth, 12), Recent: []dto.CarbonActivity{}}
		if o.Highest != nil {
			out.HighestSource = &dto.CarbonAmount{Key: o.Highest.Sub, TotalKgCO2e: dto.D(o.Highest.KgCO2e.Round(6))}
		}
		for _, a := range o.ByCategory {
			out.ByCategory = append(out.ByCategory, dto.CarbonAmount{Key: a.Key, TotalKgCO2e: dto.D(a.KgCO2e.Round(6))})
		}
		for _, a := range o.ByScope {
			out.ByScope = append(out.ByScope, dto.CarbonAmount{Key: a.Key, TotalKgCO2e: dto.D(a.KgCO2e.Round(6))})
		}
		for m, p := range o.Monthly {
			out.Monthly[m] = dto.CarbonMonth{Month: m + 1, Current: dto.D(p.Current.Round(6)), Previous: dto.D(p.Previous.Round(6))}
		}
		for _, a := range o.Recent {
			out.Recent = append(out.Recent, carbonActivityDTO(a))
		}
		return out, nil
	})
}

func (h *Handlers) carbonCatalogue(w http.ResponseWriter, _ *http.Request) {
	out := dto.CarbonCatalogue{Items: []dto.CarbonCatalogueMain{}}
	for _, m := range domain.Mains() {
		main := dto.CarbonCatalogueMain{Key: m, Subs: []dto.CarbonCatalogueSub{}}
		for _, s := range domain.Subs(m) {
			main.Subs = append(main.Subs, dto.CarbonCatalogueSub{Key: s.Key, Scope: string(s.Scope), IsoCategory: s.ISO})
		}
		out.Items = append(out.Items, main)
	}
	kit.WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) carbonSelected(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.CarbonBuildingQuery) (any, error) {
		keys, err := h.Carbon.Selected(r.Context(), mw.ScopeFrom(r), req.BuildingID)
		if err != nil {
			return nil, err
		}
		return dto.CarbonSelection{ActivityKeys: keys}, nil
	})
}

func (h *Handlers) carbonSetSelected(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.CarbonSelectionRequest) (any, error) {
		keys, err := h.Carbon.SetSelected(r.Context(), mw.ScopeFrom(r), req.BuildingID, req.ActivityKeys)
		if err != nil {
			return nil, err
		}
		return dto.CarbonSelection{ActivityKeys: keys}, nil
	})
}

func (h *Handlers) carbonActivities(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.CarbonActivitiesRequest) (any, error) {
		page, limit, err := kit.ResolvePage(req.PageRequest, pageCap)
		if err != nil {
			return nil, err
		}
		q := carbonsvc.ActivityQuery{BuildingID: req.BuildingID, Type: req.Type, Automated: req.Automated, Page: page}
		if req.From != nil {
			q.From = &req.From.Time
		}
		if req.To != nil {
			q.To = &req.To.Time
		}
		if req.Scope != nil {
			sc := model.CarbonScope(*req.Scope)
			q.Scope = &sc
		}
		if req.Status != nil {
			st := model.CarbonStatus(*req.Status)
			q.Status = &st
		}
		list, err := h.Carbon.Activities(r.Context(), mw.ScopeFrom(r), q)
		if err != nil {
			return nil, err
		}
		rows := make([]dto.CarbonActivity, len(list))
		for i, a := range list {
			rows[i] = carbonActivityDTO(a)
		}
		return kit.PageOf(rows, page, limit), nil
	})
}

func activityInput(b dto.CarbonActivityBody) carbonsvc.ActivityInput {
	return carbonsvc.ActivityInput{SubCategory: b.SubCategory, FactorKey: b.FactorKey, Unit: b.Unit, Quantity: b.Quantity.Decimal,
		PeriodStart: b.PeriodStart.Time, PeriodEnd: b.PeriodEnd.Time, Description: b.Description, Details: b.Details}
}

func (h *Handlers) carbonCreateActivity(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusCreated, func(req dto.CarbonActivityCreateRequest) (any, error) {
		in := activityInput(req.CarbonActivityBody)
		in.BuildingID = req.BuildingID
		a, err := h.Carbon.CreateActivity(r.Context(), mw.ScopeFrom(r), actingUser(r), in)
		if err != nil {
			return nil, err
		}
		return carbonActivityDTO(a), nil
	})
}

func (h *Handlers) carbonUpdateActivity(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.CarbonActivityUpdateRequest) (any, error) {
		a, err := h.Carbon.UpdateActivity(r.Context(), mw.ScopeFrom(r), req.ID, activityInput(req.CarbonActivityBody))
		if err != nil {
			return nil, err
		}
		return carbonActivityDTO(a), nil
	})
}

func (h *Handlers) carbonDeleteActivity(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusNoContent, func(req dto.IDPath) (any, error) {
		return nil, h.Carbon.DeleteActivity(r.Context(), mw.ScopeFrom(r), req.ID)
	})
}

func (h *Handlers) carbonSetStatus(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.CarbonStatusRequest) (any, error) {
		a, err := h.Carbon.SetStatus(r.Context(), mw.ScopeFrom(r), req.ID, model.CarbonStatus(req.Status))
		if err != nil {
			return nil, err
		}
		return carbonActivityDTO(a), nil
	})
}

func factorDTO(v carbonsvc.FactorView) dto.EmissionFactorView {
	out := dto.EmissionFactorView{ID: v.ID, Key: v.Key, Label: v.Label, MainCategory: v.MainCategory,
		SubCategories: append([]string{}, v.SubCategories...), CategoryPath: append([]string{}, v.CategoryPath...),
		BaseFactor: dto.D(v.BaseFactor), BaseUnit: v.BaseUnit, FuelType: v.FuelType, VehicleType: v.VehicleType,
		IsoCategory: v.IsoCategory, Status: v.Status, Source: v.Source, SourceYear: v.SourceYear, SourceURL: v.SourceURL,
		Conversions: []dto.EmissionFactorConversion{}, Overridden: v.Overridden, PlatformBaseFactor: dto.DP(v.PlatformBaseFactor),
		UpdatedAt: dto.T(v.UpdatedAt)}
	if v.Scope != nil {
		s := string(*v.Scope)
		out.Scope = &s
	}
	for _, c := range v.Conversions {
		out.Conversions = append(out.Conversions, dto.EmissionFactorConversion{Unit: c.Unit, Multiplier: dto.D(c.Multiplier), Label: c.Label})
	}
	return out
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (h *Handlers) carbonFactors(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.EmissionFactorsRequest) (any, error) {
		list, err := h.Carbon.Factors(r.Context(), mw.ScopeFrom(r), carbonsvc.FactorQuery{SubCategory: deref(req.SubCategory),
			MainCategory: deref(req.MainCategory), Q: deref(req.Q)})
		if err != nil {
			return nil, err
		}
		out := dto.EmissionFactorList{Items: make([]dto.EmissionFactorView, len(list))}
		for i, v := range list {
			out.Items[i] = factorDTO(v)
		}
		return out, nil
	})
}

func (h *Handlers) carbonOverrideFactor(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.EmissionFactorOverrideRequest) (any, error) {
		v, err := h.Carbon.OverrideFactor(r.Context(), mw.ScopeFrom(r), req.ID, carbonsvc.FactorOverride{BaseFactor: req.BaseFactor.Decimal,
			Source: req.Source, SourceYear: req.SourceYear, SourceURL: req.SourceURL})
		if err != nil {
			return nil, err
		}
		return factorDTO(v), nil
	})
}

func (h *Handlers) carbonResetFactors(w http.ResponseWriter, r *http.Request) {
	if err := h.Carbon.ResetFactors(r.Context(), mw.ScopeFrom(r)); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func reportSummaryDTO(m model.CarbonReport) dto.CarbonReportSummary {
	return dto.CarbonReportSummary{ID: m.ID, BuildingID: m.BuildingID, Name: m.Name, ReportType: m.ReportType, Period: m.Period,
		CreatedAt: dto.T(m.CreatedAt)}
}

func (h *Handlers) carbonReports(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.CarbonReportsRequest) (any, error) {
		page, limit, err := kit.ResolvePage(req.PageRequest, pageCap)
		if err != nil {
			return nil, err
		}
		list, err := h.Carbon.Reports(r.Context(), mw.ScopeFrom(r), req.BuildingID, page)
		if err != nil {
			return nil, err
		}
		rows := make([]dto.CarbonReportSummary, len(list))
		for i, m := range list {
			rows[i] = reportSummaryDTO(m)
		}
		return kit.PageOf(rows, page, limit), nil
	})
}

func (h *Handlers) carbonCreateReport(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusCreated, func(req dto.CarbonReportRequest) (any, error) {
		m, err := h.Carbon.CreateReport(r.Context(), mw.ScopeFrom(r), carbonsvc.ReportInput{BuildingID: req.BuildingID,
			Type: req.ReportType, From: req.From.Time, To: req.To.Time, Name: req.Name})
		if err != nil {
			return nil, err
		}
		return reportSummaryDTO(m), nil
	})
}

func (h *Handlers) carbonReportPDF(w http.ResponseWriter, r *http.Request) {
	var req dto.IDPath
	if err := kit.Bind(r, &req); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	body, name, err := h.Carbon.ReportPDF(r.Context(), mw.ScopeFrom(r), req.ID, kit.Locale(r))
	if err != nil {
		kit.WriteError(w, r, err)
		return
	}
	writeFile(w, "application/pdf", name, body)
}
