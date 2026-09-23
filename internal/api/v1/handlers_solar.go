package v1

import (
	"net/http"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/kit"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/solar"
)

// plantRoutes is 05 §8 (R280–R288, R298). Plants are company-level (F-1).
func plantRoutes() []Route {
	aca := auth.Roles(roleA, roleCA)
	acacr := auth.Roles(roleA, roleCA, roleCR)
	return []Route{
		{Method: http.MethodGet, Pattern: "/plants/{id}/realtime", OperationID: "plants.realtime", Tag: "power-plants", Access: RoleGated,
			Roles: acacr, Summary: "Current power, yields and capacity utilisation from the inverters' last snapshots.",
			Request: dto.IDPath{}, Response: dto.PlantRealtime{}, Status: http.StatusOK, Handler: (*Handlers).plantRealtime},
		{Method: http.MethodGet, Pattern: "/plants/{id}/production", OperationID: "plants.production", Tag: "power-plants", Access: RoleGated,
			Roles: acacr, Summary: "Production series by hour, day or month.",
			Request: dto.PlantProductionRequest{}, Response: dto.PlantProductionSeries{}, Status: http.StatusOK, Handler: (*Handlers).plantProduction},
		{Method: http.MethodGet, Pattern: "/plants/{id}/production/export", OperationID: "plants.production.export", Tag: "power-plants",
			Access: RoleGated, Roles: acacr, Summary: "The production series as a workbook.", Request: dto.PlantProductionRequest{},
			Status: http.StatusOK, RawContentType: xlsxType, Handler: (*Handlers).plantProductionExport},
		{Method: http.MethodGet, Pattern: "/plants/{id}/devices", OperationID: "plants.devices", Tag: "power-plants", Access: RoleGated,
			Roles: acacr, Summary: "Devices with status, power and last update; q searches name and serial.",
			Request: dto.PlantDevicesRequest{}, Response: dto.PlantDevices{}, Status: http.StatusOK, Handler: (*Handlers).plantDevices},
		{Method: http.MethodGet, Pattern: "/plants/{id}/alarms", OperationID: "plants.alarms", Tag: "power-plants", Access: RoleGated,
			Roles: acacr, Summary: "iSolar fault alarms, newest first, translated.",
			Request: dto.PlantAlarmsRequest{}, Response: dto.PlantFaultPage{}, Status: http.StatusOK, Handler: (*Handlers).plantAlarms},
		{Method: http.MethodPost, Pattern: "/plants/{id}/sync", OperationID: "plants.sync", Tag: "power-plants", Access: RoleGated,
			Roles: aca, Entity: "power_plant", Summary: "Enqueue an iSolar sync; returns the job to watch.",
			Request: dto.IDPath{}, Response: dto.JobAccepted{}, Status: http.StatusAccepted, Handler: (*Handlers).plantSync},
		{Method: http.MethodGet, Pattern: "/plants/{id}/revenue", OperationID: "plants.revenue", Tag: "power-plants", Access: RoleGated,
			Roles: acacr, Summary: "Daily, monthly, yearly and total revenue at the feed-in tariff effective each day.",
			Request: dto.IDPath{}, Response: dto.PlantRevenue{}, Status: http.StatusOK, Handler: (*Handlers).plantRevenue},
		{Method: http.MethodGet, Pattern: "/integrations/isolar/plants", OperationID: "isolar.plants", Tag: "integrations", Access: RoleGated,
			Roles: aca, Summary: "Plants on the connected iSolarCloud account, for linking.",
			Request: dto.ISolarPlantsRequest{}, Response: dto.ISolarAccountPlants{}, Status: http.StatusOK, Handler: (*Handlers).isolarPlants},
		{Method: http.MethodPost, Pattern: "/plants/{id}/isolar-link", OperationID: "plants.isolar_link", Tag: "power-plants",
			Access: RoleGated, Roles: aca, Entity: "power_plant", Summary: "Bind a plant to an iSolarCloud plant; imports it and backfills.",
			Request: dto.ISolarLinkRequest{}, Response: dto.ISolarLinked{}, Status: http.StatusOK, Handler: (*Handlers).isolarLink},
		{Method: http.MethodDelete, Pattern: "/plants/{id}/isolar-link", OperationID: "plants.isolar_unlink", Tag: "power-plants",
			Access: RoleGated, Roles: aca, Entity: "power_plant", Summary: "Unlink; stored production stays.",
			Request: dto.IDPath{}, Status: http.StatusNoContent, Handler: (*Handlers).isolarUnlink},
		{Method: http.MethodPut, Pattern: "/plants/{id}/alarm-recipients", OperationID: "plants.alarm_recipients", Tag: "power-plants",
			Access: RoleGated, Roles: aca, Entity: "power_plant", Summary: "Set the plant's alarm forwarding recipients.",
			Request: dto.AlarmRecipientsRequest{}, Response: dto.AlarmRecipients{}, Status: http.StatusOK, Handler: (*Handlers).plantRecipients},
	}
}

