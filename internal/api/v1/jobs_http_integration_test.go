//go:build integration

package v1_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
)

// enqueueRefresh puts a real refresh task in the shared queue the API's
// inspector reads, and returns its task id.
func (h *harness) enqueueRefresh(t *testing.T, company, analyzer uuid.UUID) string {
	t.Helper()
	client, err := job.NewClient(h.cfg.Redis)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	task, err := job.NewRefreshAnalyzerTask(job.RefreshAnalyzerPayload{
		CompanyID: company, CredentialID: uuid.New(), AnalyzerID: analyzer, Mode: job.RefreshModeHourly,
	}, job.TaskOptions{MaxRetry: 1})
	require.NoError(t, err)
	info, err := client.Enqueue(t.Context(), task)
	require.NoError(t, err)
	return info.ID
}

// R192: a screen polls the job it started; nobody else's job is visible.
func TestJobStatusHTTP(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	mine := h.enqueueRefresh(t, h.fx.CompanyA, h.fx.AnalyzerA1)
	foreignCompany := h.enqueueRefresh(t, h.fx.CompanyB, h.fx.AnalyzerB1)
	otherBuilding := h.enqueueRefresh(t, h.fx.CompanyA, h.fx.AnalyzerA2)

	ca := h.as(seed.E2ECompanyAdminEmail)
	res := ca.do(http.MethodGet, "/jobs/"+mine, nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	var view dto.Job
	res.json(t, &view)
	require.Equal(t, mine, view.ID)
	require.Equal(t, job.TypeIntegrationRefreshAnalyzer, view.Type)
	require.Equal(t, "queued", view.Status)
	require.Nil(t, view.CompletedAt)
	require.NotContains(t, strings.ToLower(string(res.body)), "last_err", "the worker's own error text never leaves the API")

	require.Equal(t, http.StatusNotFound, ca.do(http.MethodGet, "/jobs/"+foreignCompany, nil).status)
	require.Equal(t, http.StatusNotFound, ca.do(http.MethodGet, "/jobs/"+uuid.NewString(), nil).status)

	ba := h.as(seed.E2EBuildingAdminEmail)
	require.Equal(t, http.StatusOK, ba.do(http.MethodGet, "/jobs/"+mine, nil).status, "A1 is the building-admin's building")
	require.Equal(t, http.StatusNotFound, ba.do(http.MethodGet, "/jobs/"+otherBuilding, nil).status,
		"a refresh of another building's analyzer is invisible")

	cr := h.as(seed.E2ECompanyReadonlyEmail)
	require.Equal(t, http.StatusForbidden, cr.do(http.MethodGet, "/jobs/"+mine, nil).status)
}
