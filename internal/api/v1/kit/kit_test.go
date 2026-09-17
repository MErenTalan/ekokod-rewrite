package kit_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/kit"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

type sampleRequest struct {
	ID       uuid.UUID    `path:"id" json:"-"`
	From     *dto.Date    `query:"from" json:"-"`
	Kinds    []string     `query:"kinds" json:"-"`
	Active   *bool        `query:"is_active" json:"-"`
	Name     string       `json:"name" validate:"required"`
	Area     *dto.Decimal `json:"total_area_m2,omitempty"`
	Children []child      `json:"children,omitempty" validate:"dive"`
	dto.PageRequest
}

type child struct {
	Rate *dto.Decimal `json:"rate" validate:"required"`
}

func bind(t *testing.T, method, target, body string) (sampleRequest, error) {
	t.Helper()
	var got sampleRequest
	var err error
	r := chi.NewRouter()
	r.MethodFunc(method, "/things/{id}", func(_ http.ResponseWriter, req *http.Request) { err = kit.Bind(req, &got) })
	req := httptest.NewRequestWithContext(context.Background(), method, target, strings.NewReader(body))
	r.ServeHTTP(httptest.NewRecorder(), req)
	return got, err
}

const thing = "/things/9b2a4b5e-2f7c-4c43-8f0e-7c1e5b1c9a10"

type errInfo struct {
	Params map[string]any
	Status int
}

func requireCode(t *testing.T, err error, code string) errInfo {
	t.Helper()
	var e *perr.Error
	require.ErrorAs(t, err, &e)
	require.Equal(t, code, e.Code)
	return errInfo{Params: e.Params, Status: e.HTTPStatus}
}

func TestBindFillsPathQueryAndBody(t *testing.T) {
	got, err := bind(t, http.MethodPatch, thing+"?from=2026-03-01&kinds=a,b&kinds=c&is_active=true&limit=5&company_id="+uuid.NewString(),
		`{"name":"Fabrika","total_area_m2":"1250.50","children":[{"rate":"20"}]}`)
	require.NoError(t, err)
	require.Equal(t, "9b2a4b5e-2f7c-4c43-8f0e-7c1e5b1c9a10", got.ID.String())
	require.Equal(t, "2026-03-01", got.From.Format("2006-01-02"))
	require.Equal(t, "Europe/Istanbul", got.From.Location().String())
	require.Equal(t, []string{"a", "b", "c"}, got.Kinds)
	require.True(t, *got.Active)
	require.Equal(t, int32(5), *got.Limit)
	require.Equal(t, "1250.5", got.Area.String())
}

func TestBindRejectsUnknownQuery(t *testing.T) {
	_, err := bind(t, http.MethodPatch, thing+"?foo=1", `{"name":"x"}`)
	e := requireCode(t, err, "unknown_parameter")
	require.Equal(t, map[string]any{"parameter": "foo"}, e.Params)
	require.Equal(t, http.StatusBadRequest, e.Status)
}

func TestBindRejectsUnknownJSONFieldAndMalformedJSON(t *testing.T) {
	_, err := bind(t, http.MethodPatch, thing, `{"name":"x","installation_number":"1"}`)
	requireCode(t, err, "invalid_body")
	_, err = bind(t, http.MethodPatch, thing, `{"name":`)
	requireCode(t, err, "invalid_body")
	_, err = bind(t, http.MethodPatch, thing, `{"name":"x"}{"name":"y"}`)
	requireCode(t, err, "invalid_body")
}

func TestBindValidation422(t *testing.T) {
	_, err := bind(t, http.MethodPatch, thing, `{"children":[{}]}`)
	e := requireCode(t, err, "validation_failed")
	require.Equal(t, http.StatusUnprocessableEntity, e.Status)
	require.Equal(t, map[string]any{"name": []string{"required"}, "children[0].rate": []string{"required"}}, e.Params)
}

func TestBindDecimalRejectsJSONNumber(t *testing.T) {
	_, err := bind(t, http.MethodPatch, thing, `{"name":"x","total_area_m2":12.5}`)
	requireCode(t, err, "invalid_body")
	_, err = bind(t, http.MethodPatch, thing, `{"name":"x","total_area_m2":"12,5"}`)
	requireCode(t, err, "invalid_body")
}

func TestBindBadPathAndQueryValues(t *testing.T) {
	_, err := bind(t, http.MethodPatch, "/things/not-a-uuid", `{"name":"x"}`)
	require.ErrorIs(t, err, perr.NotFound)
	_, err = bind(t, http.MethodPatch, thing+"?from=01.03.2026", `{"name":"x"}`)
	e := requireCode(t, err, "invalid_parameters")
	require.Equal(t, map[string]any{"from": []string{"invalid"}}, e.Params)
	_, err = bind(t, http.MethodPatch, thing+"?is_active=true&is_active=false", `{"name":"x"}`)
	requireCode(t, err, "invalid_parameters")
}

