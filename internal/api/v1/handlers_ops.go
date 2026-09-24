package v1

import (
	"net/http"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/kit"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

func opsRoutes() []Route {
	all := auth.AllRoles
	aca := auth.Roles(roleA, roleCA)
	a := auth.Roles(roleA)
	return []Route{
		{Method: http.MethodGet, Pattern: "/messages", OperationID: "messages.list", Tag: "messages", Access: RoleGated,
			Roles: all, Summary: "Operational messages for the tenant.", Request: dto.MessagesRequest{},
			Response: dto.Page[dto.Message]{}, Status: http.StatusOK, Handler: (*Handlers).listMessages},
		{Method: http.MethodGet, Pattern: "/job-runs", OperationID: "jobRuns.list", Tag: "messages", Access: RoleGated,
			Roles: aca, Summary: "Job execution history with counts and errors.", Request: dto.JobRunsRequest{},
			Response: dto.Page[dto.JobRun]{}, Status: http.StatusOK, Handler: (*Handlers).listJobRuns},
		// NoIdempotency: re-running a job deliberately is the point of the
		// button; replaying the first response would make the second click a
		// silent no-op.
		{Method: http.MethodPost, Pattern: "/job-runs/{type}/trigger", OperationID: "jobRuns.trigger", Tag: "messages",
			Access: RoleGated, Roles: a, Entity: "job_run", NoIdempotency: true,
			Summary: "Trigger an allow-listed job for the caller's company.", Request: dto.JobTriggerRequest{},
			Response: dto.JobAccepted{}, Status: http.StatusAccepted, Handler: (*Handlers).triggerJobRun},
	}
}

func (h *Handlers) listMessages(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.MessagesRequest) (any, error) {
		page, limit, err := kit.ResolvePage(q.PageRequest, pageCap)
		if err != nil {
			return nil, err
		}
		f := store.MessageFilter{Q: q.Q, Page: page}
		if q.Kind != nil {
			f.Kinds = []string{*q.Kind}
		}
		if q.Status != nil {
			f.Statuses = []string{*q.Status}
		}
		if q.From != nil && q.To != nil {
			f.Range = &store.TimeRange{From: *q.From, To: *q.To}
		}
		list, err := h.Ops.Messages(r.Context(), mw.ScopeFrom(r), f)
		if err != nil {
			return nil, err
		}
		items := make([]dto.Message, 0, len(list))
		for _, m := range list {
			items = append(items, dto.Message{
				ID: m.ID, Kind: m.Kind, Category: m.Category, Status: m.Status, Message: m.Message,
				Detail: m.Detail, RelatedType: m.RelatedType, RelatedID: m.RelatedID, CreatedAt: dto.T(m.CreatedAt),
			})
		}
		return kit.PageOf(items, page, limit), nil
	})
}

func (h *Handlers) listJobRuns(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.JobRunsRequest) (any, error) {
		page, limit, err := kit.ResolvePage(q.PageRequest, pageCap)
		if err != nil {
			return nil, err
		}
		f := store.JobRunFilter{JobType: q.JobType, Page: page}
		if q.Status != nil {
			f.Statuses = []string{*q.Status}
		}
		list, err := h.Ops.JobRuns(r.Context(), mw.ScopeFrom(r), f)
		if err != nil {
			return nil, err
		}
		items := make([]dto.JobRun, 0, len(list))
		for _, run := range list {
			items = append(items, dto.JobRun{
				ID: run.ID, JobType: run.JobType, Scope: run.Scope, StartedAt: dto.T(run.StartedAt),
				FinishedAt: dto.TP(run.FinishedAt), Status: run.Status,
				Processed: run.Processed, Skipped: run.Skipped, Failed: run.Failed, Error: run.Error,
			})
		}
		return kit.PageOf(items, page, limit), nil
	})
}

func (h *Handlers) triggerJobRun(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusAccepted, func(q dto.JobTriggerRequest) (any, error) {
		id, err := h.Ops.Trigger(r.Context(), mw.ScopeFrom(r), q.Type)
		if err != nil {
			return nil, err
		}
		return dto.JobAccepted{JobID: id}, nil
	})
}
