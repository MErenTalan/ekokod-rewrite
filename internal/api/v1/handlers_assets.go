package v1

import (
	"net/http"
	"slices"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/kit"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/comparison"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/assets"
)

func assetRoutes() []Route {
	all := auth.AllRoles
	aca := auth.Roles(roleA, roleCA)
	acacr := auth.Roles(roleA, roleCA, roleCR)
	return []Route{
		{Method: http.MethodGet, Pattern: "/buildings", OperationID: "buildings.list", Tag: "buildings", Access: RoleGated, Roles: all,
			Summary: "Buildings in scope; include=analyzer_count,active_status.", Request: dto.BuildingListRequest{},
			Response: dto.Page[dto.Building]{}, Status: http.StatusOK, Handler: (*Handlers).listBuildings},
		{Method: http.MethodPost, Pattern: "/buildings", OperationID: "buildings.create", Tag: "buildings", Access: RoleGated, Roles: aca,
			Entity: "building", Summary: "Create a building.", Request: dto.BuildingCreateRequest{}, Response: dto.BuildingDetail{},
			Status: http.StatusCreated, Handler: (*Handlers).createBuilding},
		{Method: http.MethodGet, Pattern: "/buildings/{id}", OperationID: "buildings.get", Tag: "buildings", Access: RoleGated, Roles: all,
			Summary: "A building with contacts and tariff history.", Request: dto.IDPath{}, Response: dto.BuildingDetail{},
			Status: http.StatusOK, Handler: (*Handlers).getBuilding},
		{Method: http.MethodPatch, Pattern: "/buildings/{id}", OperationID: "buildings.update", Tag: "buildings", Access: RoleGated, Roles: aca,
			Entity: "building", Summary: "Update a building.", Request: dto.BuildingUpdateRequest{}, Response: dto.BuildingDetail{},
			Status: http.StatusOK, Handler: (*Handlers).updateBuilding},
		{Method: http.MethodDelete, Pattern: "/buildings/{id}", OperationID: "buildings.delete", Tag: "buildings", Access: RoleGated, Roles: aca,
			Entity: "building", Summary: "Soft-delete a building with no analyzers.", Request: dto.IDPath{},
			Status: http.StatusNoContent, Handler: (*Handlers).deleteBuilding},
		{Method: http.MethodGet, Pattern: "/buildings/{id}/comparison", OperationID: "buildings.comparison", Tag: "buildings",
			Access: RoleGated, Roles: all, Summary: "Sectoral comparison figures and ranks (02 §10.4).", Request: dto.IDPath{},
			Response: dto.BuildingComparison{}, Status: http.StatusOK, Handler: (*Handlers).compareBuilding},
		{Method: http.MethodGet, Pattern: "/analyzers", OperationID: "analyzers.list", Tag: "analyzers", Access: RoleGated, Roles: all,
			Summary: "Analyzers in scope.", Request: dto.AnalyzerListRequest{}, Response: dto.Page[dto.Analyzer]{},
			Status: http.StatusOK, Handler: (*Handlers).listAnalyzers},
		{Method: http.MethodGet, Pattern: "/analyzers/{id}", OperationID: "analyzers.get", Tag: "analyzers", Access: RoleGated, Roles: all,
			Summary: "One analyzer.", Request: dto.IDPath{}, Response: dto.Analyzer{}, Status: http.StatusOK, Handler: (*Handlers).getAnalyzer},
		{Method: http.MethodPatch, Pattern: "/analyzers/{id}", OperationID: "analyzers.update", Tag: "analyzers", Access: RoleGated, Roles: aca,
			Entity: "analyzer", Summary: "Update an analyzer's building, multiplier, installed power or coordinates.",
			Request: dto.AnalyzerUpdateRequest{}, Response: dto.Analyzer{}, Status: http.StatusOK, Handler: (*Handlers).updateAnalyzer},
		{Method: http.MethodPost, Pattern: "/analyzers/{id}/refresh", OperationID: "analyzers.refresh", Tag: "analyzers", Access: RoleGated,
			Roles: auth.Roles(roleA, roleCA, roleBA), Entity: "analyzer", NoIdempotency: true,
			Summary: "Enqueue an on-demand pull of hourly or energy values.", Request: dto.AnalyzerRefreshRequest{},
			Response: dto.JobAccepted{}, Status: http.StatusAccepted, Handler: (*Handlers).refreshAnalyzer},
		{Method: http.MethodGet, Pattern: "/power-plants", OperationID: "plants.list", Tag: "power-plants", Access: RoleGated, Roles: acacr,
			Summary: "Power plants of the company in scope.", Request: dto.PageRequest{}, Response: dto.Page[dto.Plant]{},
			Status: http.StatusOK, Handler: (*Handlers).listPlants},
		{Method: http.MethodPost, Pattern: "/power-plants", OperationID: "plants.create", Tag: "power-plants", Access: RoleGated, Roles: aca,
			Entity: "power_plant", Summary: "Create a power plant.", Request: dto.PlantCreateRequest{}, Response: dto.PlantDetail{},
			Status: http.StatusCreated, Handler: (*Handlers).createPlant},
		{Method: http.MethodGet, Pattern: "/power-plants/{id}", OperationID: "plants.get", Tag: "power-plants", Access: RoleGated, Roles: acacr,
			Summary: "A power plant with devices, monthly targets and alarm recipients.", Request: dto.IDPath{},
			Response: dto.PlantDetail{}, Status: http.StatusOK, Handler: (*Handlers).getPlant},
		{Method: http.MethodPatch, Pattern: "/power-plants/{id}", OperationID: "plants.update", Tag: "power-plants", Access: RoleGated, Roles: aca,
			Entity: "power_plant", Summary: "Update a power plant.", Request: dto.PlantUpdateRequest{}, Response: dto.PlantDetail{},
			Status: http.StatusOK, Handler: (*Handlers).updatePlant},
		{Method: http.MethodDelete, Pattern: "/power-plants/{id}", OperationID: "plants.delete", Tag: "power-plants", Access: RoleGated, Roles: aca,
			Entity: "power_plant", Summary: "Soft-delete a power plant.", Request: dto.IDPath{},
			Status: http.StatusNoContent, Handler: (*Handlers).deletePlant},
	}
}

