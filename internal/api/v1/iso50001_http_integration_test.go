//go:build integration

package v1_test

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

func (h *harness) isoNoteFor(company, building uuid.UUID) string {
	h.t.Helper()
	repo := postgres.NewISO50001Repository(h.pool)
	sc := store.SystemScope(company)
	p, err := repo.EnsureProject(h.t.Context(), sc, building)
	require.NoError(h.t, err)
	n, err := repo.CreateNote(h.t.Context(), sc, model.ISO50001Note{ProjectID: p.ID, ClauseID: "5.1", Body: "Sweep"})
	require.NoError(h.t, err)
	return n.ID.String()
}

func (h *harness) isoFileFor(company, building uuid.UUID) string {
	h.t.Helper()
	b, clause := building, "5.1"
	f, err := postgres.NewFileRepository(h.pool).Create(h.t.Context(), store.SystemScope(company), model.StoredFile{
		CompanyID: company, OwnerType: "iso50001", OwnerID: &b, ClauseID: &clause, OriginalName: "sweep.pdf",
		StoredPath: uuid.NewString(), ContentType: "application/pdf", SizeBytes: 1, Checksum: "x", CreatedAt: h.clock.Now()})
	require.NoError(h.t, err)
	return f.ID.String()
}

// TestFileAuthorization is 09 §F11's acceptance: evidence is reachable only
// through the authorising handler, never by path, and only inside the scope.
func TestFileAuthorization(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ba := h.as(seed.E2EBuildingAdminEmail)
	own := h.fx.BuildingA1.String()
	pdf := []byte("%PDF-1.7\n%%EOF\n")

	res := ba.upload(t, "/iso50001/"+own+"/clauses/5.1/files", "Politika.pdf", pdf)
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	var f dto.ISOFile
	res.json(t, &f)

	got := ba.do(http.MethodGet, "/iso50001/files/"+f.ID.String(), nil)
	require.Equal(t, http.StatusOK, got.status)
	require.Equal(t, pdf, got.body)

	// The stored path is never a URL.
	var stored string
	require.NoError(t, h.pool.QueryRow(t.Context(), `select stored_path from stored_files where id = $1`, f.ID).Scan(&stored))
	for _, path := range []string{"/" + stored, "/api/v1/" + stored, "/storage/iso50001/" + stored} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, h.srv.URL+path, nil)
		require.NoError(t, err)
		direct, err := h.srv.Client().Do(req)
		require.NoError(t, err)
		_ = direct.Body.Close()
		require.NotEqual(t, http.StatusOK, direct.StatusCode, path)
	}

	// Another building's file and another company's are 404.
	for _, id := range []string{h.isoFileFor(h.fx.CompanyA, h.fx.BuildingA2), h.isoFileFor(h.fx.CompanyB, h.fx.BuildingB1)} {
		require.Equal(t, http.StatusNotFound, ba.do(http.MethodGet, "/iso50001/files/"+id, nil).status)
	}
	// Deleted is gone.
	require.Equal(t, http.StatusNoContent, ba.do(http.MethodDelete, "/iso50001/files/"+f.ID.String(), nil).status)
	require.Equal(t, http.StatusNotFound, ba.do(http.MethodGet, "/iso50001/files/"+f.ID.String(), nil).status)

	// A renamed executable and an oversize file are refused clearly.
	res = ba.upload(t, "/iso50001/"+own+"/clauses/5.1/files", "fatura.pdf", append([]byte("MZ"), bytes.Repeat([]byte{0x90}, 64)...))
	require.Equal(t, http.StatusUnprocessableEntity, res.status)
	require.Contains(t, string(res.body), "type_not_allowed")
	res = ba.upload(t, "/iso50001/"+own+"/clauses/5.1/files", "buyuk.pdf", append(pdf, bytes.Repeat([]byte("x"), 31<<20)...))
	require.Equal(t, http.StatusRequestEntityTooLarge, res.status, string(res.body))
	require.True(t, strings.Contains(string(res.body), "30 MB"))
}

func TestISOProjectFlow(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	b := h.fx.BuildingA1.String()
	res := ca.do(http.MethodPost, "/iso50001/"+b+"/clauses/5.2/notes", map[string]any{"title": "Politika", "body": "Yayımlandı."})
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	clauses := []map[string]any{}
	for _, id := range []string{"5", "6", "7", "8", "9"} {
		clauses = append(clauses, map[string]any{"clause_id": id, "start": "2026-01-01", "end": "2026-12-31"})
	}
	res = ca.do(http.MethodPut, "/iso50001/"+b+"/dates", map[string]any{"clauses": clauses})
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	var p dto.ISOProject
	res.json(t, &p)
	require.True(t, p.GanttAvailable)
	require.Equal(t, 5, p.Progress)

	clauses[0]["start"] = "2027-01-01"
	res = ca.do(http.MethodPut, "/iso50001/"+b+"/dates", map[string]any{"clauses": clauses})
	require.Equal(t, http.StatusUnprocessableEntity, res.status)

	exp := ca.do(http.MethodGet, "/iso50001/"+b+"/export", nil)
	require.Equal(t, http.StatusOK, exp.status)
	require.Equal(t, "PK", string(exp.body[:2]))
	require.Equal(t, http.StatusNotFound, h.as(seed.E2EBuildingAdminEmail).do(http.MethodGet, "/iso50001/"+h.fx.BuildingA2.String()+"/export", nil).status)
}
