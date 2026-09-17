//go:build integration

package v1_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func TestCalendarReadOnlyForBuildingRolesAndCRUD(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	event := map[string]any{"title": "Bakım", "starts_at": "2026-09-10T09:00:00+03:00", "ends_at": "2026-09-10T12:00:00+03:00", "colour": "#10b981"}
	res := ca.do(http.MethodPost, "/calendar/events", event)
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	var created dto.CalendarEvent
	res.json(t, &created)

	ba := h.as(seed.E2EBuildingAdminEmail)
	var events dto.CalendarEvents
	res = ba.do(http.MethodGet, "/calendar/events?from=2026-09-01&to=2026-09-30", nil)
	require.Equal(t, http.StatusOK, res.status)
	res.json(t, &events)
	require.Len(t, events.Items, 1)
	require.Equal(t, http.StatusForbidden, ba.do(http.MethodPost, "/calendar/events", event).status)

	event["ends_at"] = "2026-09-10T08:00:00+03:00"
	require.Equal(t, http.StatusUnprocessableEntity, ca.do(http.MethodPatch, "/calendar/events/"+created.ID.String(), event).status)
	event["colour"] = "green"
	require.Equal(t, http.StatusUnprocessableEntity, ca.do(http.MethodPost, "/calendar/events", event).status)
	require.Equal(t, http.StatusNotFound, h.as(seed.E2ECompanyBAdminEmail).do(http.MethodDelete, "/calendar/events/"+created.ID.String(), nil).status)
	require.Equal(t, http.StatusNoContent, ca.do(http.MethodDelete, "/calendar/events/"+created.ID.String(), nil).status)

	var vac dto.Vacations
	ca.do(http.MethodGet, "/calendar/vacations", nil).json(t, &vac)
	require.Equal(t, []int{0, 6}, vac.WeekendDays)
	require.Equal(t, "default", vac.WeekendSource)
	res = ca.do(http.MethodPut, "/calendar/vacations", map[string]any{"weekend_days": []int{5, 6},
		"periods": []map[string]any{{"start_date": "2026-10-28", "end_date": "2026-10-29", "description": "Cumhuriyet Bayramı"}}})
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	res.json(t, &vac)
	require.Equal(t, []int{5, 6}, vac.WeekendDays)
	require.Equal(t, "company", vac.WeekendSource)
	require.Len(t, vac.Periods, 1)
	require.Equal(t, http.StatusUnprocessableEntity, ca.do(http.MethodPut, "/calendar/vacations", map[string]any{"weekend_days": []int{7}}).status)
	require.Equal(t, http.StatusUnprocessableEntity, ca.do(http.MethodPut, "/calendar/vacations", map[string]any{"weekend_days": []int{6},
		"periods": []map[string]any{{"start_date": "2026-10-29", "end_date": "2026-10-28"}}}).status)
}

// TestWeekendAndVacationChangeLoadProfileHTTP is 09 §F6's calendar acceptance under R137.
func TestWeekendAndVacationChangeLoadProfileHTTP(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.clock.Set(time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC))
	rate := func(ts time.Time) decimal.Decimal {
		switch ts.In(ist).Weekday() {
		case time.Saturday, time.Sunday:
			return decimal.NewFromInt(4)
		case time.Wednesday:
			return decimal.NewFromInt(20)
		}
		return decimal.NewFromInt(10)
	}
	testfixtures.InsertReadings(t, t.Context(), h.pool, h.fx.CompanyA, testfixtures.HourlyReadings(h.fx.AnalyzerA1,
		time.Date(2026, 8, 30, 0, 0, 0, 0, ist), time.Date(2026, 9, 15, 0, 0, 0, 0, ist), rate))
	ca := h.as(seed.E2ECompanyAdminEmail)
	hour10 := func() (weekday, weekend string) {
		t.Helper()
		var p dto.LoadProfiles
		res := ca.do(http.MethodGet, "/load-profile?analyzer_id="+h.fx.AnalyzerA1.String()+"&from=2026-08-31&to=2026-09-13&profiles=weekday,weekend", nil)
		require.Equal(t, http.StatusOK, res.status, string(res.body))
		res.json(t, &p)
		require.Len(t, p.Profiles["weekday"], 24, string(res.body))
		require.NotNil(t, p.Profiles["weekday"][10], string(res.body))
		require.NotNil(t, p.Profiles["weekend"][10], string(res.body))
		return p.Profiles["weekday"][10].Round(6).String(), p.Profiles["weekend"][10].Round(6).String()
	}
	wd, we := hour10()
	require.Equal(t, "12", wd, "ten weekdays, two Wednesdays at 20")
	require.Equal(t, "4", we)

	require.Equal(t, http.StatusCreated, ca.do(http.MethodPost, "/calendar/events", map[string]any{"title": "Denetim", "all_day": true,
		"starts_at": "2026-09-10T00:00:00+03:00", "ends_at": "2026-09-11T00:00:00+03:00"}).status)
	wd, we = hour10()
	require.Equal(t, []string{"12", "4"}, []string{wd, we}, "a calendar event never changes the split (R137)")

	require.Equal(t, http.StatusOK, ca.do(http.MethodPut, "/calendar/vacations", map[string]any{"weekend_days": []int{0, 6},
		"periods": []map[string]any{{"start_date": "2026-09-09", "end_date": "2026-09-09", "description": "Tatil"}}}).status)
	wd, we = hour10()
	require.Equal(t, "11.111111", wd, "a vacation Wednesday leaves nine weekdays: 100 / 9")
	require.Equal(t, "7.2", we, "and joins the weekend: (4 × 4 + 20) / 5")

	require.Equal(t, http.StatusOK, ca.do(http.MethodPut, "/calendar/vacations", map[string]any{"weekend_days": []int{0}, "periods": []any{}}).status)
	wd, we = hour10()
	require.Equal(t, "10.666667", wd, "Saturdays become working days: (8 × 10 + 2 × 20 + 2 × 4) / 12")
	require.Equal(t, "4", we)
}

