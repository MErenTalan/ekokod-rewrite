//go:build integration

package v1_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	v1 "github.com/MErenTalan/ekokod-rewrite/internal/api/v1"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

// foreignIDs are objects a building admin of A1 must never reach: A2's and company B's.
type foreignIDs map[string][]string

func (h *harness) foreign(t *testing.T) foreignIDs {
	t.Helper()
	ctx := t.Context()
	billA2, billB := h.bill(h.fx.BuildingA2), h.billFor(h.fx.CompanyB, h.fx.BuildingB1)
	tariffs := map[uuid.UUID]string{}
	for _, b := range []struct{ company, building uuid.UUID }{{h.fx.CompanyA, h.fx.BuildingA2}, {h.fx.CompanyB, h.fx.BuildingB1}} {
		body := validTariff(b.building)
		admin := h.as(seed.E2EAdminEmail)
		res := admin.do(http.MethodPost, "/tariffs?company_id="+b.company.String(), body)
		require.Equal(t, http.StatusCreated, res.status, string(res.body))
		var tr dto.Tariff
		res.json(t, &tr)
		tariffs[tr.ID] = tr.ID.String()
	}
	event, err := postgres.NewCalendarRepository(h.pool).CreateEvent(ctx, store.SystemScope(h.fx.CompanyB), model.CalendarEvent{
		CompanyID: h.fx.CompanyB, Title: "B", StartsAt: h.clock.Now(), EndsAt: h.clock.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	var tariffIDs []string
	for _, id := range tariffs {
		tariffIDs = append(tariffIDs, id)
	}
	// Two foreign rules: one on company B's meter, and one on A2 — the
	// building this admin is NOT responsible for, which is what R213's
	// intersection has to refuse even though the rule is in their own company.
	alarmA2 := h.alarmOn(t, h.fx.CompanyA, h.fx.AnalyzerA2)
	alarmB := h.alarmOn(t, h.fx.CompanyB, h.fx.AnalyzerB1)

	reportA2, reportB := h.reportFor(h.fx.CompanyA, h.fx.BuildingA2), h.reportFor(h.fx.CompanyB, h.fx.BuildingB1)

	other := h.client(uaSafari)
	other.login(seed.E2ECompanyAdminEmail, false)
	var sessions dto.SessionList
	other.do(http.MethodGet, "/auth/sessions", nil).json(t, &sessions)
	return foreignIDs{
		"buildings":               {h.fx.BuildingA2.String(), h.fx.BuildingB1.String()},
		"analyzers":               {h.fx.AnalyzerA2.String(), h.fx.AnalyzerB1.String()},
		"bills":                   {billA2.ID.String(), billB.ID.String()},
		"tariffs":                 tariffIDs,
		"companies":               {h.fx.CompanyB.String()},
		"users":                   {h.fx.Users[seed.E2ECompanyBAdminEmail].String(), h.fx.Users[seed.E2ECompanyAdminEmail].String()},
		"power-plants":            {uuid.NewString()},
		"calendar":                {event.ID.String()},
		"integration-definitions": {uuid.NewString()},
		"integration-credentials": {uuid.NewString()},
		"auth":                    {sessions.Items[0].ID.String()},
		"consumption":             {uuid.NewString()},
		// A job id is an asynq task id, not a UUID; an unknown one must 404 like a foreign one.
		"jobs":   {uuid.NewString()},
		"alarms": {alarmA2, alarmB},
		// F8a's families. A template, an icmal import, a solar tariff and a
		// catalogue row all live outside this building admin's reach, and an
		// unknown id must answer exactly as a foreign one does.
		"tariff-templates":         {uuid.NewString()},
		"icmal-imports":            {uuid.NewString()},
		"solar-tariffs":            {uuid.NewString()},
		"national-tariff-schedule": {uuid.NewString()},
		// F8b: a report of the other building and one of company B.
		"reports": {reportA2, reportB},
		// F9: plants are company-level, so a building admin never reaches one.
		"plants": {h.plantFor(h.fx.CompanyA, "Sweep GES A", "PS-SWEEP-A").String(), h.plantFor(h.fx.CompanyB, "Sweep GES B", "PS-SWEEP-B").String()},
		// F10a: activities and reports of A2 and of company B, and company B's own factor.
		"carbon": {h.carbonActivityFor(h.fx.CompanyA, h.fx.BuildingA2), h.carbonActivityFor(h.fx.CompanyB, h.fx.BuildingB1),
			h.carbonReportFor(h.fx.CompanyA, h.fx.BuildingA2), h.carbonReportFor(h.fx.CompanyB, h.fx.BuildingB1),
			h.carbonFactorFor(h.fx.CompanyB)},
	}
}

// reportFor stores a completed report through the repository.
func (h *harness) reportFor(company, building uuid.UUID) string {
	h.t.Helper()
	rp, err := postgres.NewReportRepository(h.pool).Upsert(h.t.Context(), store.SystemScope(company), model.Report{
		CompanyID: company, BuildingID: building, Type: model.ReportTypeMonthly, Period: "2026-08",
		PlantSelection: model.PlantSelectionAll, Payload: []byte(`{}`), Status: model.ReportStatusPending, CreatedAt: h.clock.Now(),
	})
	require.NoError(h.t, err)
	return rp.ID.String()
}

// alarmOn creates a rule on one analyzer through the repository, so the sweep
// does not depend on the HTTP surface it is testing.
func (h *harness) alarmOn(t *testing.T, company, analyzer uuid.UUID) string {
	t.Helper()
	ctx := t.Context()
	repo := postgres.NewAlarmRepository(h.pool)
	sc := store.SystemScope(company)
	hours := int32(6)
	rule, err := repo.Create(ctx, sc, model.Alarm{
		CompanyID: company, Name: "Sweep", Type: model.AlarmTypeDataCommunication,
		IsEnabled: true, CommunicationThresholdHours: &hours,
		CreatedAt: h.clock.Now(), UpdatedAt: h.clock.Now(),
	})
	require.NoError(t, err)
	require.NoError(t, repo.ReplaceAnalyzers(ctx, sc, rule.ID, []uuid.UUID{analyzer}))
	return rule.ID.String()
}

func (h *harness) billFor(company, building uuid.UUID) model.Bill {
	h.t.Helper()
	b, err := postgres.NewBillRepository(h.pool).Create(h.t.Context(), store.SystemScope(company), model.Bill{
		CompanyID: company, BuildingID: &building, Scope: model.BillScopeBuilding, PeriodKey: "2026-08",
		PeriodStart: time.Date(2026, 8, 1, 0, 0, 0, 0, ist), PeriodEnd: time.Date(2026, 9, 1, 0, 0, 0, 0, ist), DaysInPeriod: 31,
		Currency: model.CurrencyTRY, Status: model.BillStatusIssued, GenerationUsage: model.GenerationUsageNone,
		ComputedAt: h.clock.Now(), IndexStart: []byte(`{}`), IndexEnd: []byte(`{}`),
	}, nil, nil)
	require.NoError(h.t, err)
	return b
}

// validBodies holds bodies that bind and validate, so a refusal proves scoping
// rather than binding. It is a function, not a package-level literal: the
// alarm body needs a fixture id, which does not exist at init.
func validBodies(h *harness) map[string]any {
	return map[string]any{
		"analyzers.refresh": map[string]any{"mode": "hourly"},
		"reports.email":     map[string]any{"to": []string{"yonetici@firma.test"}},
		// The analyzer named is the building admin's OWN, which sharpens the
		// point: even a rule they could legitimately describe stays
		// unreachable when its id is not theirs.
		"alarms.update": map[string]any{
			"name": "Sweep", "type": "data_communication", "is_enabled": true,
			"analyzer_ids": []string{h.fx.AnalyzerA1.String()},
			"settings":     map[string]any{"communication_threshold_hours": 6},
		},
	}
}

func pathFamily(pattern string) string {
	parts := strings.Split(strings.TrimPrefix(pattern, "/"), "/")
	return parts[0]
}

// TestBuildingAdminCannotReachOtherBuildingsAnyEndpoint is 09 §F6's tenancy acceptance.
func TestBuildingAdminCannotReachOtherBuildingsAnyEndpoint(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ids := h.foreign(t)
	bodies := validBodies(h)
	ba := h.as(seed.E2EBuildingAdminEmail)
	var auditBefore int
	require.NoError(t, h.pool.QueryRow(t.Context(), `select count(*) from audit_log`).Scan(&auditBefore))
	checked := 0
	for _, rt := range v1.Table() {
		if rt.Access == v1.Public || !strings.Contains(rt.Pattern, "{id}") {
			continue
		}
		family := pathFamily(rt.Pattern)
		candidates, ok := ids[family]
		require.True(t, ok, "%s %s has no foreign id mapping: add one to foreign()", rt.Method, rt.Pattern)
		for _, id := range candidates {
			path := strings.Replace(rt.Pattern, "{id}", id, 1)
			var body any
			if rt.Mutating() {
				body = bodies[rt.OperationID]
				if body == nil {
					body = map[string]any{}
				}
			}
			res := ba.do(rt.Method, path, body)
			require.Contains(t, []int{http.StatusForbidden, http.StatusNotFound}, res.status,
				"%s %s reached a foreign object: %d %s", rt.Method, path, res.status, string(res.body))
			checked++
		}
	}
	require.Greater(t, checked, 40)

	for _, body := range []map[string]any{
		{"scope": "building", "building_ids": []string{h.fx.BuildingA2.String()}, "period": "2026-08"},
		{"scope": "building", "building_ids": []string{h.fx.BuildingB1.String()}, "period": "2026-08"},
		{"scope": "analyzer", "analyzer_ids": []string{h.fx.AnalyzerA2.String()}, "period": "2026-08"},
		{"scope": "company", "period": "2026-08"},
	} {
		res := ba.do(http.MethodPost, "/bills/compute", body)
		require.Contains(t, []int{http.StatusForbidden, http.StatusNotFound}, res.status, "%v: %s", body, string(res.body))
	}
	for _, building := range []uuid.UUID{h.fx.BuildingA2, h.fx.BuildingB1} {
		body := map[string]any{"type": "monthly", "period": "2026-08", "plant_selection": "all", "building_ids": []string{building.String()}}
		res := ba.do(http.MethodPost, "/reports/generate", body)
		require.Equal(t, http.StatusNotFound, res.status, "generate for %s: %s", building, string(res.body))
		res = ba.do(http.MethodGet, "/reports/preview?type=monthly&period=2026-08&plant_selection=all&building_ids="+building.String(), nil)
		require.Equal(t, http.StatusNotFound, res.status, "preview for %s: %s", building, string(res.body))
	}
	require.Empty(t, h.enq.snapshot(), "a refused foreign request enqueues nothing")
	var auditAfter int
	require.NoError(t, h.pool.QueryRow(t.Context(), `select count(*) from audit_log`).Scan(&auditAfter))
	require.Equal(t, auditBefore, auditAfter, "a refused foreign mutation writes no audit row")

	a2, b1 := h.fx.AnalyzerA2.String(), h.fx.AnalyzerB1.String()
	for _, path := range []string{
		"/consumption?analyzer_id=" + a2 + "&granularity=daily&from=2026-09-01&to=2026-09-02",
		"/consumption?building_id=" + h.fx.BuildingA2.String() + "&granularity=daily&from=2026-09-01&to=2026-09-02",
		"/consumption/summary?analyzer_id=" + b1 + "&granularity=daily&from=2026-09-01&to=2026-09-02",
		"/consumption/export?format=csv&analyzer_id=" + a2 + "&granularity=daily&from=2026-09-01&to=2026-09-02",
		"/consumption/anomalies?building_id=" + h.fx.BuildingA2.String(),
		"/consumption/reactive-status?building_id=" + h.fx.BuildingA2.String(),
		"/load-profile?analyzer_id=" + a2 + "&from=2026-09-01&to=2026-09-02",
		"/load-profile/statistics?analyzer_id=" + a2 + "&from=2026-09-01&to=2026-09-02",
		"/load-profile/export?analyzer_id=" + a2 + "&from=2026-09-01&to=2026-09-02",
		"/generation?analyzer_id=" + a2 + "&granularity=daily&from=2026-09-01&to=2026-09-02",
		"/energy-balance?analyzer_id=" + b1 + "&granularity=daily&from=2026-09-01&to=2026-09-02",
		"/tariffs/applicable?building_id=" + h.fx.BuildingA2.String() + "&date=2026-09-01",
		"/renewable/overview?analyzer_id=" + a2 + "&from=2026-09-01&to=2026-09-02",
		"/renewable/environmental?building_id=" + h.fx.BuildingA2.String() + "&from=2026-09-01&to=2026-09-02",
		"/renewable/analytics?analyzer_id=" + b1 + "&from=2026-09-01&to=2026-09-02",
		"/bills/latest?scope=building&subject_id=" + h.fx.BuildingA2.String(),
		"/bills/latest?scope=company&subject_id=" + h.fx.CompanyA.String(),
	} {
		res := ba.do(http.MethodGet, path, nil)
		require.Equal(t, http.StatusNotFound, res.status, "%s: %s", path, string(res.body))
	}
	for _, path := range []string{"/tariffs?building_id=" + h.fx.BuildingA2.String(), "/bills?building_id=" + h.fx.BuildingA2.String(),
		"/reports", "/reports?building_id=" + h.fx.BuildingA2.String(),
		"/analyzers?building_id=" + h.fx.BuildingA2.String(), "/buildings?company_id=" + h.fx.CompanyA.String()} {
		res := ba.do(http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, res.status, path)
		for _, list := range ids {
			for _, id := range list {
				require.NotContains(t, string(res.body), id, "%s listed a foreign object", path)
			}
		}
	}
}

// TestReadOnlyRolesForbiddenOnEveryMutationHTTP re-runs the matrix against the real stack.
func TestReadOnlyRolesForbiddenOnEveryMutationHTTP(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	emails := []string{seed.E2ECompanyReadonlyEmail, seed.E2EBuildingReadonlyEmail, h.demoUser()}
	clients := map[string]*client{}
	for _, email := range emails {
		clients[email] = h.as(email)
	}
	audited := func() (n int) {
		require.NoError(t, h.pool.QueryRow(t.Context(), `select count(*) from audit_log`).Scan(&n))
		return n
	}
	before := audited()
	checked := 0
	for _, email := range emails {
		c := clients[email]
		for _, rt := range v1.Table() {
			if !rt.Mutating() || rt.Access == v1.Public || rt.SelfService {
				continue
			}
			path := strings.ReplaceAll(rt.Pattern, "{id}", uuid.NewString())
			res := c.do(rt.Method, path, map[string]any{})
			require.Equal(t, http.StatusForbidden, res.status, "%s %s as %s: %s", rt.Method, path, email, string(res.body))
			checked++
		}
	}
	require.GreaterOrEqual(t, checked, 3*35)
	require.Equal(t, before, audited(), "a refused mutation writes no audit row")
}

func TestEveryMutationAudited(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	admin := h.as(seed.E2EAdminEmail)
	steps := []struct {
		c      *client
		method string
		path   string
		body   any
		action string
	}{
		{ca, http.MethodPost, "/buildings", map[string]any{"name": "Denetim"}, "buildings.create"},
		{ca, http.MethodPatch, "/companies/" + h.fx.CompanyA.String(), map[string]any{"contact_name": "Ayşe"}, "companies.update"},
		{ca, http.MethodPost, "/users", map[string]any{"name": "Selin Koç", "email": "selin@a.e2e.ekokod.test", "role": "building_admin", "password": "Guclu!Parola-91"}, "users.create"},
		{ca, http.MethodPatch, "/analyzers/" + h.fx.AnalyzerA1.String(), map[string]any{"installed_power_kw": "99"}, "analyzers.update"},
		{ca, http.MethodPost, "/power-plants", map[string]any{"name": "GES"}, "plants.create"},
		{ca, http.MethodPost, "/tariffs", validTariff(h.fx.BuildingA1), "tariffs.create"},
		{ca, http.MethodPost, "/calendar/events", map[string]any{"title": "x", "starts_at": "2026-09-10T09:00:00+03:00", "ends_at": "2026-09-10T10:00:00+03:00"}, "calendar.events.create"},
		{ca, http.MethodPut, "/calendar/vacations", map[string]any{"weekend_days": []int{0, 6}}, "calendar.vacations.put"},
		{admin, http.MethodPut, "/smtp-settings?company_id=" + h.fx.CompanyA.String(), map[string]any{"host": "smtp.example.com", "port": 587, "from_address": "a@example.com", "password": "x"}, "smtp.upsert"},
		{admin, http.MethodPost, "/integration-definitions", definition("aril", "Default", map[string]string{"authentication": "https://aril.example"}), "integration_definitions.create"},
	}
	for _, s := range steps {
		res := s.c.do(s.method, s.path, s.body)
		require.Less(t, res.status, 300, "%s %s: %s", s.method, s.path, string(res.body))
		var n int
		require.NoError(t, h.pool.QueryRow(t.Context(), `select count(*) from audit_log where action = $1`, s.action).Scan(&n))
		require.Equal(t, 1, n, s.action)
	}
	var smtpCompany *uuid.UUID
	require.NoError(t, h.pool.QueryRow(t.Context(), `select company_id from audit_log where action = 'smtp.upsert'`).Scan(&smtpCompany))
	require.Equal(t, h.fx.CompanyA, *smtpCompany, "an admin acting on company A is audited under company A")
}
