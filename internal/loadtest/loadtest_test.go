package loadtest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/loadtest"
)

// fakeAPI answers like the real one: a Secure session cookie, pages of ids, a
// slow consumption query and a failing bills list.
func fakeAPI(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "ekokod_at", Value: "AT", Secure: true, HttpOnly: true})
		w.WriteHeader(http.StatusNoContent)
	})
	authed := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.Header.Get("Cookie"), "ekokod_at=AT") {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			h(w, r)
		}
	}
	items := authed(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]string{{"id": "a"}, {"id": "b"}}})
	})
	mux.HandleFunc("GET /api/v1/buildings", items)
	mux.HandleFunc("GET /api/v1/analyzers", items)
	mux.HandleFunc("GET /api/v1/consumption", authed(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(40 * time.Millisecond)
		_, _ = w.Write([]byte(`{}`))
	}))
	mux.HandleFunc("GET /api/v1/bills", authed(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	for _, p := range []string{"/api/v1/alarms", "/api/v1/reports"} {
		mux.HandleFunc("GET "+p, items)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestLoadTestMeasuresAndJudges(t *testing.T) {
	srv := fakeAPI(t)
	res, err := loadtest.Run(context.Background(), loadtest.Options{Base: srv.URL, Email: "e", Password: "p", Users: 4,
		Duration: 1500 * time.Millisecond, P95Budget: 20 * time.Millisecond}, &bytes.Buffer{})
	require.NoError(t, err)
	require.Positive(t, res.Requests)
	require.False(t, res.Pass)
	joined := strings.Join(res.Failures, "\n")
	require.Contains(t, joined, "consumption hourly (building, day): p95", "the slow endpoint breaks the budget")
	require.Contains(t, joined, "bills:", "the failing endpoint breaks the error ratio")
	require.NotContains(t, joined, "alarms", "a fast, healthy endpoint passes")
	for _, e := range res.Endpoints {
		if e.Name == "alarms" {
			require.Zero(t, e.Errors, "the Secure session cookie was sent over plain HTTP")
		}
	}
}
