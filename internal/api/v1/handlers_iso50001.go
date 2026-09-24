package v1

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/kit"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/iso50001"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	isosvc "github.com/MErenTalan/ekokod-rewrite/internal/service/iso50001"
)

// isoMaxBody bounds the upload request; the store's configured cap (30 MB,
// R336) sits below it, so the store is what refuses an oversized file.
const isoMaxBody = 64 << 20

// iso50001Routes is 05 §13 (R330–R341): reads are every scope, writes A CA BA.
func iso50001Routes() []Route {
	all, write := auth.AllRoles, auth.Roles(roleA, roleCA, roleBA)
	return []Route{
		{Method: http.MethodGet, Pattern: "/iso50001/clauses", OperationID: "iso50001.clauses", Tag: "iso50001", Access: Authenticated,
			Summary: "TS EN ISO 50001:2018 clauses 5–9 with every sub-clause's text, in the request locale.", Response: dto.ISOClauses{},
			Status: http.StatusOK, Handler: (*Handlers).isoClauses},
		{Method: http.MethodGet, Pattern: "/iso50001/templates", OperationID: "iso50001.templates", Tag: "iso50001", Access: Authenticated,
			Summary: "Downloadable clause templates.", Response: dto.ISOTemplates{}, Status: http.StatusOK, Handler: (*Handlers).isoTemplates},
		{Method: http.MethodGet, Pattern: "/iso50001/templates/{id}", OperationID: "iso50001.template", Tag: "iso50001", Access: Authenticated,
			Summary: "One template file.", Request: dto.ISOTemplatePath{}, Status: http.StatusOK, RawContentType: "application/octet-stream",
			Handler: (*Handlers).isoTemplate},
		{Method: http.MethodGet, Pattern: "/iso50001/{building_id}", OperationID: "iso50001.project", Tag: "iso50001", Access: RoleGated,
			Roles: all, Summary: "Project state: clause dates and Gantt statuses, per-sub-clause counts, progress.",
			Request: dto.ISOBuildingPath{}, Response: dto.ISOProject{}, Status: http.StatusOK, Handler: (*Handlers).isoProject},
		{Method: http.MethodPut, Pattern: "/iso50001/{building_id}/dates", OperationID: "iso50001.dates", Tag: "iso50001", Access: RoleGated,
			Roles: write, Entity: "iso50001_project", Summary: "Set every main clause's start and end dates.",
			Request: dto.ISODatesRequest{}, Response: dto.ISOProject{}, Status: http.StatusOK, Handler: (*Handlers).isoDates},
		{Method: http.MethodGet, Pattern: "/iso50001/{building_id}/clauses/{clause}/notes", OperationID: "iso50001.notes", Tag: "iso50001",
			Access: RoleGated, Roles: all, Summary: "A sub-clause's notes, newest first.", Request: dto.ISOClausePath{},
			Response: dto.ISONotes{}, Status: http.StatusOK, Handler: (*Handlers).isoNotes},
		{Method: http.MethodPost, Pattern: "/iso50001/{building_id}/clauses/{clause}/notes", OperationID: "iso50001.notes.create", Tag: "iso50001",
			Access: RoleGated, Roles: write, Entity: "iso50001_note", Summary: "Add a note.", Request: dto.ISONoteCreateRequest{},
			Response: dto.ISONote{}, Status: http.StatusCreated, Handler: (*Handlers).isoCreateNote},
		{Method: http.MethodPatch, Pattern: "/iso50001/notes/{id}", OperationID: "iso50001.notes.update", Tag: "iso50001", Access: RoleGated,
			Roles: write, Entity: "iso50001_note", Summary: "Edit a note.", Request: dto.ISONoteUpdateRequest{}, Response: dto.ISONote{},
			Status: http.StatusOK, Handler: (*Handlers).isoUpdateNote},
		{Method: http.MethodDelete, Pattern: "/iso50001/notes/{id}", OperationID: "iso50001.notes.delete", Tag: "iso50001", Access: RoleGated,
			Roles: write, Entity: "iso50001_note", Summary: "Delete a note.", Request: dto.IDPath{}, Status: http.StatusNoContent,
			Handler: (*Handlers).isoDeleteNote},
		{Method: http.MethodGet, Pattern: "/iso50001/{building_id}/clauses/{clause}/files", OperationID: "iso50001.files", Tag: "iso50001",
			Access: RoleGated, Roles: all, Summary: "A sub-clause's evidence files.", Request: dto.ISOClausePath{}, Response: dto.ISOFiles{},
			Status: http.StatusOK, Handler: (*Handlers).isoFiles},
		{Method: http.MethodPost, Pattern: "/iso50001/{building_id}/clauses/{clause}/files", OperationID: "iso50001.files.upload", Tag: "iso50001",
			Access: RoleGated, Roles: write, Entity: "iso50001_file", Multipart: true, NoIdempotency: true, MaxBody: isoMaxBody,
			Summary: "Upload one evidence file (multipart field `file`; size and type checked, bytes sniffed).", Request: dto.ISOClausePath{},
			Response: dto.ISOFile{}, Status: http.StatusCreated, Handler: (*Handlers).isoUpload},
		{Method: http.MethodGet, Pattern: "/iso50001/files/{id}", OperationID: "iso50001.files.download", Tag: "iso50001", Access: RoleGated,
			Roles: all, Summary: "The evidence file, served only through this authorising handler.", Request: dto.IDPath{},
			Status: http.StatusOK, RawContentType: "application/octet-stream", Handler: (*Handlers).isoDownload},
		{Method: http.MethodDelete, Pattern: "/iso50001/files/{id}", OperationID: "iso50001.files.delete", Tag: "iso50001", Access: RoleGated,
			Roles: write, Entity: "iso50001_file", Summary: "Delete an evidence file.", Request: dto.IDPath{}, Status: http.StatusNoContent,
			Handler: (*Handlers).isoDeleteFile},
		{Method: http.MethodGet, Pattern: "/iso50001/{building_id}/export", OperationID: "iso50001.export", Tag: "iso50001", Access: RoleGated,
			Roles: all, Summary: "Every note and file as a zip, streamed.", Request: dto.ISOBuildingPath{}, Status: http.StatusOK,
			RawContentType: "application/zip", Handler: (*Handlers).isoExport},
	}
}

