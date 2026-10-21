//go:build integration

package v1_test

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
)

// fkIDs are the objects one side of an FK pair names in a body.
type fkIDs struct {
	building, analyzer, user, plant, isolarCred, pmCred string
}

// fkCase sends body(own) as the control, then body(foreign) for each foreign
// side; the control must succeed so a refusal proves scoping, not binding.
type fkCase struct {
	op, method string
	path       func(own fkIDs) string
	body       func(ids fkIDs) any
}

// overwritten wraps a body whose foreign field the service replaces with a
// validated id; a 2xx is then allowed as long as the foreign id appears in no
// response or row.
type overwritten struct {
	why  string
	body any
}

func unwrap(b any) (any, string) {
	if o, ok := b.(overwritten); ok {
		return o.body, o.why
	}
	return b, ""
}

func (h *harness) created(t *testing.T, res response) string {
	t.Helper()
	require.Less(t, res.status, 300, string(res.body))
	var v struct {
		ID uuid.UUID `json:"id"`
	}
	res.json(t, &v)
	return v.ID.String()
}

// fkCredentials creates an iSolar and a PM5340 credential for c's company.
func (h *harness) fkCredentials(t *testing.T, c *client, building uuid.UUID) (isolar, pm string) {
	t.Helper()
	isolar = h.created(t, c.do(http.MethodPost, "/integration-credentials", map[string]any{
		"provider": "isolar", "subtype": "EU", "username": "u", "secret": "s", "extra": map[string]string{"app_key": "k", "app_id": "a"}}))
	pm = h.created(t, c.do(http.MethodPost, "/integration-credentials", map[string]any{
		"provider": "pm5340", "subtype": "Default", "pm5340_url": "https://pm5340.example/api", "building_id": building.String()}))
	return isolar, pm
}

// alarmBody is commsBody with a unique name; rule names are unique per company.
func alarmBody(h *harness, analyzers ...string) map[string]any {
	b := commsBody(h, 6, analyzers...)
	b["name"] = "FK " + uuid.NewString()[:8]
	return b
}

func tariffFields(building string) map[string]any {
	t := validTariff(uuid.Nil)
	if building == "" {
		delete(t, "building_id")
	} else {
		t["building_id"] = building
	}
	return t
}

