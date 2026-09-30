package v1

import (
	"net/http"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
)

func jobRoutes() []Route {
	return []Route{{
		Method: http.MethodGet, Pattern: "/jobs/{id}", OperationID: "jobs.get", Tag: "jobs", Access: RoleGated,
		Roles: auth.Roles(roleA, roleCA, roleBA), Summary: "The state of a background job this user started (R192).",
		Request: dto.JobIDPath{}, Response: dto.Job{}, Status: http.StatusOK, Handler: (*Handlers).getJob,
	}, {
		Method: http.MethodGet, Pattern: "/job-queues", OperationID: "jobs.queues", Tag: "jobs", Access: RoleGated,
		Roles: auth.Roles(roleA), Summary: "Current load of every background queue (platform-wide).",
		Request: struct{}{}, Response: dto.JobQueues{}, Status: http.StatusOK, Handler: (*Handlers).listJobQueues,
	}}
}

func (h *Handlers) listJobQueues(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(struct{}) (any, error) {
		qs, err := h.Jobs.Queues()
		if err != nil {
			return nil, err
		}
		out := dto.JobQueues{Items: make([]dto.JobQueue, len(qs))}
		for i, q := range qs {
			out.Items[i] = dto.JobQueue{Queue: q.Queue, Pending: q.Pending, Active: q.Active, Scheduled: q.Scheduled,
				Retry: q.Retry, Archived: q.Archived, Completed: q.Completed, ProcessedToday: q.ProcessedToday,
				FailedToday: q.FailedToday, Paused: q.Paused, LatencySeconds: int64(q.Latency.Seconds())}
		}
		return out, nil
	})
}

func (h *Handlers) getJob(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.JobIDPath) (any, error) {
		v, err := h.Jobs.Get(r.Context(), mw.ScopeFrom(r), req.ID)
		if err != nil {
			return nil, err
		}
		out := dto.Job{ID: v.ID, Type: v.Type, Status: string(v.Status), CompletedAt: dto.TP(v.CompletedAt), ErrorCode: v.ErrorCode}
		if c := v.Children; c != nil {
			out.Children = &dto.JobChildren{Enqueued: c.Enqueued, AlreadyQueued: c.AlreadyQueued, Running: c.Running,
				Succeeded: c.Succeeded, Partial: c.Partial, Failed: c.Failed, Errors: make([]dto.JobChildError, len(c.Errors))}
			for i, e := range c.Errors {
				out.Children.Errors[i] = dto.JobChildError{Message: e.Message, Count: e.Count}
			}
		}
		return out, nil
	})
}
