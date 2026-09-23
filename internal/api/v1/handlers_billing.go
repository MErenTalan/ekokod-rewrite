package v1

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/kit"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	domainbilling "github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	domaintariff "github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	billingsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/billing"
	tariffsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/tariff"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

func billingRoutes() []Route {
	all := auth.AllRoles
	aca := auth.Roles(roleA, roleCA)
	return []Route{
		{Method: http.MethodGet, Pattern: "/tariffs", OperationID: "tariffs.list", Tag: "tariffs", Access: RoleGated, Roles: all,
			Summary: "Tariff history.", Request: dto.TariffListRequest{}, Response: dto.Page[dto.TariffSummaryItem]{}, Status: http.StatusOK,
			Handler: (*Handlers).listTariffs},
		{Method: http.MethodPost, Pattern: "/tariffs", OperationID: "tariffs.create", Tag: "tariffs", Access: RoleGated, Roles: aca,
			Entity: "tariff", Summary: "Create a tariff version with taxes, extra charges and manual YEKDEM.", Request: dto.TariffFields{},
			Response: dto.Tariff{}, Status: http.StatusCreated, Handler: (*Handlers).createTariff},
		{Method: http.MethodGet, Pattern: "/tariffs/applicable", OperationID: "tariffs.applicable", Tag: "tariffs", Access: RoleGated, Roles: all,
			Summary: "The tariff in force for a building on a date.", Request: dto.TariffApplicableRequest{}, Response: dto.Tariff{},
			Status: http.StatusOK, Handler: (*Handlers).applicableTariff},
		{Method: http.MethodGet, Pattern: "/tariffs/{id}", OperationID: "tariffs.get", Tag: "tariffs", Access: RoleGated, Roles: all,
			Summary: "A tariff version.", Request: dto.IDPath{}, Response: dto.Tariff{}, Status: http.StatusOK, Handler: (*Handlers).getTariff},
		{Method: http.MethodPatch, Pattern: "/tariffs/{id}", OperationID: "tariffs.update", Tag: "tariffs", Access: RoleGated, Roles: aca,
			Entity: "tariff", Summary: "Replace a tariff version.", Request: dto.TariffUpdateRequest{}, Response: dto.Tariff{},
			Status: http.StatusOK, Handler: (*Handlers).updateTariff},
		{Method: http.MethodDelete, Pattern: "/tariffs/{id}", OperationID: "tariffs.delete", Tag: "tariffs", Access: RoleGated, Roles: aca,
			Entity: "tariff", Summary: "Soft-delete a tariff version.", Request: dto.IDPath{}, Status: http.StatusNoContent,
			Handler: (*Handlers).deleteTariff},
		{Method: http.MethodGet, Pattern: "/bills", OperationID: "bills.list", Tag: "bills", Access: RoleGated, Roles: all,
			Summary: "Bills in scope.", Request: dto.BillListRequest{}, Response: dto.Page[dto.Bill]{}, Status: http.StatusOK,
			Handler: (*Handlers).listBills},
		{Method: http.MethodPost, Pattern: "/bills/compute", OperationID: "bills.compute", Tag: "bills", Access: RoleGated,
			Roles: auth.Roles(roleA, roleCA, roleBA), Entity: "bill",
			Summary: "Enqueue bill computation; recomputation supersedes.", Request: dto.BillComputeRequest{},
			Response: dto.BillComputeAccepted{}, Status: http.StatusAccepted, Handler: (*Handlers).computeBills},
		{Method: http.MethodGet, Pattern: "/bills/dashboard", OperationID: "bills.dashboard", Tag: "bills", Access: RoleGated, Roles: all,
			Summary: "The month's invoice dashboard: analyzer rows per building, their totals and the netting summary.",
			Request: dto.BillDashboardRequest{}, Response: dto.BillDashboard{}, Status: http.StatusOK,
			Handler: (*Handlers).billDashboard},
		{Method: http.MethodGet, Pattern: "/bills/dashboard/export", OperationID: "bills.dashboard.export", Tag: "bills",
			Access: RoleGated, Roles: all, Summary: "The whole dashboard as one XLSX or PDF.",
			Request: dto.BillDashboardExportRequest{}, Status: http.StatusOK, RawContentType: "application/octet-stream",
			Handler: (*Handlers).billDashboardExport},
		{Method: http.MethodGet, Pattern: "/bills/latest", OperationID: "bills.latest", Tag: "bills", Access: RoleGated, Roles: all,
			Summary: "The most recent bill of a subject.", Request: dto.BillLatestRequest{}, Response: dto.Bill{}, Status: http.StatusOK,
			Handler: (*Handlers).latestBill},
		{Method: http.MethodGet, Pattern: "/bills/{id}", OperationID: "bills.get", Tag: "bills", Access: RoleGated, Roles: all,
			Summary: "A bill with lines and members.", Request: dto.IDPath{}, Response: dto.BillDetail{}, Status: http.StatusOK,
			Handler: (*Handlers).getBill},
		{Method: http.MethodGet, Pattern: "/bills/{id}/pdf", OperationID: "bills.pdf", Tag: "bills", Access: RoleGated, Roles: all,
			Summary: "The invoice PDF, rendered on demand when not yet stored.", Request: dto.IDPath{}, Status: http.StatusOK,
			RawContentType: "application/pdf", Handler: (*Handlers).billPDF},
		{Method: http.MethodGet, Pattern: "/bills/{id}/hourly-detail", OperationID: "bills.hourly_detail", Tag: "bills", Access: RoleGated,
			Roles: all, Summary: "Per-hour PTF detail as JSON or XLSX.", Request: dto.BillHourlyRequest{}, Response: dto.BillHours{},
			Status: http.StatusOK, Handler: (*Handlers).billHourly},
	}
}