// TestForeignKeysAreRefused is the body-id half of BOLA: a company admin's
// AllBuildings scope says yes to any building id in memory, so every write
// that stores or acts on a body id must check it against the company in SQL.
func TestForeignKeysAreRefused(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.ml.err = nil
	_, _, err := seed.LoadEmissionFactors(t.Context(), h.pool, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	admin := h.as(seed.E2EAdminEmail)
	for _, d := range []map[string]any{
		definition("isolar", "EU", map[string]string{"gateway": "https://gateway.isolar.example", "authorize_origin": "https://web3.isolar.example", "cloud_id": "3"}),
		definition("pm5340", "Default", map[string]string{"hourly_values": "/hourly"}),
		definition("gridbox", "Default", map[string]string{"token": "https://gridbox.example/token"}),
		definition("aril", "Default", map[string]string{"authentication": "https://aril.example/login"}),
	} {
		require.Equal(t, http.StatusCreated, admin.do(http.MethodPost, "/integration-definitions", d).status)
	}
	ca, cb := h.as(seed.E2ECompanyAdminEmail), h.as(seed.E2ECompanyBAdminEmail)
	for _, x := range []struct {
		c *client
		b uuid.UUID
	}{{ca, h.fx.BuildingA1}, {ca, h.fx.BuildingA2}, {cb, h.fx.BuildingB1}} {
		res := x.c.do(http.MethodPut, "/carbon/selected-activities?building_id="+x.b.String(), map[string]any{"activity_keys": []string{"sub_waste_disposal"}})
		require.Equal(t, http.StatusOK, res.status, string(res.body))
	}
	var factors dto.EmissionFactorList
	ca.do(http.MethodGet, "/carbon/emission-factors?sub_category=sub_waste_disposal", nil).json(t, &factors)
	require.NotEmpty(t, factors.Items)
	factor := factors.Items[0]

	isoA, pmA := h.fkCredentials(t, ca, h.fx.BuildingA1)
	isoB, pmB := h.fkCredentials(t, cb, h.fx.BuildingB1)
	a1 := fkIDs{building: h.fx.BuildingA1.String(), analyzer: h.fx.AnalyzerA1.String(), user: h.fx.Users[seed.E2EBuildingAdminEmail].String(),
		plant: ca.plant(t, "FK GES A").String(), isolarCred: isoA, pmCred: pmA}
	a2 := fkIDs{building: h.fx.BuildingA2.String(), analyzer: h.fx.AnalyzerA2.String(), user: h.fx.Users[seed.E2EBuildingReadonlyEmail].String()}
	b1 := fkIDs{building: h.fx.BuildingB1.String(), analyzer: h.fx.AnalyzerB1.String(), user: h.fx.Users[seed.E2ECompanyBAdminEmail].String(),
		plant: cb.plant(t, "FK GES B").String(), isolarCred: isoB, pmCred: pmB}

	alarm := h.created(t, ca.do(http.MethodPost, "/alarms", alarmBody(h)))
	tariff := h.created(t, ca.do(http.MethodPost, "/tariffs", validTariff(h.fx.BuildingA1)))
	template := h.created(t, ca.do(http.MethodPost, "/tariff-templates", map[string]any{"name": "FK", "tariff": tariffFields("")}))
	// users.update's control re-assigns a building of its own, so the BA/BR fixtures keep theirs.
	spare := h.created(t, ca.do(http.MethodPost, "/buildings", map[string]any{"name": "FK Yedek"}))
	member := h.created(t, ca.do(http.MethodPost, "/users", map[string]any{"name": "Fk Üye", "email": "fk-uye@a.e2e.ekokod.test",
		"role": "building_admin", "password": "Guclu!Parola-91"}))
	n := 0
	email := func() string { n++; return "fk-" + uuid.NewString()[:8] + "@a.e2e.ekokod.test" }

	fixed := func(p string) func(fkIDs) string { return func(fkIDs) string { return p } }
	cases := []fkCase{
		{"alarms.create", http.MethodPost, fixed("/alarms"), func(x fkIDs) any { return alarmBody(h, x.analyzer) }},
		{"alarms.create", http.MethodPost, fixed("/alarms"), func(x fkIDs) any {
			if x.analyzer == a1.analyzer {
				return alarmBody(h, a1.analyzer)
			}
			return alarmBody(h, a1.analyzer, x.analyzer)
		}},
		{"alarms.update", http.MethodPatch, fixed("/alarms/" + alarm), func(x fkIDs) any { return alarmBody(h, x.analyzer) }},
		{"analyzers.update", http.MethodPatch, fixed("/analyzers/" + a1.analyzer), func(x fkIDs) any { return map[string]any{"building_id": x.building} }},
		{"anomaly.check", http.MethodPost, fixed("/anomaly/check"), func(x fkIDs) any {
			return map[string]any{"analyzer_id": x.analyzer, "ts": "2026-09-16T10:00:00+03:00", "actual": "4"}
		}},
		{"bills.compute", http.MethodPost, fixed("/bills/compute"), func(x fkIDs) any {
			return map[string]any{"scope": "building", "period": "2026-08", "building_ids": []string{x.building}}
		}},
		{"bills.compute", http.MethodPost, fixed("/bills/compute"), func(x fkIDs) any {
			return map[string]any{"scope": "analyzer", "period": "2026-08", "analyzer_ids": []string{x.analyzer}}
		}},
		{"buildings.create", http.MethodPost, fixed("/buildings"), func(x fkIDs) any {
			return map[string]any{"name": "FK " + uuid.NewString()[:6], "responsible_user_id": x.user}
		}},
		{"buildings.update", http.MethodPatch, fixed("/buildings/" + spare), func(x fkIDs) any { return map[string]any{"responsible_user_id": x.user} }},
		{"bulk_tariff.assign", http.MethodPost, fixed("/buildings/bulk-tariff"), func(x fkIDs) any {
			return map[string]any{"building_ids": []string{x.building}, "tariff": tariffFields("")}
		}},
		{"bulk_tariff.assign", http.MethodPost, fixed("/buildings/bulk-tariff"), func(x fkIDs) any {
			return overwritten{"bulkAssign sets each tariff's building to a listed, validated building",
				map[string]any{"building_ids": []string{a1.building}, "tariff": tariffFields(x.building)}}
		}},
		{"carbon.activities.create", http.MethodPost, fixed("/carbon/activities"), func(x fkIDs) any {
			return map[string]any{"building_id": x.building, "sub_category": "sub_waste_disposal", "factor_key": factor.Key, "quantity": "2",
				"unit": factor.BaseUnit, "period_start": "2026-08-01", "period_end": "2026-08-31"}
		}},
		{"carbon.reports.create", http.MethodPost, fixed("/carbon/reports"), func(x fkIDs) any {
			return map[string]any{"building_id": x.building, "from": "2026-08-01", "to": "2026-08-31", "report_type": "ghg"}
		}},
		{"forecast.run", http.MethodPost, fixed("/forecast/run"), func(x fkIDs) any { return map[string]any{"analyzer_id": x.analyzer, "horizon_hours": 24} }},
		{"forecast.weekly", http.MethodPost, fixed("/forecast/weekly"), func(x fkIDs) any {
			return map[string]any{"analyzer_id": x.analyzer, "week_start": "2026-09-14"}
		}},
		{"forecast.monthly", http.MethodPost, fixed("/forecast/monthly"), func(x fkIDs) any { return map[string]any{"analyzer_id": x.analyzer, "month": "2026-10"} }},
		{"integration_credentials.create", http.MethodPost, fixed("/integration-credentials"), func(x fkIDs) any {
			// One credential per provider and company: the control and the attack use different ones.
			provider := "aril"
			if x.building == a1.building {
				provider = "gridbox"
			}
			return map[string]any{"provider": provider, "subtype": "Default", "username": "u", "secret": "s", "building_id": x.building}
		}},
		{"integration_credentials.update", http.MethodPatch, fixed("/integration-credentials/" + pmA), func(x fkIDs) any { return map[string]any{"building_id": x.building} }},
		{"integration_credentials.backfill", http.MethodPost, fixed("/integration-credentials/" + pmA + "/backfill"), func(x fkIDs) any {
			return map[string]any{"analyzer_ids": []string{x.analyzer}, "from": "2026-08-01T00:00:00+03:00", "to": "2026-08-02T00:00:00+03:00"}
		}},
		{"plants.create", http.MethodPost, fixed("/power-plants"), func(x fkIDs) any {
			return map[string]any{"name": "FK " + uuid.NewString()[:6], "netting_analyzer_id": x.analyzer}
		}},
		{"plants.update", http.MethodPatch, fixed("/power-plants/" + a1.plant), func(x fkIDs) any { return map[string]any{"netting_analyzer_id": x.analyzer} }},
		{"plants.isolar_link", http.MethodPost, fixed("/plants/" + a1.plant + "/isolar-link"), func(x fkIDs) any {
			return map[string]any{"credential_id": x.isolarCred, "ps_id": "PS-FK-" + uuid.NewString()[:6]}
		}},
		{"reports.generate", http.MethodPost, fixed("/reports/generate"), func(x fkIDs) any {
			return map[string]any{"type": "monthly", "period": "2026-08", "plant_selection": "all", "building_ids": []string{x.building}}
		}},
		{"reports.generate", http.MethodPost, fixed("/reports/generate"), func(x fkIDs) any {
			return map[string]any{"type": "monthly", "period": "2026-07", "plant_selection": "rooftop", "building_ids": []string{a1.building}, "plant_ids": []string{x.plant}}
		}},
		{"solar_tariffs.create", http.MethodPost, fixed("/solar-tariffs"), func(x fkIDs) any {
			return map[string]any{"plant_id": x.plant, "effective_from": "2026-01-01", "feed_in_tariff": "1.5"}
		}},
		{"tariff_templates.apply", http.MethodPost, fixed("/tariff-templates/" + template + "/apply"), func(x fkIDs) any {
			return map[string]any{"building_ids": []string{x.building}, "effective_from": "2026-09-01"}
		}},
		{"tariff_templates.create", http.MethodPost, fixed("/tariff-templates"), func(x fkIDs) any {
			return overwritten{"templates.payload clears the tariff's building, company and id",
				map[string]any{"name": "FK " + uuid.NewString()[:6], "tariff": tariffFields(x.building)}}
		}},
		{"tariff_templates.update", http.MethodPatch, fixed("/tariff-templates/" + template), func(x fkIDs) any {
			return overwritten{"templates.payload clears the tariff's building, company and id",
				map[string]any{"name": "FK", "tariff": tariffFields(x.building)}}
		}},
		{"tariffs.create", http.MethodPost, fixed("/tariffs"), func(x fkIDs) any { return validTariff(uuid.MustParse(x.building)) }},
		{"tariffs.update", http.MethodPatch, fixed("/tariffs/" + tariff), func(x fkIDs) any { return validTariff(uuid.MustParse(x.building)) }},
		{"users.create", http.MethodPost, fixed("/users"), func(x fkIDs) any {
			b := x.building
			if b == a1.building {
				b = spare
			}
			return map[string]any{"name": "Fk Kişi", "email": email(), "role": "building_admin", "password": "Guclu!Parola-91", "building_ids": []string{b}}
		}},
		{"users.update", http.MethodPatch, fixed("/users/" + member), func(x fkIDs) any {
			b := x.building
			if b == a1.building {
				b = spare
			}
			return map[string]any{"building_ids": []string{b}}
		}},
	}
	seen := map[string]bool{}
	for _, cs := range cases {
		seen[cs.op] = true
	}
	for op := range fkCovered {
		require.True(t, seen[op], "fkCovered names %s but TestForeignKeysAreRefused has no case for it", op)
	}

	// A company admin's foreign side is company B; a building admin's is A2 and B1.
	baRoutes := map[string]bool{"alarms.create": true, "alarms.update": true, "anomaly.check": true, "bills.compute": true,
		"forecast.run": true, "forecast.weekly": true, "forecast.monthly": true, "reports.generate": true}
	ba := h.as(seed.E2EBuildingAdminEmail)
	baAlarm := h.created(t, ba.do(http.MethodPost, "/alarms", alarmBody(h)))
	for _, cs := range cases {
		path := cs.path(a1)
		control, _ := unwrap(cs.body(a1))
		res := ca.do(cs.method, path, control)
		if cs.op == "plants.isolar_link" {
			// The control credential was never OAuth-authorised, so the link
			// cannot finish here; it must at least get past the credential check.
			require.NotContains(t, string(res.body), `"credential_id"`, "control %s: %s", cs.op, string(res.body))
			res = ca.do(cs.method, path, cs.body(b1))
			require.Equal(t, http.StatusUnprocessableEntity, res.status, string(res.body))
			require.Contains(t, string(res.body), `"credential_id":["not_isolar"]`, "company B's credential must be unknown here")
			continue
		}
		require.Less(t, res.status, 300, "control %s %s: %s", cs.op, path, string(res.body))
		attack, why := unwrap(cs.body(b1))
		res = ca.do(cs.method, path, attack)
		if why != "" && res.status < 300 {
			require.NotContains(t, string(res.body), b1.building, "%s echoed company B's building", cs.op)
		} else {
			require.Contains(t, []int{http.StatusNotFound, http.StatusUnprocessableEntity}, res.status,
				"%s accepted company B's id: %d %s", cs.op, res.status, string(res.body))
		}
		if !baRoutes[cs.op] {
			continue
		}
		if cs.op == "alarms.update" {
			path = "/alarms/" + baAlarm
		}
		if cs.op == "reports.generate" {
			continue // plant_selection needs plants.read, which a building admin does not hold
		}
		res = ba.do(cs.method, path, cs.body(a1))
		require.Less(t, res.status, 300, "BA control %s: %s", cs.op, string(res.body))
		for _, foreign := range []fkIDs{a2, b1} {
			res = ba.do(cs.method, path, cs.body(foreign))
			require.Contains(t, []int{http.StatusNotFound, http.StatusUnprocessableEntity}, res.status,
				"BA %s accepted building %s: %d %s", cs.op, foreign.building, res.status, string(res.body))
		}
	}
	var stray int
	require.NoError(t, h.pool.QueryRow(t.Context(), `select
		(select count(*) from tariffs where company_id = $1 and building_id = $2) +
		(select count(*) from carbon_activities where company_id = $1 and building_id = $2) +
		(select count(*) from analyzers where company_id = $1 and building_id = $2) +
		(select count(*) from buildings where company_id = $1 and responsible_user_id = $3)`,
		h.fx.CompanyA, h.fx.BuildingB1, h.fx.Users[seed.E2ECompanyBAdminEmail]).Scan(&stray))
	require.Zero(t, stray, "no company A row may point at company B's building or user")
}