func dec(d *dto.Decimal) *decimal.Decimal {
	if d == nil {
		return nil
	}
	v := d.Decimal
	return &v
}

func buildingDTO(v assets.BuildingView) dto.Building {
	b := v.Building
	return dto.Building{
		ID: b.ID, Name: b.Name, Address: b.Address, Latitude: dto.DP(b.Latitude), Longitude: dto.DP(b.Longitude),
		Floors: b.Floors, PersonnelCount: b.PersonnelCount, TotalAreaM2: dto.DP(b.TotalAreaM2), Sector: b.Sector,
		ResponsibleUserID: b.ResponsibleUserID, BillCutoffDay: b.BillCutoffDay, CreatedAt: dto.T(b.CreatedAt), UpdatedAt: dto.T(b.UpdatedAt),
		AnalyzerCount: v.AnalyzerCount, ActiveAnalyzerCount: v.ActiveAnalyzerCount, ActivityStatus: v.ActivityStatus,
	}
}

func buildingDetailDTO(d assets.BuildingDetail) dto.BuildingDetail {
	out := dto.BuildingDetail{Building: buildingDTO(assets.BuildingView{Building: d.Building}),
		Contacts: make([]dto.Contact, len(d.Contacts)), TariffHistory: make([]dto.TariffSummary, len(d.TariffHistory))}
	for i, c := range d.Contacts {
		out.Contacts[i] = dto.Contact{Name: c.Name, Phone: c.Phone}
	}
	for i, t := range d.TariffHistory {
		out.TariffHistory[i] = dto.TariffSummary{ID: t.ID, Name: t.Name, EffectiveFrom: dto.Date{Time: t.EffectiveFrom}}
	}
	return out
}

