package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/health"
)

const readyCheckTimeout = 3 * time.Second

func liveHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func readyHandler(checks []health.Check) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report := health.Run(r.Context(), readyCheckTimeout, checks...)
		status := http.StatusOK
		if !report.Healthy() {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, report)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