// billingErr maps F4's errors onto the envelope (R179).
func billingErr(err error) error {
	var ce *billingsvc.ComputeError
	var ve *domaintariff.ValidationError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &ce):
		status := map[string]int{
			billingsvc.CodeTariffNotFound: 422, billingsvc.CodeNoConsumptionData: 422, billingsvc.CodeUnresolvedAnomaly: 409,
			billingsvc.CodePeriodNotClosed: 409, "ptf_data_missing": 422,
			billingsvc.CodeBillingParametersMissing: 500, billingsvc.CodeBillingParametersInvalid: 500,
		}[ce.Code]
		if status == 0 {
			status = 500
		}
		details := map[string]any{}
		for k, v := range ce.Detail {
			details[k] = v
		}
		return perr.New(ce.Code, status, "errors.billing."+ce.Code).WithParams(details)
	case errors.As(err, &ve):
		details := map[string]any{}
		for field, code := range ve.Fields {
			details[field] = []string{code}
		}
		return perr.Validation.WithParams(details)
	case errors.Is(err, billingsvc.ErrInvalidRequest), errors.Is(err, tariffsvc.ErrInvalidRequest), errors.Is(err, billingsvc.ErrUnknownScope):
		return kit.ErrInvalidParameters
	}
	return err
}

func dp(d *dto.Decimal) *decimal.Decimal { return dec(d) }

func dv(d *dto.Decimal) decimal.Decimal {
	if d == nil {
		return decimal.Zero
	}
	return d.Decimal
}

func tariffInput(f dto.TariffFields) tariffsvc.Input {
	t := model.Tariff{
		BuildingID: f.BuildingID, Name: f.Name, EffectiveFrom: f.EffectiveFrom.Time,
		Currency: model.CurrencyCode(f.Currency), EnergyType: model.EnergyType(f.EnergyType), VoltageLevel: model.VoltageLevel(f.VoltageLevel),
		UserGroup: model.DistributionUserGroup(f.UserGroup), PriceType: model.PriceType(f.PriceType), Term: model.TariffTerm(f.Term),
		SupplyCompany: model.SupplyCompany(f.SupplyCompany), SingleTimePrice: dp(f.SingleTimePrice), T1Price: dp(f.T1Price),
		T2Price: dp(f.T2Price), T3Price: dp(f.T3Price), OverusePrice: dp(f.OverusePrice), OveruseThresholdKwhPerDay: dp(f.OveruseThresholdKwhPerDay),
		DistributionCost: dv(f.DistributionCost), ReactivePowerPrice: dv(f.ReactivePowerPrice), GreenEnergyPrice: dp(f.GreenEnergyPrice),
		GreenEnergyDistributionCost: dp(f.GreenEnergyDistributionCost), ContractedPowerKw: dp(f.ContractedPowerKw),
		PowerUnitPrice: dp(f.PowerUnitPrice), GenerationUsage: model.GenerationUsage(f.GenerationUsage),
		GenerationPricePerKwh: dp(f.GenerationPricePerKwh), UsePtfYekdem: f.UsePtfYekdem, KbkEnergy: dp(f.KbkEnergy),
		KbkT1: dp(f.KbkT1), KbkT2: dp(f.KbkT2), KbkT3: dp(f.KbkT3), KbkPowerPrice: dp(f.KbkPowerPrice),
		KbkOverusePrice: dp(f.KbkOverusePrice), KbkReactivePower: dp(f.KbkReactivePower),
		KbkDistributionCostTlPerKwh: dp(f.KbkDistributionCostTlPerKwh), UseManualYekdem: f.UseManualYekdem,
		PowerPriceSource: model.PriceSource(f.PowerPriceSource), ReactivePriceSource: model.PriceSource(f.ReactivePriceSource),
		DistributionPriceSource: model.PriceSource(f.DistributionPriceSource),
	}
	in := tariffsvc.Input{Tariff: t, VatRate: dp(f.VatRate)}
	for i, tax := range f.Taxes {
		in.Taxes = append(in.Taxes, model.TariffTax{Name: tax.Name, Rate: dv(tax.Rate), SortOrder: int16(i)})
	}
	for i, c := range f.ExtraCharges {
		in.ExtraCharges = append(in.ExtraCharges, model.TariffExtraCharge{Name: c.Name, Basis: model.ExtraChargeBasis(c.Basis), Amount: dv(c.Amount), SortOrder: int16(i)})
	}
	for _, y := range f.ManualYekdem {
		in.ManualYekdem = append(in.ManualYekdem, model.TariffManualYekdem{Year: y.Year, Month: y.Month, Value: dv(y.Value)})
	}
	return in
}

