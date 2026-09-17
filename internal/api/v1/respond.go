package v1

import (
	"net/http"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/kit"
)

// serve binds Req, runs fn and writes its result with status (204 writes no body).
func serve[Req any](w http.ResponseWriter, r *http.Request, status int, fn func(req Req) (any, error)) {
	var req Req
	if err := kit.Bind(r, &req); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	out, err := fn(req)
	if err != nil {
		kit.WriteError(w, r, err)
		return
	}
	if status == http.StatusNoContent {
		kit.NoContent(w)
		return
	}
	kit.WriteJSON(w, status, out)
}