func realtimeDTO(rt solar.Realtime) dto.PlantRealtime {
	return dto.PlantRealtime{AsOf: dto.TP(rt.AsOf), Stale: rt.Stale, InverterCount: rt.InverterCount,
		ActivePowerKw: dto.DP(rt.ActivePowerKw), YieldTodayKwh: dto.DP(rt.YieldTodayKwh), YieldMonthKwh: dto.DP(rt.YieldMonthKwh),
		YieldYearKwh: dto.DP(rt.YieldYearKwh), YieldTotalKwh: dto.DP(rt.YieldTotalKwh), CapacityKw: dto.DP(rt.CapacityKw),
		CapacityUtilisationPct: dto.DP(rt.CapacityUtilisationPct), Connection: rt.Connection, ConnectionError: rt.ConnectionError,
		LastSyncAt: dto.TP(rt.LastSyncAt)}
}

func (h *Handlers) plantRealtime(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.IDPath) (any, error) {
		rt, err := h.Solar.Realtime(r.Context(), mw.ScopeFrom(r), req.ID)
		if err != nil {
			return nil, err
		}
		return realtimeDTO(rt), nil
	})
}

func (h *Handlers) plantProduction(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.PlantProductionRequest) (any, error) {
		s, err := h.Solar.ProductionSeries(r.Context(), mw.ScopeFrom(r), req.ID, req.Granularity, req.From.Time, req.To.Time)
		if err != nil {
			return nil, err
		}
		out := dto.PlantProductionSeries{Granularity: s.Granularity, Points: make([]dto.PlantProductionPoint, len(s.Points)), MixedBasis: s.MixedBasis}
		for i, p := range s.Points {
			out.Points[i] = dto.PlantProductionPoint{Ts: dto.T(p.Ts), ProductionKwh: dto.DP(p.ProductionKwh), Basis: p.Basis}
		}
		return out, nil
	})
}

func (h *Handlers) plantProductionExport(w http.ResponseWriter, r *http.Request) {
	var req dto.PlantProductionRequest
	if err := kit.Bind(r, &req); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	body, name, err := h.Solar.ExportProduction(r.Context(), mw.ScopeFrom(r), req.ID, req.Granularity, req.From.Time, req.To.Time, kit.Locale(r))
	if err != nil {
		kit.WriteError(w, r, err)
		return
	}
	writeFile(w, xlsxType, name, body)
}

func (h *Handlers) plantDevices(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.PlantDevicesRequest) (any, error) {
		list, err := h.Solar.Devices(r.Context(), mw.ScopeFrom(r), req.ID, req.Q)
		if err != nil {
			return nil, err
		}
		out := dto.PlantDevices{Items: make([]dto.PlantDeviceView, len(list))}
		for i, d := range list {
			out.Items[i] = dto.PlantDeviceView{ID: d.ID, DeviceSN: d.DeviceSN, DeviceName: d.DeviceName, DeviceType: d.DeviceType,
				Status: d.Status, ActivePowerKw: dto.DP(d.ActivePowerKw), YieldTodayKwh: dto.DP(d.YieldTodayKwh),
				YieldTotalKwh: dto.DP(d.YieldTotalKwh), LastUpdate: dto.TP(d.LastUpdate)}
		}
		return out, nil
	})
}

