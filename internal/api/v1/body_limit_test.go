package v1_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/MErenTalan/ekokod-rewrite/internal/api/v1"
)

// The global 1 MiB cap is right for JSON and wrong for a spreadsheet upload,
// so the limit is per route. An inner MaxBytesReader cannot lift an outer one,
// which is why the router applies exactly one — the route's own.
func TestBodyLimitIsPerRoute(t *testing.T) {
	t.Parallel()
	echo := func(_ *v1.Handlers, w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
	table := []v1.Route{
		{Method: http.MethodPost, Pattern: "/small", OperationID: "test.small", Tag: "test", Entity: "test",
			Access: v1.Public, Status: http.StatusOK, Handler: echo},
		{Method: http.MethodPost, Pattern: "/big", OperationID: "test.big", Tag: "test", Entity: "test",
			Access: v1.Public, Status: http.StatusOK, MaxBody: 4 << 20, Handler: echo},
	}
	router := v1.NewRouterForTable(table, &v1.Handlers{}, passthrough(nil), nil)

	post := func(path string, size int) int {
		body := strings.NewReader(strings.Repeat("a", size))
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path, body)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}

	require.Equal(t, http.StatusRequestEntityTooLarge, post("/small", 2<<20), "an ordinary route keeps the 1 MiB cap")
	require.Equal(t, http.StatusOK, post("/big", 2<<20), "the route's own limit applies")
	require.Equal(t, http.StatusRequestEntityTooLarge, post("/big", 5<<20), "and still bounds it")
	require.Equal(t, http.StatusOK, post("/small", 1024))
}
