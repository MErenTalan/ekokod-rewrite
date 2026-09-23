package v1

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/kit"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff/icmal"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/assets"
	tariffsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/tariff"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// tariffRoutes is 05 §6's template, bulk-assignment and icmal surface. Every
// handler is a mapping between the DTO and the service that F4 already
// wrote — no validation or copying logic lives here (R240).
func tariffRoutes() []Route {
	aca := auth.Roles(roleA, roleCA)
	acacr := auth.Roles(roleA, roleCA, roleCR)
	return []Route{
		{Method: http.MethodGet, Pattern: "/tariff-templates", OperationID: "tariff_templates.list", Tag: "tariffs",
			Access: RoleGated, Roles: acacr, Summary: "The company's tariff templates.",
			Request: dto.TariffTemplateListRequest{}, Response: dto.Page[dto.TariffTemplate]{}, Status: http.StatusOK,
			Handler: (*Handlers).listTariffTemplates},
		{Method: http.MethodPost, Pattern: "/tariff-templates", OperationID: "tariff_templates.create", Tag: "tariffs",
			Access: RoleGated, Roles: aca, Entity: "tariff_template", Summary: "Create a tariff template.",
			Request: dto.TariffTemplateFields{}, Response: dto.TariffTemplate{}, Status: http.StatusCreated,
			Handler: (*Handlers).createTariffTemplate},
		{Method: http.MethodPatch, Pattern: "/tariff-templates/{id}", OperationID: "tariff_templates.update", Tag: "tariffs",
			Access: RoleGated, Roles: aca, Entity: "tariff_template", Summary: "Replace a tariff template.",
			Request: dto.TariffTemplateUpdateRequest{}, Response: dto.TariffTemplate{}, Status: http.StatusOK,
			Handler: (*Handlers).updateTariffTemplate},
		{Method: http.MethodDelete, Pattern: "/tariff-templates/{id}", OperationID: "tariff_templates.delete", Tag: "tariffs",
			Access: RoleGated, Roles: aca, Entity: "tariff_template", Summary: "Delete a tariff template.",
			Request: dto.IDPath{}, Status: http.StatusNoContent, Handler: (*Handlers).deleteTariffTemplate},
		{Method: http.MethodPost, Pattern: "/tariff-templates/{id}/apply", OperationID: "tariff_templates.apply", Tag: "tariffs",
			Access: RoleGated, Roles: aca, Entity: "tariff", Summary: "Apply a template to a set of buildings.",
			Request: dto.TariffTemplateApplyRequest{}, Response: dto.BulkTariffResult{}, Status: http.StatusOK,
			Handler: (*Handlers).applyTariffTemplate},
		{Method: http.MethodGet, Pattern: "/buildings/bulk-tariff/current", OperationID: "bulk_tariff.current", Tag: "tariffs",
			Access: RoleGated, Roles: acacr, Summary: "Every building's tariff in force today.",
			Request: dto.PageRequest{}, Response: dto.Page[dto.BuildingTariffState]{}, Status: http.StatusOK,
			Handler: (*Handlers).bulkTariffCurrent},
		{Method: http.MethodGet, Pattern: "/buildings/bulk-tariff/history", OperationID: "bulk_tariff.history", Tag: "tariffs",
			Access: RoleGated, Roles: acacr, Summary: "Bulk assignment history, newest first.",
			Request: dto.PageRequest{}, Response: dto.Page[dto.BulkTariffAssignment]{}, Status: http.StatusOK,
			Handler: (*Handlers).bulkTariffHistory},
		{Method: http.MethodPost, Pattern: "/buildings/bulk-tariff", OperationID: "bulk_tariff.assign", Tag: "tariffs",
			Access: RoleGated, Roles: aca, Entity: "tariff", Summary: "Assign one tariff definition to many buildings.",
			Request: dto.BulkTariffRequest{}, Response: dto.BulkTariffResult{}, Status: http.StatusOK,
			Handler: (*Handlers).bulkTariffAssign},
	}
}

// actingUser is the principal a bulk assignment is recorded against;
// uuid.Nil for a system call, which the history stores as "no user".
func actingUser(r *http.Request) uuid.UUID {
	if p, ok := mw.PrincipalFrom(r.Context()); ok {
		return p.User.ID
	}
	return uuid.Nil
}