func buildingInput(name *string, f dto.BuildingFields) assets.BuildingInput {
	in := assets.BuildingInput{
		Name: name, Address: f.Address, Sector: f.Sector, Latitude: dec(f.Latitude), Longitude: dec(f.Longitude),
		TotalAreaM2: dec(f.TotalAreaM2), Floors: f.Floors, PersonnelCount: f.PersonnelCount,
		ResponsibleUserID: f.ResponsibleUserID, ClearResponsible: f.ClearResponsible, BillCutoffDay: f.BillCutoffDay,
	}
	if f.Contacts != nil {
		contacts := make([]assets.Contact, len(*f.Contacts))
		for i, c := range *f.Contacts {
			contacts[i] = assets.Contact{Name: c.Name, Phone: c.Phone}
		}
		in.Contacts = &contacts
	}
	return in
}

func (h *Handlers) listBuildings(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.BuildingListRequest) (any, error) {
		page, limit, err := kit.ResolvePage(req.PageRequest, pageCap)
		if err != nil {
			return nil, err
		}
		for _, inc := range req.Include {
			if inc != "analyzer_count" && inc != "active_status" {
				return nil, kit.ErrInvalidParameters.WithParams(map[string]any{"include": []string{"invalid"}})
			}
		}
		views, err := h.Assets.ListBuildings(r.Context(), mw.ScopeFrom(r), assets.BuildingListInput{
			Q: req.Q, Sector: req.Sector, Page: page,
			IncludeCounts: slices.Contains(req.Include, "analyzer_count"), IncludeStatus: slices.Contains(req.Include, "active_status"),
		})
		if err != nil {
			return nil, err
		}
		items := make([]dto.Building, len(views))
		for i, v := range views {
			items[i] = buildingDTO(v)
		}
		return kit.PageOf(items, page, limit), nil
	})
}

func (h *Handlers) createBuilding(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusCreated, func(req dto.BuildingCreateRequest) (any, error) {
		d, err := h.Assets.CreateBuilding(r.Context(), mw.ScopeFrom(r), buildingInput(&req.Name, req.BuildingFields))
		return buildingDetailDTO(d), err
	})
}

func (h *Handlers) getBuilding(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.IDPath) (any, error) {
		d, err := h.Assets.GetBuilding(r.Context(), mw.ScopeFrom(r), req.ID)
		return buildingDetailDTO(d), err
	})
}

func (h *Handlers) updateBuilding(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.BuildingUpdateRequest) (any, error) {
		d, err := h.Assets.UpdateBuilding(r.Context(), mw.ScopeFrom(r), req.ID, buildingInput(req.Name, req.BuildingFields))
		return buildingDetailDTO(d), err
	})
}

func (h *Handlers) deleteBuilding(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusNoContent, func(req dto.IDPath) (any, error) {
		return nil, h.Assets.DeleteBuilding(r.Context(), mw.ScopeFrom(r), req.ID)
	})
}

func (h *Handlers) analyzerDTO(a model.Analyzer) dto.Analyzer {
	status := assets.StatusPassive
	if assets.Active(a, h.Clock.Now()) {
		status = assets.StatusActive
	}
	return dto.Analyzer{
		ID: a.ID, BuildingID: a.BuildingID, Provider: string(a.Provider), ProviderSubtype: a.ProviderSubtype,
		InstallationNumber: a.InstallationNumber, CustomerName: a.CustomerName, Address: a.Address, Province: a.Province,
		District: a.District, TariffType: a.TariffType, InstalledPowerKw: dto.DP(a.InstalledPowerKw), MeterNumber: a.MeterNumber,
		MeterModel: a.MeterModel, MeterMultiplier: dto.D(a.MeterMultiplier), Latitude: dto.DP(a.Latitude), Longitude: dto.DP(a.Longitude),
		LastReadingAt: dto.TP(a.LastReadingAt), IsActive: a.IsActive, ActivityStatus: status,
	}
}