func tariffDTO(d tariffsvc.Definition) dto.Tariff {
	t := d.Tariff
	f := dto.TariffFields{
		BuildingID: t.BuildingID, Name: t.Name, EffectiveFrom: dto.Date{Time: t.EffectiveFrom}, Currency: string(t.Currency),
		EnergyType: string(t.EnergyType), VoltageLevel: string(t.VoltageLevel), UserGroup: string(t.UserGroup), PriceType: string(t.PriceType),
		Term: string(t.Term), SupplyCompany: string(t.SupplyCompany), SingleTimePrice: dto.DP(t.SingleTimePrice), T1Price: dto.DP(t.T1Price),
		T2Price: dto.DP(t.T2Price), T3Price: dto.DP(t.T3Price), OverusePrice: dto.DP(t.OverusePrice),
		OveruseThresholdKwhPerDay: dto.DP(t.OveruseThresholdKwhPerDay), DistributionCost: dto.DP(&t.DistributionCost),
		ReactivePowerPrice: dto.DP(&t.ReactivePowerPrice), GreenEnergyPrice: dto.DP(t.GreenEnergyPrice),
		GreenEnergyDistributionCost: dto.DP(t.GreenEnergyDistributionCost), ContractedPowerKw: dto.DP(t.ContractedPowerKw),
		PowerUnitPrice: dto.DP(t.PowerUnitPrice), GenerationUsage: string(t.GenerationUsage), GenerationPricePerKwh: dto.DP(t.GenerationPricePerKwh),
		VatRate: dto.DP(&t.VatRate), UsePtfYekdem: t.UsePtfYekdem, KbkEnergy: dto.DP(t.KbkEnergy), KbkT1: dto.DP(t.KbkT1), KbkT2: dto.DP(t.KbkT2),
		KbkT3: dto.DP(t.KbkT3), KbkPowerPrice: dto.DP(t.KbkPowerPrice), KbkOverusePrice: dto.DP(t.KbkOverusePrice),
		KbkReactivePower: dto.DP(t.KbkReactivePower), KbkDistributionCostTlPerKwh: dto.DP(t.KbkDistributionCostTlPerKwh),
		UseManualYekdem: t.UseManualYekdem, PowerPriceSource: string(t.PowerPriceSource), ReactivePriceSource: string(t.ReactivePriceSource),
		DistributionPriceSource: string(t.DistributionPriceSource),
		Taxes:                   []dto.TariffTax{}, ExtraCharges: []dto.TariffExtraCharge{}, ManualYekdem: []dto.TariffManualYekdem{},
	}
	for _, tax := range d.Taxes {
		f.Taxes = append(f.Taxes, dto.TariffTax{Name: tax.Name, Rate: dto.DP(&tax.Rate)})
	}
	for _, c := range d.ExtraCharges {
		f.ExtraCharges = append(f.ExtraCharges, dto.TariffExtraCharge{Name: c.Name, Basis: string(c.Basis), Amount: dto.DP(&c.Amount)})
	}
	for _, y := range d.ManualYekdem {
		f.ManualYekdem = append(f.ManualYekdem, dto.TariffManualYekdem{Year: y.Year, Month: y.Month, Value: dto.DP(&y.Value)})
	}
	return dto.Tariff{ID: t.ID, TariffFields: f, CreatedAt: dto.T(t.CreatedAt), UpdatedAt: dto.T(t.UpdatedAt)}
}