func templateDTO(t model.TariffTemplate) (dto.TariffTemplate, error) {
	var in tariffsvc.Input
	if err := json.Unmarshal(t.Payload, &in); err != nil {
		return dto.TariffTemplate{}, err
	}
	fields := tariffDTO(tariffsvc.Definition{Tariff: in.Tariff, Taxes: in.Taxes, ExtraCharges: in.ExtraCharges,
		ManualYekdem: in.ManualYekdem}).TariffFields
	if in.VatRate != nil {
		fields.VatRate = dto.DP(in.VatRate)
	}
	return dto.TariffTemplate{
		ID: t.ID,
		TariffTemplateFields: dto.TariffTemplateFields{
			Name: t.Name, Description: t.Description, IsDefault: t.IsDefault, Tariff: fields,
		},
		CreatedAt: dto.T(t.CreatedAt), UpdatedAt: dto.T(t.UpdatedAt),
	}, nil
}

func (h *Handlers) listTariffTemplates(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.TariffTemplateListRequest) (any, error) {
		page, limit, err := kit.ResolvePage(req.PageRequest, pageCap)
		if err != nil {
			return nil, err
		}
		list, err := h.Tariffs.ListTemplates(r.Context(), mw.ScopeFrom(r), store.TariffTemplateFilter{Page: page})
		if err != nil {
			return nil, billingErr(err)
		}
		items := make([]dto.TariffTemplate, 0, len(list))
		for _, t := range list {
			item, err := templateDTO(t)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		return kit.PageOf(items, page, limit), nil
	})
}

func (h *Handlers) createTariffTemplate(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusCreated, func(req dto.TariffTemplateFields) (any, error) {
		created, err := h.Tariffs.CreateTemplate(r.Context(), mw.ScopeFrom(r), req.Name, req.Description,
			tariffInput(req.Tariff), req.IsDefault)
		if err != nil {
			return nil, billingErr(err)
		}
		return templateDTO(created)
	})
}

func (h *Handlers) updateTariffTemplate(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.TariffTemplateUpdateRequest) (any, error) {
		updated, err := h.Tariffs.UpdateTemplate(r.Context(), mw.ScopeFrom(r), req.ID, req.Name, req.Description,
			tariffInput(req.Tariff), req.IsDefault)
		if err != nil {
			return nil, billingErr(err)
		}
		return templateDTO(updated)
	})
}

func (h *Handlers) deleteTariffTemplate(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusNoContent, func(req dto.IDPath) (any, error) {
		return nil, billingErr(h.Tariffs.DeleteTemplate(r.Context(), mw.ScopeFrom(r), req.ID))
	})
}

func bulkResult(defs []tariffsvc.Definition) dto.BulkTariffResult {
	out := dto.BulkTariffResult{BuildingIDs: make([]uuid.UUID, 0, len(defs)), TariffIDs: make([]uuid.UUID, 0, len(defs))}
	for _, d := range defs {
		if d.Tariff.BuildingID != nil {
			out.BuildingIDs = append(out.BuildingIDs, *d.Tariff.BuildingID)
		}
		out.TariffIDs = append(out.TariffIDs, d.Tariff.ID)
	}
	return out
}

func (h *Handlers) applyTariffTemplate(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.TariffTemplateApplyRequest) (any, error) {
		defs, err := h.Tariffs.ApplyTemplate(r.Context(), mw.ScopeFrom(r), req.ID, req.BuildingIDs,
			req.EffectiveFrom.Time, actingUser(r))
		if err != nil {
			return nil, billingErr(err)
		}
		return bulkResult(defs), nil
	})
}

func (h *Handlers) bulkTariffAssign(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.BulkTariffRequest) (any, error) {
		defs, err := h.Tariffs.BulkAssign(r.Context(), mw.ScopeFrom(r), tariffInput(req.Tariff), req.BuildingIDs, actingUser(r))
		if err != nil {
			return nil, billingErr(err)
		}
		return bulkResult(defs), nil
	})
}