func definition(provider, subtype string, endpoints map[string]string) map[string]any {
	return map[string]any{"provider": provider, "subtype": subtype, "endpoints": endpoints}
}

func TestIntegrationDefinitionsAdmin(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	admin := h.as(seed.E2EAdminEmail)
	res := admin.do(http.MethodPost, "/integration-definitions", definition("osos", "Baskent", map[string]string{"authentication": "http://osos.example/login"}))
	require.Equal(t, http.StatusUnprocessableEntity, res.status)
	res = admin.do(http.MethodPost, "/integration-definitions", definition("osos", "Baskent", map[string]string{"Bad-Key": "https://x"}))
	require.Equal(t, http.StatusUnprocessableEntity, res.status)
	res = admin.do(http.MethodPost, "/integration-definitions", definition("osos", "Baskent", map[string]string{"authentication": "https://osos.example/login", "load_profiles": "/api/lp"}))
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	var def dto.IntegrationDefinition
	res.json(t, &def)
	require.Equal(t, http.StatusConflict, admin.do(http.MethodPost, "/integration-definitions", definition("osos", "Baskent", map[string]string{"a": "b"})).status)

	ca := h.as(seed.E2ECompanyAdminEmail)
	require.Equal(t, http.StatusForbidden, ca.do(http.MethodGet, "/integration-definitions", nil).status)
	res = ca.do(http.MethodPost, "/integration-credentials", map[string]any{"provider": "osos", "subtype": "Baskent", "username": "osos-user", "secret": "pw"})
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	res = admin.do(http.MethodDelete, "/integration-definitions/"+def.ID.String(), nil)
	require.Equal(t, http.StatusConflict, res.status)
	require.Equal(t, "definition_in_use", res.code(t))
	var audited int
	require.NoError(t, h.pool.QueryRow(t.Context(), `select count(*) from audit_log where company_id is null and action = 'integration_definitions.create'`).Scan(&audited))
	require.Equal(t, 1, audited)
}

func (h *harness) sentinelCredentials(t *testing.T) map[string]dto.IntegrationCredential {
	t.Helper()
	admin := h.as(seed.E2EAdminEmail)
	for _, d := range []map[string]any{
		definition("osos", "Baskent", map[string]string{"authentication": "https://osos.example/login"}),
		definition("gridbox", "Default", map[string]string{"token": "https://gridbox.example/token"}),
		definition("aril", "Default", map[string]string{"authentication": "https://aril.example/login"}),
		definition("pm5340", "Default", map[string]string{"hourly_values": "/hourly"}),
		definition("isolar", "EU", map[string]string{"gateway": "https://gateway.isolar.example", "authorize_origin": "https://web3.isolar.example", "cloud_id": "3"}),
	} {
		res := admin.do(http.MethodPost, "/integration-definitions", d)
		require.Equal(t, http.StatusCreated, res.status, string(res.body))
	}
	ca := h.as(seed.E2ECompanyAdminEmail)
	out := map[string]dto.IntegrationCredential{}
	for _, p := range []struct{ provider, subtype string }{{"osos", "Baskent"}, {"gridbox", "Default"}, {"aril", "Default"}, {"pm5340", "Default"}, {"isolar", "EU"}} {
		body := map[string]any{"provider": p.provider, "subtype": p.subtype, "username": p.provider + "-user",
			"secret": "SENTINEL-" + p.provider + "-SECRET", "extra": map[string]string{"app_key": "SENTINEL-" + p.provider + "-EXTRA", "app_id": "app-" + p.provider}}
		if p.provider == "pm5340" {
			body["pm5340_url"] = "https://pm5340.example/api"
		}
		res := ca.do(http.MethodPost, "/integration-credentials", body)
		require.Equal(t, http.StatusCreated, res.status, string(res.body))
		var cred dto.IntegrationCredential
		res.json(t, &cred)
		out[p.provider] = cred
	}
	return out
}

