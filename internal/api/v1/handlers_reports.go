package v1

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/kit"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
	reportsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// reportRoutes is 05 §11. /reports/preview is a literal segment, so it is
// never read as an {id}.
func reportRoutes() []Route {
	all := auth.AllRoles
	writers := auth.Roles(roleA, roleCA, roleBA)
	return []Route{
		{Method: http.MethodGet, Pattern: "/reports", OperationID: "reports.list", Tag: "reports", Access: RoleGated, Roles: all,
			Summary: "The report archive, newest first, with the whole match's count.", Request: dto.ReportListRequest{},
			Response: dto.ReportPage{}, Status: http.StatusOK, Handler: (*Handlers).listReports},
		{Method: http.MethodGet, Pattern: "/reports/preview", OperationID: "reports.preview", Tag: "reports", Access: RoleGated, Roles: all,
			Summary: "Compute report figures for buildings and a period without persisting them.", Request: dto.ReportPreviewRequest{},
			Response: dto.ReportPayload{}, Status: http.StatusOK, Handler: (*Handlers).previewReport},
		{Method: http.MethodPost, Pattern: "/reports/generate", OperationID: "reports.generate", Tag: "reports", Access: RoleGated,
			Roles: writers, Entity: "report", Summary: "Generate one report per building; returns each building's job.",
			Request: dto.ReportGenerateRequest{}, Response: dto.ReportGenerateAccepted{}, Status: http.StatusAccepted,
			Handler: (*Handlers).generateReports},
		{Method: http.MethodGet, Pattern: "/reports/{id}", OperationID: "reports.get", Tag: "reports", Access: RoleGated, Roles: all,
			Summary: "A stored report with its figures.", Request: dto.IDPath{}, Response: dto.Report{}, Status: http.StatusOK,
			Handler: (*Handlers).getReport},
		{Method: http.MethodGet, Pattern: "/reports/{id}/pdf", OperationID: "reports.pdf", Tag: "reports", Access: RoleGated, Roles: all,
			Summary: "The report as PDF, in the reader's language.", Request: dto.IDPath{}, Status: http.StatusOK,
			RawContentType: "application/pdf", Handler: (*Handlers).reportPDF},
		{Method: http.MethodGet, Pattern: "/reports/{id}/excel", OperationID: "reports.excel", Tag: "reports", Access: RoleGated, Roles: all,
			Summary: "The report as a workbook, in the reader's language.", Request: dto.IDPath{}, Status: http.StatusOK,
			RawContentType: xlsxType, Handler: (*Handlers).reportExcel},
		{Method: http.MethodPost, Pattern: "/reports/{id}/email", OperationID: "reports.email", Tag: "reports", Access: RoleGated,
			Roles: writers, Entity: "report", Summary: "E-mail the report with its PDF and workbook attached.",
			Request: dto.ReportEmailRequest{}, Response: dto.ReportEmailAccepted{}, Status: http.StatusAccepted,
			Handler: (*Handlers).emailReport},
	}
}

func reportRequest(kind, period, selection string, buildings, plants []uuid.UUID) reportsvc.Request {
	return reportsvc.Request{Type: kind, Period: period, Selection: selection, BuildingIDs: buildings, PlantIDs: plants}
}

func (h *Handlers) listReports(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.ReportListRequest) (any, error) {
		page, limit, err := kit.ResolvePage(req.PageRequest, pageCap)
		if err != nil {
			return nil, err
		}
		f := store.ReportFilter{BuildingID: req.BuildingID, Year: req.Year, Page: page}
		if req.Type != nil {
			t := model.ReportType(*req.Type)
			f.Type = &t
		}
		items, total, err := h.Reports.Archive(r.Context(), mw.ScopeFrom(r), f)
		if err != nil {
			return nil, err
		}
		rows := make([]dto.ReportSummary, len(items))
		for i, it := range items {
			rows[i] = reportSummary(it)
		}
		p := kit.PageOf(rows, page, limit)
		return dto.ReportPage{Items: p.Items, NextCursor: p.NextCursor, Total: total}, nil
	})
}

func (h *Handlers) previewReport(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.ReportPreviewRequest) (any, error) {
		p, err := h.Reports.Service.Preview(r.Context(), mw.ScopeFrom(r),
			reportRequest(req.Type, req.Period, req.PlantSelection, req.BuildingIDs, req.PlantIDs))
		if err != nil {
			return nil, err
		}
		return payloadDTO(p)
	})
}

func (h *Handlers) generateReports(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusAccepted, func(req dto.ReportGenerateRequest) (any, error) {
		out, err := h.Reports.Enqueue(r.Context(), mw.ScopeFrom(r),
			reportRequest(req.Type, req.Period, req.PlantSelection, req.BuildingIDs, req.PlantIDs))
		if err != nil {
			return nil, err
		}
		items := make([]dto.ReportGenerateItem, len(out))
		for i, e := range out {
			items[i] = dto.ReportGenerateItem{BuildingID: e.BuildingID, ReportID: e.ReportID, JobID: e.JobID}
		}
		return dto.ReportGenerateAccepted{Items: items}, nil
	})
}

func (h *Handlers) getReport(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.IDPath) (any, error) {
		it, err := h.Reports.Get(r.Context(), mw.ScopeFrom(r), req.ID)
		if err != nil {
			return nil, err
		}
		out := dto.Report{ReportSummary: reportSummary(it)}
		// A pending report's payload is {}: it has no figures to show yet.
		var p dto.ReportPayload
		if err := json.Unmarshal(it.Report.Payload, &p); err == nil && (p.Monthly != nil || p.Yearly != nil) {
			out.Payload = &p
		}
		return out, nil
	})
}

func (h *Handlers) reportFile(w http.ResponseWriter, r *http.Request, format, contentType string) {
	var req dto.IDPath
	if err := kit.Bind(r, &req); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	body, name, err := h.Reports.File(r.Context(), mw.ScopeFrom(r), req.ID, format, kit.Locale(r))
	if err != nil {
		kit.WriteError(w, r, err)
		return
	}
	writeFile(w, contentType, name, body)
}

func (h *Handlers) reportPDF(w http.ResponseWriter, r *http.Request) {
	h.reportFile(w, r, reportsvc.FormatPDF, "application/pdf")
}

func (h *Handlers) reportExcel(w http.ResponseWriter, r *http.Request) {
	h.reportFile(w, r, reportsvc.FormatExcel, xlsxType)
}

func (h *Handlers) emailReport(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusAccepted, func(req dto.ReportEmailRequest) (any, error) {
		id, err := h.Reports.EnqueueDelivery(r.Context(), mw.ScopeFrom(r), req.ID, req.To)
		if err != nil {
			return nil, err
		}
		return dto.ReportEmailAccepted{JobID: id}, nil
	})
}

func reportSummary(it reportsvc.ArchiveItem) dto.ReportSummary {
	rp := it.Report
	return dto.ReportSummary{ID: rp.ID, BuildingID: rp.BuildingID, BuildingName: it.BuildingName, Type: string(rp.Type),
		Period: rp.Period, PlantSelection: string(rp.PlantSelection), Status: string(rp.Status), ProcessedAt: rp.ProcessedAt,
		CreatedAt: rp.CreatedAt}
}

// payloadDTO maps the domain payload onto the wire through its JSON: the
// field names are the same snake_case on both sides, and a stored payload
// decodes into the DTO the same way (TestReportPayloadDTOKeepsEveryField).
func payloadDTO(p domain.Payload) (dto.ReportPayload, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return dto.ReportPayload{}, err
	}
	var out dto.ReportPayload
	return out, json.Unmarshal(raw, &out)
}