func (h *Handlers) bulkTariffCurrent(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.PageRequest) (any, error) {
		page, limit, err := kit.ResolvePage(req, pageCap)
		if err != nil {
			return nil, err
		}
		sc := mw.ScopeFrom(r)
		buildings, err := h.Assets.ListBuildings(r.Context(), sc, assets.BuildingListInput{Page: page})
		if err != nil {
			return nil, err
		}
		current, err := h.Tariffs.CurrentForBuildings(r.Context(), sc, h.Clock.Now())
		if err != nil {
			return nil, billingErr(err)
		}
		items := make([]dto.BuildingTariffState, 0, len(buildings))
		for _, b := range buildings {
			row := dto.BuildingTariffState{BuildingID: b.Building.ID, BuildingName: b.Building.Name}
			if def := current[b.Building.ID]; def != nil {
				effective := dto.Date{Time: def.Tariff.EffectiveFrom}
				row.TariffID, row.TariffName, row.EffectiveFrom = &def.Tariff.ID, def.Tariff.Name, &effective
				row.UsePtfYekdem = def.Tariff.UsePtfYekdem
			}
			items = append(items, row)
		}
		return kit.PageOf(items, page, limit), nil
	})
}

func (h *Handlers) bulkTariffHistory(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.PageRequest) (any, error) {
		page, limit, err := kit.ResolvePage(req, pageCap)
		if err != nil {
			return nil, err
		}
		list, err := h.Tariffs.BulkHistory(r.Context(), mw.ScopeFrom(r), page)
		if err != nil {
			return nil, billingErr(err)
		}
		items := make([]dto.BulkTariffAssignment, 0, len(list))
		for _, a := range list {
			items = append(items, dto.BulkTariffAssignment{
				ID: a.ID, TemplateID: a.TemplateID, TariffName: a.TariffName,
				EffectiveFrom: dto.Date{Time: a.EffectiveFrom}, BuildingIDs: a.BuildingIDs,
				CreatedBy: a.CreatedBy, CreatedAt: dto.T(a.CreatedAt),
			})
		}
		return kit.PageOf(items, page, limit), nil
	})
}

// icmalMaxBody is the upload cap for a supplier's icmal workbook. 1 MiB is
// right for JSON and far too small for a spreadsheet with a year of rows.
const icmalMaxBody = 10 << 20

func icmalRoutes() []Route {
	aca := auth.Roles(roleA, roleCA)
	return []Route{
		{Method: http.MethodPost, Pattern: "/icmal-imports", OperationID: "icmal.create", Tag: "tariffs", Access: RoleGated,
			Roles: aca, Entity: "icmal_import", Multipart: true, NoIdempotency: true, MaxBody: icmalMaxBody,
			Summary: "Upload and analyse an icmal; writes no tariff.", Response: dto.IcmalImport{},
			Status: http.StatusCreated, Handler: (*Handlers).createIcmalImport},
		{Method: http.MethodGet, Pattern: "/icmal-imports/{id}", OperationID: "icmal.get", Tag: "tariffs", Access: RoleGated,
			Roles: aca, Summary: "A stored icmal analysis.", Request: dto.IDPath{}, Response: dto.IcmalImport{},
			Status: http.StatusOK, Handler: (*Handlers).getIcmalImport},
		{Method: http.MethodPost, Pattern: "/icmal-imports/{id}/apply", OperationID: "icmal.apply", Tag: "tariffs",
			Access: RoleGated, Roles: aca, Entity: "tariff",
			Summary: "Write the confirmed coefficients into a new tariff version per building.",
			Request: dto.IcmalApplyRequest{}, Response: dto.BulkTariffResult{}, Status: http.StatusOK,
			Handler: (*Handlers).applyIcmalImport},
	}
}

func icmalCoefficientDTO(c icmal.Coefficient) dto.IcmalCoefficient {
	return dto.IcmalCoefficient{Value: dto.DP(c.Value), Samples: c.Samples, StdDev: dto.DP(c.StdDev),
		Stable: c.Stable, BackCalcErrorPct: dto.DP(c.BackCalcErrorPct)}
}

func icmalWarningsDTO(warnings []icmal.Warning) []dto.IcmalWarning {
	out := make([]dto.IcmalWarning, 0, len(warnings))
	for _, w := range warnings {
		out = append(out, dto.IcmalWarning{Row: w.Row, Code: w.Code, Text: w.Text})
	}
	return out
}