func TestPageCursorRoundTrip(t *testing.T) {
	limit := int32(2)
	page, size, err := kit.ResolvePage(dto.PageRequest{Limit: &limit}, 1000)
	require.NoError(t, err)
	require.Equal(t, store.Page{Limit: 3, Offset: 0}, page)

	full := kit.PageOf([]int{1, 2, 3}, page, size)
	require.Equal(t, []int{1, 2}, full.Items)
	require.NotNil(t, full.NextCursor)

	next, _, err := kit.ResolvePage(dto.PageRequest{Limit: &limit, Cursor: *full.NextCursor}, 1000)
	require.NoError(t, err)
	require.Equal(t, int32(2), next.Offset)
	last := kit.PageOf([]int{3}, next, size)
	require.Nil(t, last.NextCursor)
	require.Equal(t, []int{}, kit.PageOf[int](nil, next, size).Items, "items is never null")

	tooBig := int32(501)
	_, _, err = kit.ResolvePage(dto.PageRequest{Limit: &tooBig}, 1000)
	requireCode(t, err, "invalid_limit")
	capped := int32(300)
	_, _, err = kit.ResolvePage(dto.PageRequest{Limit: &capped}, 200)
	requireCode(t, err, "invalid_limit")
	_, _, err = kit.ResolvePage(dto.PageRequest{Cursor: "zz"}, 1000)
	requireCode(t, err, "invalid_cursor")
	def, size, err := kit.ResolvePage(dto.PageRequest{}, 1000)
	require.NoError(t, err)
	require.Equal(t, int32(50), size)
	require.Equal(t, int32(51), def.Limit)
}

func TestErrorEnvelopeLocalised(t *testing.T) {
	write := func(err error, lang string) (int, dto.Error) {
		rec := httptest.NewRecorder()
		rec.Header().Set("X-Request-Id", "req-1")
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		if lang != "" {
			req.Header.Set("Accept-Language", lang)
		}
		kit.WriteError(rec, req, err)
		var body dto.Error
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		return rec.Code, body
	}
	status, body := write(perr.NotFound, "en-US,en;q=0.9,tr;q=0.8")
	require.Equal(t, http.StatusNotFound, status)
	require.Equal(t, "Not found.", body.Error.Message)
	require.Equal(t, "req-1", body.Error.RequestID)

	_, body = write(perr.NotFound, "")
	require.Equal(t, "Kayıt bulunamadı.", body.Error.Message)
	_, body = write(perr.NotFound, "de-DE,en;q=0.5,tr;q=0.9")
	require.Equal(t, "Kayıt bulunamadı.", body.Error.Message, "higher q wins")

	status, body = write(errors.New("pq: password=hunter2 connection refused"), "en")
	require.Equal(t, http.StatusInternalServerError, status)
	require.Equal(t, "internal", body.Error.Code)
	raw, _ := json.Marshal(body)
	require.NotContains(t, string(raw), "hunter2")

	status, body = write(store.ErrNotFound, "")
	require.Equal(t, http.StatusNotFound, status)
	require.Equal(t, "not_found", body.Error.Code)
	status, _ = write(store.ErrConflict, "")
	require.Equal(t, http.StatusConflict, status)

	_, body = write(perr.Validation.WithParams(map[string]any{"name": []string{"required"}}), "")
	require.Equal(t, map[string]any{"name": []any{"required"}}, body.Error.Details)
}

var codeLiteral = regexp.MustCompile(`perr\.New\("([a-z_]+)"|errors\.New\("([a-z_]+)", http\.`)

// Every error code created in the API and service layers has both catalogue entries (R152).
func TestEveryErrorCodeHasBothLocales(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "..", "..")
	found := 0
	for _, dir := range []string{"internal/api", "internal/service", "internal/platform/errors", "internal/credentials"} {
		require.NoError(t, filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, m := range codeLiteral.FindAllStringSubmatch(string(src), -1) {
				code := m[1] + m[2]
				found++
				require.True(t, kit.HasMessage(code), "%s: code %q has no message catalogue entry", path, code)
			}
			return nil
		}))
	}
	require.Greater(t, found, 15, "the scan found too few codes to be trusted")
	for _, code := range kit.Codes() {
		require.NotEmpty(t, kit.Message(code, "tr"), code)
		require.NotEmpty(t, kit.Message(code, "en"), code)
		require.NotEqual(t, kit.Message(code, "tr"), kit.Message(code, "en"), code)
	}
}