func (h *Handlers) listAnalyzers(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.AnalyzerListRequest) (any, error) {
		page, limit, err := kit.ResolvePage(req.PageRequest, pageCap)
		if err != nil {
			return nil, err
		}
		providers := make([]model.IntegrationProvider, len(req.Provider))
		for i, p := range req.Provider {
			providers[i] = model.IntegrationProvider(p)
			if !providers[i].Valid() {
				return nil, kit.ErrInvalidParameters.WithParams(map[string]any{"provider": []string{"invalid"}})
			}
		}
		list, err := h.Assets.ListAnalyzers(r.Context(), mw.ScopeFrom(r), assets.AnalyzerListInput{
			BuildingID: req.BuildingID, Unassigned: req.Unassigned, Providers: providers, IsActive: req.IsActive, Q: req.Q, Page: page,
		})
		if err != nil {
			return nil, err
		}
		items := make([]dto.Analyzer, len(list))
		for i, a := range list {
			items[i] = h.analyzerDTO(a)
		}
		return kit.PageOf(items, page, limit), nil
	})
}

func (h *Handlers) getAnalyzer(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.IDPath) (any, error) {
		a, err := h.Assets.GetAnalyzer(r.Context(), mw.ScopeFrom(r), req.ID)
		return h.analyzerDTO(a), err
	})
}

func (h *Handlers) updateAnalyzer(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.AnalyzerUpdateRequest) (any, error) {
		a, err := h.Assets.UpdateAnalyzer(r.Context(), mw.ScopeFrom(r), req.ID, assets.AnalyzerInput{
			BuildingID: req.BuildingID, UnassignBuilding: req.UnassignBuilding, MeterMultiplier: dec(req.MeterMultiplier),
			InstalledPowerKw: dec(req.InstalledPowerKw), Latitude: dec(req.Latitude), Longitude: dec(req.Longitude),
		})
		return h.analyzerDTO(a), err
	})
}

func (h *Handlers) refreshAnalyzer(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusAccepted, func(req dto.AnalyzerRefreshRequest) (any, error) {
		id, err := h.Assets.RefreshAnalyzer(r.Context(), mw.ScopeFrom(r), req.ID, req.Mode)
		return dto.JobAccepted{JobID: id}, err
	})
}

func plantDTO(p model.PowerPlant) dto.Plant {
	out := dto.Plant{
		ID: p.ID, Name: p.Name, InstallationNumber: p.InstallationNumber, PlantKind: p.PlantKind, PvBrandModel: p.PvBrandModel,
		PanelPowerW: dto.DP(p.PanelPowerW), PanelEfficiencyPct: dto.DP(p.PanelEfficiencyPct), PanelCount: p.PanelCount,
		StringCount: p.StringCount, TiltAngleDeg: dto.DP(p.TiltAngleDeg), TotalCapacityKw: dto.DP(p.TotalCapacityKw),
		YearlyTargetKwh: dto.DP(p.YearlyTargetKwh), Address: p.Address, Latitude: dto.DP(p.Latitude), Longitude: dto.DP(p.Longitude),
		IsolarPsName: p.IsolarPsName, IsolarLinkedAt: dto.TP(p.IsolarLinkedAt), CreatedAt: dto.T(p.CreatedAt),
	}
	if p.Orientation != nil {
		o := string(*p.Orientation)
		out.Orientation = &o
	}
	if p.InstallationDate != nil {
		out.InstallationDate = &dto.Date{Time: *p.InstallationDate}
	}
	return out
}

func plantDetailDTO(d assets.PlantDetail) dto.PlantDetail {
	out := dto.PlantDetail{Plant: plantDTO(d.Plant), MonthlyTargets: []dto.Decimal{}, Devices: make([]dto.PlantDevice, len(d.Devices)),
		AlarmRecipients: d.AlarmRecipients}
	if out.AlarmRecipients == nil {
		out.AlarmRecipients = []string{}
	}
	for _, t := range d.MonthlyTargets {
		out.MonthlyTargets = append(out.MonthlyTargets, dto.D(t.TargetKwh))
	}
	for i, dev := range d.Devices {
		out.Devices[i] = dto.PlantDevice{ID: dev.ID, DeviceSN: dev.DeviceSN, DeviceName: dev.DeviceName, Brand: dev.Brand, Model: dev.Model,
			RatedPowerKw: dto.DP(dev.RatedPowerKw), Status: dev.Status, EfficiencyPct: dto.DP(dev.EfficiencyPct), LastSeenAt: dto.TP(dev.LastSeenAt)}
	}
	return out
}