func icmalImportDTO(res tariffsvc.ImportResult) dto.IcmalImport {
	out := dto.IcmalImport{
		ID: res.Import.ID, FileName: res.Import.FileName, Status: res.Import.Status, RowCount: int(res.Import.RowCount),
		Analyses: make([]dto.IcmalAnalysis, 0, len(res.Analyses)), Unmatched: res.Unmatched,
		Warnings: icmalWarningsDTO(res.Warnings), CreatedAt: dto.T(res.Import.CreatedAt),
	}
	if out.Unmatched == nil {
		out.Unmatched = []string{}
	}
	for _, a := range res.Analyses {
		item := dto.IcmalAnalysis{
			EtsoCode: a.EtsoCode, Periods: a.Periods, EnergyKbk: icmalCoefficientDTO(a.EnergyKbk),
			DistributionTlPerKwh: icmalCoefficientDTO(a.DistributionTlPerKwh), PowerUnitPrice: icmalCoefficientDTO(a.PowerUnitPrice),
			ImpliedContractedPowerKw: dto.DP(a.ImpliedContractedPowerKw), ReactiveUnitPrice: icmalCoefficientDTO(a.ReactiveUnitPrice),
			ReactiveKbk: icmalCoefficientDTO(a.ReactiveKbk), VatRate: icmalCoefficientDTO(a.VatRate),
			Taxes: map[string]dto.IcmalCoefficient{}, IsMultiTime: a.IsMultiTime, Term: a.Term, VoltageLevel: a.VoltageLevel,
			WithinTolerance: a.WithinTolerance, OveruseRequiresManualEntry: a.OveruseRequiresManualEntry,
			Warnings: icmalWarningsDTO(a.Warnings),
		}
		if item.Periods == nil {
			item.Periods = []string{}
		}
		for name, c := range a.Taxes {
			item.Taxes[name] = icmalCoefficientDTO(c)
		}
		if id, ok := res.Buildings[a.EtsoCode]; ok {
			building := id
			item.BuildingID = &building
		}
		out.Analyses = append(out.Analyses, item)
	}
	return out
}

func (h *Handlers) createIcmalImport(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(icmalMaxBody); err != nil {
		kit.WriteError(w, r, kit.ErrBodyTooLarge)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		kit.WriteError(w, r, perr.Validation.WithParams(map[string]any{"file": []string{"required"}}))
		return
	}
	defer func() { _ = file.Close() }()
	content, err := io.ReadAll(file)
	if err != nil {
		kit.WriteError(w, r, kit.ErrBodyTooLarge)
		return
	}
	res, err := h.Tariffs.Import(r.Context(), mw.ScopeFrom(r), actingUser(r), header.Filename, content)
	if err != nil {
		// A file that cannot be parsed is the operator's problem to fix, and
		// naming the field is what lets the screen say so.
		if errors.Is(err, tariffsvc.ErrInvalidRequest) {
			kit.WriteError(w, r, perr.Validation.WithParams(map[string]any{"file": []string{"unreadable"}}))
			return
		}
		kit.WriteError(w, r, billingErr(err))
		return
	}
	kit.WriteJSON(w, http.StatusCreated, icmalImportDTO(res))
}

func (h *Handlers) getIcmalImport(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.IDPath) (any, error) {
		res, err := h.Tariffs.GetImport(r.Context(), mw.ScopeFrom(r), req.ID)
		if err != nil {
			return nil, billingErr(err)
		}
		return icmalImportDTO(res), nil
	})
}

func (h *Handlers) applyIcmalImport(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.IcmalApplyRequest) (any, error) {
		confirmations := make([]tariffsvc.ApplyConfirmation, 0, len(req.Confirmations))
		for _, c := range req.Confirmations {
			confirmation := tariffsvc.ApplyConfirmation{BuildingID: c.BuildingID, EtsoCode: c.EtsoCode, EffectiveFrom: c.EffectiveFrom.Time}
			if c.Base != nil {
				base := tariffInput(*c.Base)
				confirmation.Base = &base
			}
			confirmations = append(confirmations, confirmation)
		}
		defs, err := h.Tariffs.ApplyImport(r.Context(), mw.ScopeFrom(r), req.ID, confirmations)
		if err != nil {
			return nil, billingErr(err)
		}
		return bulkResult(defs), nil
	})
}
