//go:build integration

package v1_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
)

func TestCompanyAdminCannotReadOtherCompany(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	own := ca.do(http.MethodGet, "/companies/"+h.fx.CompanyA.String(), nil)
	require.Equal(t, http.StatusOK, own.status, string(own.body))
	var detail dto.CompanyDetail
	own.json(t, &detail)
	require.Equal(t, []dto.AnalyzerCount{{Provider: "osos", Subtype: "Baskent", Count: 2}}, detail.AnalyzerCounts)

	require.Equal(t, http.StatusNotFound, ca.do(http.MethodGet, "/companies/"+h.fx.CompanyB.String(), nil).status)
	require.Equal(t, http.StatusNotFound, ca.do(http.MethodGet, "/companies/"+h.fx.CompanyB.String()+"?company_id="+h.fx.CompanyB.String(), nil).status)
	require.Equal(t, http.StatusNotFound, ca.do(http.MethodPatch, "/companies/"+h.fx.CompanyB.String(), map[string]any{"name": "Ele geçirildi"}).status)
	require.Equal(t, http.StatusForbidden, ca.do(http.MethodGet, "/companies", nil).status)

	cr := h.as(seed.E2ECompanyReadonlyEmail)
	require.Equal(t, http.StatusOK, cr.do(http.MethodGet, "/companies/"+h.fx.CompanyA.String(), nil).status)
	require.Equal(t, http.StatusForbidden, cr.do(http.MethodPatch, "/companies/"+h.fx.CompanyA.String(), map[string]any{"name": "x"}).status)

	res := ca.do(http.MethodPatch, "/companies/"+h.fx.CompanyA.String(), map[string]any{"total_area_m2": "12500.50", "sector": ""})
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	var updated dto.Company
	res.json(t, &updated)
	require.Equal(t, "12500.5", updated.TotalAreaM2.String())
	require.Nil(t, updated.Sector, `"" clears a field`)
	require.Equal(t, "E2E Şirket A", updated.Name, "omitted fields are kept")
}

func TestAdminCompanyLifecycle(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	admin := h.as(seed.E2EAdminEmail)
	res := admin.do(http.MethodPost, "/companies", map[string]any{"name": "Yeni Müşteri", "personnel_count": 12})
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	var created dto.Company
	res.json(t, &created)

	res = admin.do(http.MethodPost, "/companies", map[string]any{"name": "yeni müşteri"})
	require.Equal(t, http.StatusConflict, res.status)
	require.Equal(t, "company_name_taken", res.code(t))

	var page dto.Page[dto.Company]
	res = admin.do(http.MethodGet, "/companies?q=Müşteri", nil)
	require.Equal(t, http.StatusOK, res.status)
	res.json(t, &page)
	require.Len(t, page.Items, 1)
	require.Equal(t, created.ID, page.Items[0].ID)

	require.Equal(t, http.StatusOK, admin.do(http.MethodGet, "/companies/"+h.fx.CompanyB.String(), nil).status, "the admin reads any company by id")
	require.Equal(t, http.StatusNoContent, admin.do(http.MethodDelete, "/companies/"+created.ID.String(), nil).status)
	res = admin.do(http.MethodGet, "/companies?q=Müşteri", nil)
	res.json(t, &page)
	require.Empty(t, page.Items)

	res = admin.do(http.MethodDelete, "/companies/"+h.fx.PlatformCompany.String(), nil)
	require.Equal(t, http.StatusConflict, res.status)
	require.Equal(t, "cannot_delete_own_company", res.code(t))

	var audited int
	require.NoError(t, h.pool.QueryRow(t.Context(), `select count(*) from audit_log where company_id is null and action in ('companies.create','companies.delete')`).Scan(&audited))
	require.Equal(t, 2, audited, "company create and delete are platform audit rows")
}

func TestUserRoleOptions(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	user := func(role, email string) map[string]any {
		return map[string]any{"name": "Selin Koç", "email": email, "role": role, "password": "Guclu!Parola-91"}
	}
	res := ca.do(http.MethodPost, "/users", user("admin", "yeni1@a.e2e.ekokod.test"))
	require.Equal(t, http.StatusUnprocessableEntity, res.status)
	var env dto.Error
	res.json(t, &env)
	require.Equal(t, map[string]any{"role": []any{"not_assignable"}}, env.Error.Details)

	res = ca.do(http.MethodPost, "/users", user("building_admin", "yeni2@a.e2e.ekokod.test"))
	require.Equal(t, http.StatusCreated, res.status, string(res.body))

	admin := h.as(seed.E2EAdminEmail)
	res = admin.do(http.MethodPost, "/users?company_id="+h.fx.CompanyB.String(), user("demo", "yeni3@b.e2e.ekokod.test"))
	require.Equal(t, http.StatusUnprocessableEntity, res.status)
	res = admin.do(http.MethodPost, "/users?company_id="+h.fx.CompanyB.String(), user("company_readonly_admin", "yeni4@b.e2e.ekokod.test"))
	require.Equal(t, http.StatusCreated, res.status, string(res.body))

	res = ca.do(http.MethodPost, "/users", user("building_admin", seed.E2ECompanyReadonlyEmail))
	require.Equal(t, http.StatusConflict, res.status)
	require.Equal(t, "email_taken", res.code(t))

	var page dto.Page[dto.User]
	res = ca.do(http.MethodGet, "/users?limit=2", nil)
	require.Equal(t, http.StatusOK, res.status)
	res.json(t, &page)
	require.Len(t, page.Items, 2)
	require.NotNil(t, page.NextCursor)
	res = ca.do(http.MethodGet, "/users?role=building_admin", nil)
	res.json(t, &page)
	require.Len(t, page.Items, 2)
	for _, u := range page.Items {
		require.NotEqual(t, "yeni4@b.e2e.ekokod.test", u.Email, "users of company B never appear in A")
	}
	require.Equal(t, http.StatusBadRequest, ca.do(http.MethodGet, "/users?role=root", nil).status)
}