func (h *Handlers) listTariffs(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.TariffListRequest) (any, error) {
		page, limit, err := kit.ResolvePage(req.PageRequest, pageCap)
		if err != nil {
			return nil, err
		}
		list, err := h.Tariffs.List(r.Context(), mw.ScopeFrom(r), store.TariffFilter{BuildingID: req.BuildingID, Page: page})
		if err != nil {
			return nil, billingErr(err)
		}
		items := make([]dto.TariffSummaryItem, len(list))
		for i, t := range list {
			items[i] = dto.TariffSummaryItem{ID: t.ID, BuildingID: t.BuildingID, Name: t.Name, EffectiveFrom: dto.Date{Time: t.EffectiveFrom},
				PriceType: string(t.PriceType), Term: string(t.Term), UsePtfYekdem: t.UsePtfYekdem}
		}
		return kit.PageOf(items, page, limit), nil
	})
}

func (h *Handlers) createTariff(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusCreated, func(req dto.TariffFields) (any, error) {
		d, err := h.Tariffs.Create(r.Context(), mw.ScopeFrom(r), tariffInput(req))
		if err != nil {
			return nil, billingErr(err)
		}
		return tariffDTO(d), nil
	})
}

func (h *Handlers) getTariff(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.IDPath) (any, error) {
		d, err := h.Tariffs.Get(r.Context(), mw.ScopeFrom(r), req.ID)
		if err != nil {
			return nil, billingErr(err)
		}
		return tariffDTO(d), nil
	})
}

func (h *Handlers) updateTariff(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.TariffUpdateRequest) (any, error) {
		if _, err := h.Tariffs.Get(r.Context(), mw.ScopeFrom(r), req.ID); err != nil {
			return nil, billingErr(err)
		}
		d, err := h.Tariffs.Update(r.Context(), mw.ScopeFrom(r), req.ID, tariffInput(req.TariffFields))
		if err != nil {
			return nil, billingErr(err)
		}
		return tariffDTO(d), nil
	})
}

func (h *Handlers) deleteTariff(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusNoContent, func(req dto.IDPath) (any, error) {
		return nil, billingErr(h.Tariffs.Delete(r.Context(), mw.ScopeFrom(r), req.ID))
	})
}

func (h *Handlers) applicableTariff(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.TariffApplicableRequest) (any, error) {
		d, err := h.Tariffs.Applicable(r.Context(), mw.ScopeFrom(r), req.BuildingID, req.Date.Time)
		if err != nil {
			return nil, billingErr(err)
		}
		return tariffDTO(d), nil
	})
}

func billDTO(b model.Bill) dto.Bill {
	return dto.Bill{
		ID: b.ID, Scope: string(b.Scope), BuildingID: b.BuildingID, AnalyzerID: b.AnalyzerID, PeriodKey: b.PeriodKey,
		PeriodStart: dto.T(b.PeriodStart), PeriodEnd: dto.T(b.PeriodEnd), DaysInPeriod: b.DaysInPeriod, TariffID: b.TariffID,
		ActiveImport: dto.D(b.ActiveImport), NetConsumption: dto.D(b.NetConsumption), InductiveKvarh: dto.D(b.InductiveKvarh),
		CapacitiveKvarh: dto.D(b.CapacitiveKvarh), MaxDemandKw: dto.DP(b.MaxDemandKw), EnergyCost: dto.D(b.EnergyCost),
		DistributionCost: dto.D(b.DistributionCost), PowerCost: dto.D(b.PowerCost), ReactivePenalty: dto.D(b.ReactivePenalty),
		VatCost: dto.D(b.VatCost), TotalCost: dto.D(b.TotalCost), Currency: string(b.Currency), InductiveRatio: dto.DP(b.InductiveRatio),
		CapacitiveRatio: dto.DP(b.CapacitiveRatio), ReactivePenaltyApplied: b.ReactivePenaltyApplied, PtfYekdemUsed: b.PtfYekdemUsed,
		Status: string(b.Status), FlagReason: b.FlagReason, HasPDF: b.PdfPath != nil, ComputedAt: dto.T(b.ComputedAt),
	}
}