func (h *Handlers) plantAlarms(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.PlantAlarmsRequest) (any, error) {
		page, limit, err := kit.ResolvePage(req.PageRequest, pageCap)
		if err != nil {
			return nil, err
		}
		list, total, err := h.Solar.Alarms(r.Context(), mw.ScopeFrom(r), req.ID, page)
		if err != nil {
			return nil, err
		}
		rows := make([]dto.PlantFault, len(list))
		for i, f := range list {
			rows[i] = dto.PlantFault{Ref: f.Ref, Code: f.Code, Name: f.Name, MessageTr: f.MessageTr, Translated: f.Translated,
				Level: f.Level, Type: f.Type, DeviceName: f.DeviceName, OccurredAt: dto.T(f.OccurredAt), ClosedAt: dto.TP(f.ClosedAt)}
		}
		p := kit.PageOf(rows, page, limit)
		return dto.PlantFaultPage{Items: p.Items, NextCursor: p.NextCursor, Total: total}, nil
	})
}

func (h *Handlers) plantSync(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusAccepted, func(req dto.IDPath) (any, error) {
		id, err := h.Solar.EnqueueSync(r.Context(), mw.ScopeFrom(r), req.ID)
		if err != nil {
			return nil, err
		}
		return dto.JobAccepted{JobID: id}, nil
	})
}

func revenuePeriodDTO(p solar.RevenuePeriod) *dto.RevenuePeriod {
	out := &dto.RevenuePeriod{Amounts: make([]dto.MoneyAmount, len(p.Amounts)), UnpricedDays: p.UnpricedDays, Partial: p.Partial}
	for i, a := range p.Amounts {
		out.Amounts[i] = dto.MoneyAmount{Currency: string(a.Currency), Amount: dto.D(a.Amount)}
	}
	if p.Since != nil {
		out.Since = &dto.Date{Time: *p.Since}
	}
	return out
}

func (h *Handlers) plantRevenue(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.IDPath) (any, error) {
		v, err := h.Solar.Revenue(r.Context(), mw.ScopeFrom(r), req.ID)
		if err != nil {
			return nil, err
		}
		if !v.Available {
			return dto.PlantRevenue{Reason: v.Reason}, nil
		}
		return dto.PlantRevenue{Available: true, Daily: revenuePeriodDTO(v.Daily), Monthly: revenuePeriodDTO(v.Monthly),
			Yearly: revenuePeriodDTO(v.Yearly), Total: revenuePeriodDTO(v.Total)}, nil
	})
}

func (h *Handlers) isolarPlants(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.ISolarPlantsRequest) (any, error) {
		list, err := h.Solar.AccountPlants(r.Context(), mw.ScopeFrom(r), req.CredentialID)
		if err != nil {
			return nil, err
		}
		out := dto.ISolarAccountPlants{Items: make([]dto.ISolarAccountPlant, len(list))}
		for i, p := range list {
			out.Items[i] = dto.ISolarAccountPlant{PSID: p.PSID, Name: p.Name, InstalledKw: dto.DP(p.InstalledKw), LinkedPlantID: p.LinkedPlantID}
		}
		return out, nil
	})
}

func (h *Handlers) isolarLink(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.ISolarLinkRequest) (any, error) {
		sc := mw.ScopeFrom(r)
		_, jobID, err := h.Solar.Link(r.Context(), sc, req.ID, req.CredentialID, req.PSID)
		if err != nil {
			return nil, err
		}
		detail, err := h.Assets.GetPlant(r.Context(), sc, req.ID)
		if err != nil {
			return nil, err
		}
		return dto.ISolarLinked{Plant: plantDetailDTO(detail), JobID: jobID}, nil
	})
}

func (h *Handlers) isolarUnlink(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusNoContent, func(req dto.IDPath) (any, error) {
		return nil, h.Solar.Unlink(r.Context(), mw.ScopeFrom(r), req.ID)
	})
}

func (h *Handlers) plantRecipients(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.AlarmRecipientsRequest) (any, error) {
		emails, err := h.Solar.SetRecipients(r.Context(), mw.ScopeFrom(r), req.ID, req.Emails)
		if err != nil {
			return nil, err
		}
		return dto.AlarmRecipients{Emails: emails}, nil
	})
}