func TestUserSelfModificationAndAdminProtection(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	self := h.fx.Users[seed.E2ECompanyAdminEmail].String()
	res := ca.do(http.MethodDelete, "/users/"+self, nil)
	require.Equal(t, http.StatusConflict, res.status)
	require.Equal(t, "cannot_modify_self", res.code(t))
	require.Equal(t, http.StatusConflict, ca.do(http.MethodPatch, "/users/"+self, map[string]any{"role": "building_admin"}).status)
	require.Equal(t, http.StatusOK, ca.do(http.MethodPatch, "/users/"+self, map[string]any{"phone": "+90 555 000 0000"}).status)

	require.Equal(t, http.StatusNotFound, ca.do(http.MethodPatch, "/users/"+h.fx.Users[seed.E2ECompanyBAdminEmail].String(), map[string]any{"name": "x"}).status)
	require.Equal(t, http.StatusNotFound, ca.do(http.MethodDelete, "/users/"+h.fx.Users[seed.E2EAdminEmail].String(), nil).status, "another company's admin is not visible")
}

func TestUserRoleChangeRevokesSessions(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ba := h.as(seed.E2EBuildingAdminEmail)
	require.Equal(t, http.StatusOK, ba.do(http.MethodGet, "/auth/me", nil).status)
	ca := h.as(seed.E2ECompanyAdminEmail)
	res := ca.do(http.MethodPatch, "/users/"+h.fx.Users[seed.E2EBuildingAdminEmail].String(), map[string]any{"role": "building_readonly_admin"})
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	res = ba.do(http.MethodGet, "/auth/me", nil)
	require.Equal(t, http.StatusUnauthorized, res.status)
	require.Equal(t, "session_revoked", res.code(t))

	br := h.as(seed.E2EBuildingReadonlyEmail)
	require.Equal(t, http.StatusNoContent, ca.do(http.MethodDelete, "/users/"+h.fx.Users[seed.E2EBuildingReadonlyEmail].String(), nil).status)
	require.Equal(t, http.StatusUnauthorized, br.do(http.MethodGet, "/auth/me", nil).status)
}

func TestUserCreateEnforcesPolicy(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	res := ca.do(http.MethodPost, "/users", map[string]any{"name": "Selin Koç", "email": "selin@a.e2e.ekokod.test", "role": "building_admin", "password": "selin123"})
	require.Equal(t, http.StatusUnprocessableEntity, res.status)
	var env dto.Error
	res.json(t, &env)
	codes := env.Error.Details["password"].([]any)
	require.Contains(t, codes, "password_too_short")
	require.Contains(t, codes, "password_personal")
}

func TestSMTPPasswordNeverReturned(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	admin := h.as(seed.E2EAdminEmail)
	target := "?company_id=" + h.fx.CompanyA.String()
	require.Equal(t, http.StatusNotFound, admin.do(http.MethodGet, "/smtp-settings"+target, nil).status)

	settings := map[string]any{"host": "smtp.example.com", "port": 587, "secure": false, "username": "mailer", "from_address": "bildirim@a.example.com"}
	res := admin.do(http.MethodPut, "/smtp-settings"+target, settings)
	require.Equal(t, http.StatusUnprocessableEntity, res.status, "the first save needs a password")

	settings["password"] = "Sentinel-SMTP-9f2c"
	res = admin.do(http.MethodPut, "/smtp-settings"+target, settings)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	require.NotContains(t, string(res.body), "Sentinel-SMTP-9f2c")
	delete(settings, "password")
	settings["port"] = 465
	res = admin.do(http.MethodPut, "/smtp-settings"+target, settings)
	require.Equal(t, http.StatusOK, res.status, "an omitted password keeps the stored one")
	res = admin.do(http.MethodGet, "/smtp-settings"+target, nil)
	require.Equal(t, http.StatusOK, res.status)
	require.NotContains(t, string(res.body), "Sentinel-SMTP-9f2c")
	var got dto.SMTPSettings
	res.json(t, &got)
	require.True(t, got.HasPassword)
	require.EqualValues(t, 465, got.Port)

	res = admin.do(http.MethodPost, "/smtp-settings/test"+target, map[string]any{"to": "alici@example.com"})
	require.Equal(t, http.StatusNoContent, res.status, string(res.body))
	require.Len(t, h.mail.msgs, 1)

	h.mail.fail = errors.New("535 authentication failed")
	res = admin.do(http.MethodPost, "/smtp-settings/test"+target, map[string]any{"to": "alici@example.com"})
	require.Equal(t, http.StatusUnprocessableEntity, res.status)
	require.Equal(t, "smtp_test_failed", res.code(t))
	require.True(t, strings.Contains(string(res.body), "535 authentication failed"))
	require.NotContains(t, string(res.body), "Sentinel-SMTP-9f2c", "the password is redacted from the failure reason")

	require.Equal(t, http.StatusForbidden, h.as(seed.E2ECompanyAdminEmail).do(http.MethodGet, "/smtp-settings", nil).status)
}