func (h *Handlers) listBills(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.BillListRequest) (any, error) {
		page, limit, err := kit.ResolvePage(req.PageRequest, pageCap)
		if err != nil {
			return nil, err
		}
		f := store.BillFilter{BuildingID: req.BuildingID, AnalyzerID: req.AnalyzerID, Page: page}
		if req.Scope != "" {
			s := model.BillScope(req.Scope)
			f.BillScope = &s
		}
		if req.Period != "" {
			f.PeriodKey = &req.Period
		}
		for _, st := range req.Status {
			status := model.BillStatus(st)
			if !status.Valid() {
				return nil, kit.ErrInvalidParameters.WithParams(map[string]any{"status": []string{"invalid"}})
			}
			f.Statuses = append(f.Statuses, status)
		}
		list, err := h.Billing.List(r.Context(), mw.ScopeFrom(r), f)
		if err != nil {
			return nil, billingErr(err)
		}
		items := make([]dto.Bill, len(list))
		for i, b := range list {
			items[i] = billDTO(b)
		}
		return kit.PageOf(items, page, limit), nil
	})
}

func (h *Handlers) getBill(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.IDPath) (any, error) {
		b, lines, members, err := h.Billing.Get(r.Context(), mw.ScopeFrom(r), req.ID)
		if err != nil {
			return nil, billingErr(err)
		}
		out := dto.BillDetail{Bill: billDTO(b), Lines: make([]dto.BillLine, len(lines))}
		for i, l := range lines {
			out.Lines[i] = dto.BillLine{Code: l.Code, Label: l.Label, Quantity: dto.DP(l.Quantity), Unit: l.Unit,
				UnitPrice: dto.DP(l.UnitPrice), RatePct: dto.DP(l.RatePct), Amount: dto.D(l.Amount)}
		}
		for _, m := range members {
			out.MemberIDs = append(out.MemberIDs, m.AnalyzerID)
		}
		if out.MemberIDs == nil {
			out.MemberIDs = []uuid.UUID{}
		}
		return out, nil
	})
}

func (h *Handlers) latestBill(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.BillLatestRequest) (any, error) {
		b, err := h.Billing.Latest(r.Context(), mw.ScopeFrom(r), model.BillScope(req.Scope), req.SubjectID)
		if err != nil {
			return nil, billingErr(err)
		}
		return billDTO(b), nil
	})
}

func (h *Handlers) computeBills(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusAccepted, func(req dto.BillComputeRequest) (any, error) {
		ids, err := h.BillRequests.Compute(r.Context(), mw.ScopeFrom(r), billingsvc.ComputeRequest{
			Scope: model.BillScope(req.Scope), BuildingIDs: req.BuildingIDs, AnalyzerIDs: req.AnalyzerIDs, PeriodKey: req.Period, Force: req.Force,
		})
		if err != nil {
			return nil, billingErr(err)
		}
		return dto.BillComputeAccepted{JobIDs: ids}, nil
	})
}

func (h *Handlers) billPDF(w http.ResponseWriter, r *http.Request) {
	var req dto.IDPath
	if err := kit.Bind(r, &req); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	pdf, b, err := h.BillRequests.PDF(r.Context(), mw.ScopeFrom(r), req.ID)
	if err != nil {
		kit.WriteError(w, r, billingErr(err))
		return
	}
	writeFile(w, "application/pdf", "fatura-"+b.PeriodKey+"-"+b.ID.String()[:8]+".pdf", pdf)
}

func (h *Handlers) billDashboard(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.BillDashboardRequest) (any, error) {
		res, err := h.Billing.Dashboard(r.Context(), mw.ScopeFrom(r), billingsvc.DashboardInput{Year: req.Year, Month: req.Month})
		if err != nil {
			return nil, billingErr(err)
		}
		return dashboardDTO(res, fmt.Sprintf("%04d-%02d", req.Year, req.Month)), nil
	})
}