func localized(m map[string]string, locale string) string {
	if v := m[locale]; v != "" {
		return v
	}
	return m["tr"]
}

func (h *Handlers) isoClauses(w http.ResponseWriter, r *http.Request) {
	locale := kit.Locale(r)
	out := dto.ISOClauses{Items: []dto.ISOClause{}}
	for _, m := range domain.Mains() {
		c := dto.ISOClause{ID: m.ID, Title: localized(m.Title, locale), Subs: []dto.ISOSubClause{}}
		for _, s := range m.Subs {
			sub := dto.ISOSubClause{ID: s.ID, Title: localized(s.Title, locale), Description: localized(s.Description, locale)}
			if s.Template != "" {
				tpl := s.Template
				sub.Template = &tpl
			}
			c.Subs = append(c.Subs, sub)
		}
		out.Items = append(out.Items, c)
	}
	kit.WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) isoTemplates(w http.ResponseWriter, r *http.Request) {
	locale := kit.Locale(r)
	out := dto.ISOTemplates{Items: []dto.ISOTemplate{}}
	for _, t := range h.ISO.Templates() {
		out.Items = append(out.Items, dto.ISOTemplate{ID: t.ID, FileName: t.FileName, Clauses: t.Clauses, Description: localized(t.Description, locale)})
	}
	kit.WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) isoTemplate(w http.ResponseWriter, r *http.Request) {
	var req dto.ISOTemplatePath
	if err := kit.Bind(r, &req); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	body, name, contentType, err := h.ISO.Template(req.ID)
	if err != nil {
		kit.WriteError(w, r, err)
		return
	}
	writeFile(w, contentType, name, body)
}