func sentinels() []string {
	var out []string
	for _, p := range []string{"osos", "gridbox", "aril", "pm5340", "isolar"} {
		for _, kind := range []string{"SECRET", "EXTRA"} {
			s := "SENTINEL-" + p + "-" + kind
			out = append(out, s, base64.StdEncoding.EncodeToString([]byte(s)), hex.EncodeToString([]byte(s)))
		}
	}
	return out
}

// TestCredentialsNeverLeak is 09 §F6's secrecy acceptance (R177).
func TestCredentialsNeverLeak(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	creds := h.sentinelCredentials(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	admin := h.as(seed.E2EAdminEmail)
	var bodies []string
	record := func(res response) response {
		bodies = append(bodies, string(res.body), strings.Join(res.header.Values("Set-Cookie"), ";"))
		return res
	}
	osos := creds["osos"].ID.String()
	record(ca.do(http.MethodGet, "/integration-credentials", nil))
	record(ca.do(http.MethodPatch, "/integration-credentials/"+osos, map[string]any{"username": "osos-user-2"}))
	record(ca.do(http.MethodPost, "/integration-credentials/"+osos+"/verify", nil))
	record(ca.do(http.MethodPost, "/integration-credentials/"+osos+"/discover", nil))
	record(ca.do(http.MethodPost, "/integration-credentials/"+osos+"/backfill", map[string]any{"from": "2026-08-01T00:00:00+03:00", "to": "2026-09-01T00:00:00+03:00"}))
	record(ca.do(http.MethodGet, "/integrations/isolar/authorize-url?credential_id="+creds["isolar"].ID.String(), nil))
	record(admin.do(http.MethodGet, "/integration-credentials?company_id="+h.fx.CompanyA.String(), nil))
	record(admin.do(http.MethodGet, "/integration-definitions", nil))
	for _, path := range []string{"/auth/me", "/profile", "/companies/" + h.fx.CompanyA.String(), "/users", "/buildings?include=analyzer_count,active_status",
		"/analyzers", "/power-plants", "/tariffs", "/bills", "/calendar/vacations", "/consumption/reactive-status", "/openapi.json"} {
		record(ca.do(http.MethodGet, path, nil))
	}
	require.Greater(t, len(bodies), 30)
	for _, body := range bodies {
		for _, s := range sentinels() {
			require.NotContains(t, body, s, "a credential secret leaked")
		}
	}
	var list dto.IntegrationCredentials
	ca.do(http.MethodGet, "/integration-credentials", nil).json(t, &list)
	require.Len(t, list.Items, 5)
	for _, c := range list.Items {
		require.True(t, c.HasSecret)
		require.Equal(t, []string{"app_id", "app_key"}, c.ExtraKeys, "names only, never values")
	}
	require.Equal(t, http.StatusNotFound, h.as(seed.E2ECompanyBAdminEmail).do(http.MethodPost, "/integration-credentials/"+osos+"/verify", nil).status)
}

func TestCredentialErrorMapping(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	creds := h.sentinelCredentials(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	osos := "/integration-credentials/" + creds["osos"].ID.String()
	require.Equal(t, http.StatusOK, ca.do(http.MethodPatch, osos, map[string]any{"username": "config-broken"}).status)
	res := ca.do(http.MethodPost, osos+"/verify", nil)
	require.Equal(t, http.StatusUnprocessableEntity, res.status)
	require.Equal(t, "integration_config_invalid", res.code(t))
	require.Equal(t, http.StatusOK, ca.do(http.MethodPatch, osos, map[string]any{"username": "auth-broken"}).status)
	res = ca.do(http.MethodPost, osos+"/verify", nil)
	require.Equal(t, "integration_auth_failed", res.code(t))
	res = ca.do(http.MethodPost, "/integration-credentials", map[string]any{"provider": "osos", "subtype": "Baskent", "secret": "x"})
	require.Equal(t, http.StatusConflict, res.status)
	require.Equal(t, "integration_already_configured", res.code(t))
	res = ca.do(http.MethodGet, "/integrations/isolar/authorize-url?credential_id="+creds["osos"].ID.String(), nil)
	require.Equal(t, http.StatusUnprocessableEntity, res.status)
}

func TestIsolarCallbackRequiresBrowserBinding(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	creds := h.sentinelCredentials(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	res := ca.do(http.MethodGet, "/integrations/isolar/authorize-url?credential_id="+creds["isolar"].ID.String(), nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	cookie := setCookie(t, res, "ekokod_oauth")
	for _, attr := range []string{"Path=/api/v1/integrations/isolar/callback", "HttpOnly", "Secure", "SameSite=Lax", "Max-Age=600"} {
		require.Contains(t, cookie, attr)
	}
	var auth dto.AuthorizeURL
	res.json(t, &auth)
	_, redirectEncoded, _ := strings.Cut(auth.URL, "redirectUrl=")
	redirect, err := url.QueryUnescape(redirectEncoded)
	require.NoError(t, err)
	parsed, err := url.Parse(redirect)
	require.NoError(t, err)
	require.Equal(t, "/api/v1/integrations/isolar/callback", parsed.Path)
	state := parsed.Query().Get("state")
	sum := sha256.Sum256([]byte(state))
	require.Contains(t, cookie, "ekokod_oauth="+hex.EncodeToString(sum[:]))

	stranger := h.client(uaChrome)
	res = stranger.do(http.MethodGet, "/integrations/isolar/callback?code=c1&state="+url.QueryEscape(state), nil)
	require.Equal(t, http.StatusFound, res.status)
	require.Equal(t, "/ekorm/settings?tab=company&isolar=error", res.header.Get("Location"), "no cookie: refused")
	_, used := h.providers.exchanged.Load("c1")
	require.False(t, used, "the nonce is not consumed and no code is exchanged")

	res = ca.do(http.MethodGet, "/integrations/isolar/callback?code=c2&state="+url.QueryEscape(state)+"&lang=tr", nil)
	require.Equal(t, http.StatusFound, res.status)
	require.Equal(t, "/ekorm/settings?tab=company&isolar=ok", res.header.Get("Location"))
	require.Contains(t, setCookie(t, res, "ekokod_oauth"), "Max-Age=0")
	_, used = h.providers.exchanged.Load("c2")
	require.True(t, used)
}

// R210: GridBox metering points are typed in, so the API takes wiring numbers
// and a target building — and refuses both for providers that discover theirs.
func TestGridboxWiringNumbersHTTP(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := t.Context()
	_, err := admin.NewCatalogueRepository(h.pool).UpsertIntegrationDefinitions(ctx, []model.IntegrationDefinition{
		{Provider: model.IntegrationProviderGridbox, Subtype: "default", Endpoints: json.RawMessage(`{}`)},
		{Provider: model.IntegrationProviderOSOS, Subtype: "Baskent", Endpoints: json.RawMessage(`{}`)},
	})
	require.NoError(t, err)
	ca := h.as(seed.E2ECompanyAdminEmail)

	res := ca.do(http.MethodPost, "/integration-credentials", map[string]any{
		"provider": "gridbox", "subtype": "default", "username": "gb", "secret": "pw",
		"wiring_numbers": []string{"1001", "1002"}, "building_id": h.fx.BuildingA1.String(),
	})
	require.Equal(t, http.StatusCreated, res.status, string(res.body))

	var list dto.Page[dto.Analyzer]
	ca.do(http.MethodGet, "/analyzers?building_id="+h.fx.BuildingA1.String()+"&limit=100", nil).json(t, &list)
	numbers := map[string]bool{}
	for _, a := range list.Items {
		numbers[a.InstallationNumber] = true
	}
	require.True(t, numbers["1001"] && numbers["1002"], "both wiring numbers became analyzers of A1")

	// Another company's building is invisible, and a discovering provider takes neither field.
	bad := ca.do(http.MethodPost, "/integration-credentials", map[string]any{
		"provider": "osos", "subtype": "Baskent", "username": "o", "secret": "p", "wiring_numbers": []string{"1003"}})
	require.Equal(t, http.StatusUnprocessableEntity, bad.status)
	require.Equal(t, "validation_failed", bad.code(t))

	foreign := ca.do(http.MethodPost, "/integration-credentials", map[string]any{
		"provider": "osos", "subtype": "Baskent", "username": "o", "secret": "p", "building_id": h.fx.BuildingB1.String()})
	require.Equal(t, http.StatusUnprocessableEntity, foreign.status)
}