func (h *Handlers) billDashboardExport(w http.ResponseWriter, r *http.Request) {
	var req dto.BillDashboardExportRequest
	if err := kit.Bind(r, &req); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	format := req.Format
	if format == "" {
		format = "xlsx"
	}
	body, name, err := h.BillRequests.DashboardExport(r.Context(), mw.ScopeFrom(r),
		billingsvc.DashboardInput{Year: req.Year, Month: req.Month}, format, kit.Locale(r))
	if err != nil {
		kit.WriteError(w, r, billingErr(err))
		return
	}
	contentType := xlsxType
	if format == "pdf" {
		contentType = "application/pdf"
	}
	writeFile(w, contentType, name, body)
}

// dashboardDTO maps the pure aggregate onto the wire. The plant section is
// always unavailable: 01 §7.10 describes it, and nothing in the schema can
// answer it before F9 (R235).
func dashboardDTO(res domainbilling.DashboardResult, period string) dto.BillDashboard {
	out := dto.BillDashboard{
		Period:    period,
		Buildings: make([]dto.BillDashboardBuilding, 0, len(res.Buildings)),
		Netting:   make([]dto.BillDashboardNetting, 0, len(res.Netting)),
		Plants:    dto.BillDashboardPlants{Available: false, Reason: "no_plant_production_source"},
	}
	for _, b := range res.Buildings {
		building := dto.BillDashboardBuilding{
			BuildingID: b.BuildingID, BuildingName: b.BuildingName, Rows: make([]dto.BillDashboardRow, 0, len(b.Rows)),
			TotalConsumption: dto.D(b.TotalConsumption), TotalProduction: dto.D(b.TotalProduction),
			TotalInvoice: dto.D(b.TotalInvoice), Currency: string(b.Currency), DivergesFromRows: b.DivergesFromRows,
		}
		for _, row := range b.Rows {
			building.Rows = append(building.Rows, dashboardRowDTO(row))
		}
		if b.BuildingBill != nil {
			bill := dashboardRowDTO(*b.BuildingBill)
			building.BuildingBill = &bill
		}
		out.Buildings = append(out.Buildings, building)
	}
	for _, n := range res.Netting {
		out.Netting = append(out.Netting, dto.BillDashboardNetting{
			Currency: string(n.Currency), TotalConsumption: dto.D(n.TotalConsumption), TotalProduction: dto.D(n.TotalProduction),
			Net: dto.D(n.Net), NetStatus: n.NetStatus, TotalInvoice: dto.D(n.TotalInvoice),
			EfficiencyPct: dto.DP(n.EfficiencyPct), PeriodKey: n.PeriodKey, CompanyBillID: n.CompanyBillID,
		})
	}
	return out
}

func dashboardRowDTO(row domainbilling.DashboardRow) dto.BillDashboardRow {
	return dto.BillDashboardRow{
		BillID: row.BillID, BuildingID: row.BuildingID, AnalyzerID: row.AnalyzerID, BuildingName: row.BuildingName,
		AnalyzerName: row.AnalyzerName, InstallationNumber: row.InstallationNumber, EtsoCode: row.EtsoCode,
		PeriodKey: row.PeriodKey, Consumption: dto.D(row.Consumption), Production: dto.D(row.Production),
		ConsumptionPrice: dto.DP(row.ConsumptionPrice), ProductionPrice: dto.DP(row.ProductionPrice),
		Invoice: dto.D(row.Invoice), Currency: string(row.Currency),
	}
}

func (h *Handlers) billHourly(w http.ResponseWriter, r *http.Request) {
	var req dto.BillHourlyRequest
	if err := kit.Bind(r, &req); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	if req.Format == "xlsx" {
		body, b, err := h.BillRequests.HourlyXLSX(r.Context(), mw.ScopeFrom(r), req.ID, kit.Locale(r))
		if err != nil {
			kit.WriteError(w, r, billingErr(err))
			return
		}
		writeFile(w, xlsxType, "saatlik-ptf-"+b.PeriodKey+".xlsx", body)
		return
	}
	rows, err := h.Billing.HourlyDetail(r.Context(), mw.ScopeFrom(r), req.ID)
	if err != nil {
		kit.WriteError(w, r, billingErr(err))
		return
	}
	out := dto.BillHours{Items: make([]dto.BillHour, len(rows))}
	for i, row := range rows {
		out.Items[i] = dto.BillHour{Ts: dto.T(row.Ts), Consumption: dto.D(row.Consumption), PTF: dto.D(row.PTF), Yekdem: dto.D(row.Yekdem),
			Kbk: dto.D(row.Kbk), UnitPrice: dto.D(row.UnitPrice), Cost: dto.D(row.Cost)}
	}
	kit.WriteJSON(w, http.StatusOK, out)
}
