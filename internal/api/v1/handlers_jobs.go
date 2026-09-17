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
	}}
}

func (h *Handlers) getJob(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.JobIDPath) (any, error) {
		v, err := h.Jobs.Get(r.Context(), mw.ScopeFrom(r), req.ID)
		if err != nil {
			return nil, err
		}
		return dto.Job{ID: v.ID, Type: v.Type, Status: string(v.Status), CompletedAt: dto.TP(v.CompletedAt)}, nil
	})
}