func isoProjectDTO(st isosvc.State) dto.ISOProject {
	out := dto.ISOProject{Progress: st.Progress, GanttAvailable: st.GanttAvailable, Clauses: []dto.ISOClauseState{}, Counts: []dto.ISOCount{}}
	if st.ProjectStart != nil {
		out.ProjectStart = &dto.Date{Time: *st.ProjectStart}
	}
	if st.ProjectEnd != nil {
		out.ProjectEnd = &dto.Date{Time: *st.ProjectEnd}
	}
	for _, id := range domain.MainIDs {
		c := dto.ISOClauseState{ClauseID: id}
		if d, ok := st.Dates[id]; ok {
			c.Start, c.End = &dto.Date{Time: d.Start}, &dto.Date{Time: d.End}
		}
		if s, ok := st.Statuses[id]; ok {
			status := s
			c.Status = &status
		}
		out.Clauses = append(out.Clauses, c)
	}
	for _, id := range domain.SubIDs() {
		c := st.Counts[id]
		out.Counts = append(out.Counts, dto.ISOCount{ClauseID: id, Notes: c.Notes, Files: c.Files})
	}
	return out
}

func (h *Handlers) isoProject(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.ISOBuildingPath) (any, error) {
		st, err := h.ISO.Project(r.Context(), mw.ScopeFrom(r), req.BuildingID)
		if err != nil {
			return nil, err
		}
		return isoProjectDTO(st), nil
	})
}

func (h *Handlers) isoDates(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.ISODatesRequest) (any, error) {
		rows := make([]domain.ClauseDates, len(req.Clauses))
		for i, c := range req.Clauses {
			rows[i].ClauseID = c.ClauseID
			if c.Start != nil {
				rows[i].Start = c.Start.Time
			}
			if c.End != nil {
				rows[i].End = c.End.Time
			}
		}
		st, err := h.ISO.SetDates(r.Context(), mw.ScopeFrom(r), req.BuildingID, rows)
		if err != nil {
			return nil, err
		}
		return isoProjectDTO(st), nil
	})
}

func isoNoteDTO(n model.ISO50001Note) dto.ISONote {
	return dto.ISONote{ID: n.ID, ClauseID: n.ClauseID, Title: n.Title, Body: n.Body, CreatedBy: n.CreatedBy,
		CreatedAt: dto.T(n.CreatedAt), UpdatedAt: dto.T(n.UpdatedAt)}
}

func (h *Handlers) isoNotes(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.ISOClausePath) (any, error) {
		list, err := h.ISO.Notes(r.Context(), mw.ScopeFrom(r), req.BuildingID, req.Clause)
		if err != nil {
			return nil, err
		}
		out := dto.ISONotes{Items: make([]dto.ISONote, len(list))}
		for i, n := range list {
			out.Items[i] = isoNoteDTO(n)
		}
		return out, nil
	})
}

func (h *Handlers) isoCreateNote(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusCreated, func(req dto.ISONoteCreateRequest) (any, error) {
		n, err := h.ISO.CreateNote(r.Context(), mw.ScopeFrom(r), actingUser(r), req.BuildingID, req.Clause, req.Title, req.Body)
		if err != nil {
			return nil, err
		}
		return isoNoteDTO(n), nil
	})
}

func (h *Handlers) isoUpdateNote(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.ISONoteUpdateRequest) (any, error) {
		n, err := h.ISO.UpdateNote(r.Context(), mw.ScopeFrom(r), req.ID, req.ClauseID, req.Title, req.Body)
		if err != nil {
			return nil, err
		}
		return isoNoteDTO(n), nil
	})
}

func (h *Handlers) isoDeleteNote(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusNoContent, func(req dto.IDPath) (any, error) {
		return nil, h.ISO.DeleteNote(r.Context(), mw.ScopeFrom(r), req.ID)
	})
}

func isoFileDTO(f model.StoredFile) dto.ISOFile {
	clause := ""
	if f.ClauseID != nil {
		clause = *f.ClauseID
	}
	return dto.ISOFile{ID: f.ID, ClauseID: clause, Name: f.OriginalName, ContentType: f.ContentType, SizeBytes: f.SizeBytes,
		CreatedAt: dto.T(f.CreatedAt)}
}

