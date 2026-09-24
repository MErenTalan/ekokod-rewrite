//go:build integration

package v1_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

// seedMessages writes one row per kind, with distinct statuses and text, so
// every filter in §7.13 has something to include and something to exclude.
func (h *harness) seedMessages(t *testing.T, company uuid.UUID) {
	t.Helper()
	repo := postgres.NewOpsRepository(h.pool)
	sc := store.SystemScope(company)
	detail := "Endüktif oran eşiği aşıldı"
	for _, m := range []model.OperationalMessage{
		{CompanyID: &company, Kind: "alarm", Category: "alarm-trigger", Status: "warning",
			Message: "Alarm tetiklendi", Detail: &detail},
		{CompanyID: &company, Kind: "job", Category: "analyzer-refresh", Status: "success",
			Message: "Analizör yenilendi"},
		{CompanyID: &company, Kind: "system", Category: "auth", Status: "info",
			Message: "Oturum açıldı"},
	} {
		_, err := repo.AppendMessage(t.Context(), sc, m)
		require.NoError(t, err)
	}
}

// 09 §F7 acceptance: Messages filters return the expected sets on a seeded fixture.
func TestMessagesFiltersReturnTheExpectedSets(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seedMessages(t, h.fx.CompanyA)
	ca := h.as(seed.E2ECompanyAdminEmail)

	for _, tc := range []struct {
		query string
		want  int
	}{
		{"", 3},
		{"?kind=alarm", 1},
		{"?kind=job", 1},
		{"?status=success", 1},
		{"?status=error", 0},
		{"?q=tetiklendi", 1},
		{"?q=Endüktif", 1},   // the detail column is searched too (R226)
		{"?q=TETIKLENDI", 1}, // case-insensitively
		{"?kind=job&status=success", 1},
		{"?kind=job&status=error", 0},
		{"?q=bulunmayan", 0},
	} {
		res := ca.do(http.MethodGet, "/messages"+tc.query, nil)
		require.Equal(t, http.StatusOK, res.status, tc.query+" "+string(res.body))
		var page dto.Page[dto.Message]
		res.json(t, &page)
		require.Len(t, page.Items, tc.want, tc.query)
	}
}

func TestMessagesNeverShowAnotherTenantsRows(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seedMessages(t, h.fx.CompanyB)
	ca := h.as(seed.E2ECompanyAdminEmail)

	res := ca.do(http.MethodGet, "/messages", nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	var page dto.Page[dto.Message]
	res.json(t, &page)
	require.Empty(t, page.Items, "company A must not see company B's messages")
}

func TestMessagesAreReadableByEveryRole(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seedMessages(t, h.fx.CompanyA)
	for _, email := range []string{
		seed.E2ECompanyAdminEmail, seed.E2ECompanyReadonlyEmail,
		seed.E2EBuildingAdminEmail, seed.E2EBuildingReadonlyEmail,
	} {
		res := h.as(email).do(http.MethodGet, "/messages", nil)
		require.Equal(t, http.StatusOK, res.status, email)
	}
}

func TestJobRunsAreAdminAndCompanyAdminOnly(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	require.Equal(t, http.StatusOK, h.as(seed.E2EAdminEmail).do(http.MethodGet, "/job-runs", nil).status)
	require.Equal(t, http.StatusOK, h.as(seed.E2ECompanyAdminEmail).do(http.MethodGet, "/job-runs", nil).status)
	for _, email := range []string{
		seed.E2ECompanyReadonlyEmail, seed.E2EBuildingAdminEmail, seed.E2EBuildingReadonlyEmail,
	} {
		require.Equal(t, http.StatusForbidden, h.as(email).do(http.MethodGet, "/job-runs", nil).status, email)
	}
}

// 09 §F7 acceptance: a manually triggered job appears in job_runs with its
// scope and result. The enqueue is asserted here; the run row is written by
// the worker, which internal/service/alarms' own tests cover.
func TestTriggerEnqueuesTheJobItNames(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	admin := h.as(seed.E2EAdminEmail)

	res := admin.do(http.MethodPost, "/job-runs/alarm.evaluate/trigger?company_id="+h.fx.CompanyA.String(), nil)
	require.Equal(t, http.StatusAccepted, res.status, string(res.body))
	var acc dto.JobAccepted
	res.json(t, &acc)
	require.NotEmpty(t, acc.JobID)

	var types []string
	for _, task := range h.enq.snapshot() {
		types = append(types, task.Type())
	}
	require.Contains(t, types, job.TypeAlarmEvaluate)
}

func TestTriggerIsAdminOnlyAndRefusesUnknownTypes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	require.Equal(t, http.StatusForbidden,
		h.as(seed.E2ECompanyAdminEmail).do(http.MethodPost, "/job-runs/alarm.evaluate/trigger", nil).status)

	admin := h.as(seed.E2EAdminEmail)
	// R220: anything off the allow-list is refused by the binder's own enum,
	// so no arbitrary task name can reach the queue.
	for _, bad := range []string{"system.noop", "alarm.notify", "billing.generate"} {
		res := admin.do(http.MethodPost, "/job-runs/"+bad+"/trigger", nil)
		require.Equal(t, http.StatusUnprocessableEntity, res.status, bad+" "+string(res.body))
	}
	require.Empty(t, h.enq.snapshot(), "a refused trigger enqueues nothing")
}

func TestJobRunsListsWhatTheTenantOwns(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	repo := postgres.NewOpsRepository(h.pool)
	company := h.fx.CompanyA
	started, err := repo.StartRun(t.Context(), store.SystemScope(company), model.JobRun{
		CompanyID: &company, JobType: job.TypeAlarmEvaluate, Scope: []byte(`{"company_id":"` + company.String() + `"}`),
	})
	require.NoError(t, err)
	_, err = repo.FinishRun(t.Context(), store.SystemScope(company), started.ID, "partial", 3, 1, 2, nil, nil, h.clock.Now())
	require.NoError(t, err)

	res := h.as(seed.E2ECompanyAdminEmail).do(http.MethodGet, "/job-runs?job_type=alarm.evaluate", nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	var page dto.Page[dto.JobRun]
	res.json(t, &page)
	require.Len(t, page.Items, 1)
	require.Equal(t, "partial", page.Items[0].Status)
	require.EqualValues(t, 3, page.Items[0].Processed)
	require.EqualValues(t, 1, page.Items[0].Skipped)
	require.EqualValues(t, 2, page.Items[0].Failed)
	require.NotNil(t, page.Items[0].FinishedAt)
	require.Contains(t, string(page.Items[0].Scope), company.String())
}