func plantInput(name *string, f dto.PlantFields) assets.PlantInput {
	in := assets.PlantInput{
		Name: name, InstallationNumber: f.InstallationNumber, PlantKind: f.PlantKind, PvBrandModel: f.PvBrandModel, Address: f.Address,
		PanelPowerW: dec(f.PanelPowerW), PanelEfficiencyPct: dec(f.PanelEfficiencyPct), TiltAngleDeg: dec(f.TiltAngleDeg),
		TotalCapacityKw: dec(f.TotalCapacityKw), YearlyTargetKwh: dec(f.YearlyTargetKwh), Latitude: dec(f.Latitude),
		Longitude: dec(f.Longitude), PanelCount: f.PanelCount, StringCount: f.StringCount, AlarmRecipients: f.AlarmRecipients,
	}
	if f.Orientation != nil {
		o := model.PanelOrientation(*f.Orientation)
		in.Orientation = &o
	}
	if f.InstallationDate != nil {
		in.InstallationDate = &f.InstallationDate.Time
	}
	if f.MonthlyTargets != nil {
		targets := make([]decimal.Decimal, len(*f.MonthlyTargets))
		for i, t := range *f.MonthlyTargets {
			targets[i] = t.Decimal
		}
		in.MonthlyTargets = &targets
	}
	return in
}

func (h *Handlers) listPlants(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.PageRequest) (any, error) {
		page, limit, err := kit.ResolvePage(req, pageCap)
		if err != nil {
			return nil, err
		}
		list, err := h.Assets.ListPlants(r.Context(), mw.ScopeFrom(r), page)
		if err != nil {
			return nil, err
		}
		items := make([]dto.Plant, len(list))
		for i, p := range list {
			items[i] = plantDTO(p)
		}
		return kit.PageOf(items, page, limit), nil
	})
}

func (h *Handlers) createPlant(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusCreated, func(req dto.PlantCreateRequest) (any, error) {
		d, err := h.Assets.CreatePlant(r.Context(), mw.ScopeFrom(r), plantInput(&req.Name, req.PlantFields))
		return plantDetailDTO(d), err
	})
}

func (h *Handlers) getPlant(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.IDPath) (any, error) {
		d, err := h.Assets.GetPlant(r.Context(), mw.ScopeFrom(r), req.ID)
		return plantDetailDTO(d), err
	})
}

func (h *Handlers) updatePlant(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.PlantUpdateRequest) (any, error) {
		d, err := h.Assets.UpdatePlant(r.Context(), mw.ScopeFrom(r), req.ID, plantInput(req.Name, req.PlantFields))
		return plantDetailDTO(d), err
	})
}

func (h *Handlers) deletePlant(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusNoContent, func(req dto.IDPath) (any, error) {
		return nil, h.Assets.DeletePlant(r.Context(), mw.ScopeFrom(r), req.ID)
	})
}

func comparisonMetric(m comparison.Metric) dto.ComparisonMetric {
	return dto.ComparisonMetric{Value: dto.DP(m.Value), Average: dto.DP(m.Average), Rank: m.Rank, Ranked: m.Ranked}
}

func (h *Handlers) compareBuilding(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.IDPath) (any, error) {
		c, err := h.Assets.Compare(r.Context(), mw.ScopeFrom(r), req.ID)
		if err != nil {
			return nil, err
		}
		out := dto.BuildingComparison{
			Available: c.Result.Available, Sector: c.Sector, Peers: c.Result.Peers,
			DailyConsumption: comparisonMetric(c.Result.Daily), MonthlyConsumption: comparisonMetric(c.Result.Monthly),
			CO2EmissionKg: comparisonMetric(c.Result.CO2), ConsumptionPerCapita: comparisonMetric(c.Result.PerCapita),
			ConsumptionPerArea: comparisonMetric(c.Result.PerArea),
		}
		if c.Result.Reason != "" {
			reason := c.Result.Reason
			out.Reason = &reason
		}
		return out, nil
	})
}
