package api

import (
	"net/http"

	"github.com/MErenTalan/ekokod-rewrite/internal/buildinfo"
)

func versionHandler(info buildinfo.Info) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, info)
	}
}