func (h *Handlers) isoFiles(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.ISOClausePath) (any, error) {
		list, err := h.ISO.Files(r.Context(), mw.ScopeFrom(r), req.BuildingID, req.Clause)
		if err != nil {
			return nil, err
		}
		out := dto.ISOFiles{Items: make([]dto.ISOFile, len(list))}
		for i, f := range list {
			out.Items[i] = isoFileDTO(f)
		}
		return out, nil
	})
}

// isoUpload streams the `file` part straight into the store: the upload is
// never buffered whole in memory or in a multipart temp file.
func (h *Handlers) isoUpload(w http.ResponseWriter, r *http.Request) {
	var req dto.ISOClausePath
	if err := kit.Bind(r, &req); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	mr, err := r.MultipartReader()
	if err != nil {
		kit.WriteError(w, r, perr.Validation.WithParams(map[string]any{"file": []string{"required"}}))
		return
	}
	for {
		part, err := mr.NextPart()
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				kit.WriteError(w, r, isosvc.ErrFileTooLarge.WithParams(map[string]any{"max_bytes": h.UploadMax}))
				return
			}
			kit.WriteError(w, r, perr.Validation.WithParams(map[string]any{"file": []string{"required"}}))
			return
		}
		if part.FormName() != "file" {
			_ = part.Close()
			continue
		}
		f, err := h.ISO.Upload(r.Context(), mw.ScopeFrom(r), actingUser(r), req.BuildingID, req.Clause, part.FileName(), part)
		_ = part.Close()
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			err = isosvc.ErrFileTooLarge.WithParams(map[string]any{"max_bytes": h.UploadMax})
		}
		if err != nil {
			kit.WriteError(w, r, err)
			return
		}
		kit.WriteJSON(w, http.StatusCreated, isoFileDTO(f))
		return
	}
}

// contentDisposition carries an ASCII fallback and the RFC 5987 UTF-8 name.
func contentDisposition(name string) string {
	ascii := strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii, url.PathEscape(name))
}

func (h *Handlers) isoDownload(w http.ResponseWriter, r *http.Request) {
	var req dto.IDPath
	if err := kit.Bind(r, &req); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	f, body, err := h.ISO.Download(r.Context(), mw.ScopeFrom(r), req.ID)
	if err != nil {
		kit.WriteError(w, r, err)
		return
	}
	defer func() { _ = body.Close() }()
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Disposition", contentDisposition(f.OriginalName))
	w.Header().Set("Content-Length", strconv.FormatInt(f.SizeBytes, 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
}

func (h *Handlers) isoDeleteFile(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusNoContent, func(req dto.IDPath) (any, error) {
		return nil, h.ISO.DeleteFile(r.Context(), mw.ScopeFrom(r), req.ID)
	})
}

// lazyZip sets the zip headers on the first byte, so a refusal before any
// output (another building) still answers as a normal error.
type lazyZip struct {
	w       http.ResponseWriter
	name    string
	started bool
}

func (z *lazyZip) Write(p []byte) (int, error) {
	if !z.started {
		z.started = true
		z.w.Header().Set("Content-Type", "application/zip")
		z.w.Header().Set("Content-Disposition", contentDisposition(z.name))
		z.w.Header().Set("Cache-Control", "private, no-store")
		z.w.WriteHeader(http.StatusOK)
	}
	return z.w.Write(p)
}

func (h *Handlers) isoExport(w http.ResponseWriter, r *http.Request) {
	var req dto.ISOBuildingPath
	if err := kit.Bind(r, &req); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	out := &lazyZip{w: w, name: "ISO-50001-" + time.Now().In(dto.Istanbul).Format("2006-01-02") + ".zip"}
	if err := h.ISO.Export(r.Context(), mw.ScopeFrom(r), req.BuildingID, kit.Locale(r), out); err != nil && !out.started {
		kit.WriteError(w, r, err)
	}
}
