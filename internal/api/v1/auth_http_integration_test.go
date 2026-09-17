//go:build integration

package v1_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
)

func TestWebLoginMeRefreshLogout(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	c := h.client(uaChrome)

	res := c.login(seed.E2ECompanyAdminEmail, false)
	at := setCookie(t, res, "ekokod_at")
	for _, attr := range []string{"Path=/", "Max-Age=900", "HttpOnly", "Secure", "SameSite=Strict"} {
		require.Contains(t, at, attr)
	}
	rt := setCookie(t, res, "ekokod_rt")
	for _, attr := range []string{"Path=/", "HttpOnly", "Secure", "SameSite=Strict"} {
		require.Contains(t, rt, attr)
	}
	require.NotContains(t, rt, "Max-Age", "without remember-me the refresh cookie is a browser-session cookie")

	var me dto.Me
	res = c.do(http.MethodGet, "/auth/me", nil)
	require.Equal(t, http.StatusOK, res.status)
	res.json(t, &me)
	require.Equal(t, dto.Role("company_admin"), me.Role)
	require.Equal(t, h.fx.CompanyA, me.Company.ID)
	require.Equal(t, "E2E Şirket A", me.Company.Name)
	perms := make([]string, len(me.Permissions))
	for i, p := range me.Permissions {
		perms[i] = string(p)
	}
	require.Equal(t, auth.PermissionsFor(model.UserRoleCompanyAdmin), perms)

	oldRefresh := c.cookie("ekokod_rt").Value
	h.clock.Advance(time.Minute)
	res = c.do(http.MethodPost, "/auth/refresh", nil)
	require.Equal(t, http.StatusNoContent, res.status, string(res.body))
	require.NotEqual(t, oldRefresh, c.cookie("ekokod_rt").Value)
	require.Equal(t, http.StatusOK, c.do(http.MethodGet, "/auth/me", nil).status)

	thief := h.client(uaChrome)
	h.clock.Advance(31 * time.Second)
	u := c.cookie("ekokod_rt")
	thief.http.Jar.SetCookies(mustURL(t, h.srv.URL), []*http.Cookie{{Name: "ekokod_rt", Value: oldRefresh, Path: "/", Secure: true}})
	res = thief.do(http.MethodPost, "/auth/refresh", nil)
	require.Equal(t, http.StatusUnauthorized, res.status)
	require.Equal(t, "session_revoked", res.code(t))
	require.NotNil(t, u)
	require.Equal(t, http.StatusUnauthorized, c.do(http.MethodGet, "/auth/me", nil).status, "reuse detection revoked the live session too")

	c2 := h.client(uaChrome)
	c2.login(seed.E2ECompanyAdminEmail, false)
	res = c2.do(http.MethodPost, "/auth/logout", nil)
	require.Equal(t, http.StatusNoContent, res.status)
	require.Contains(t, setCookie(t, res, "ekokod_at"), "Max-Age=0")
	res = c2.do(http.MethodGet, "/auth/me", nil)
	require.Equal(t, http.StatusUnauthorized, res.status)
}

func TestRememberMeCookieLifetime(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	res := h.client(uaChrome).login(seed.E2ECompanyAdminEmail, true)
	require.Contains(t, setCookie(t, res, "ekokod_rt"), "Max-Age=2592000")
}

func TestDeviceMismatchInvalidatesSession(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	c := h.client(uaChrome)
	c.login(seed.E2EBuildingAdminEmail, false)

	stolen := h.client(uaSafari)
	stolen.http.Jar.SetCookies(mustURL(t, h.srv.URL), c.http.Jar.Cookies(mustURL(t, h.srv.URL)))
	res := stolen.do(http.MethodGet, "/auth/me", nil)
	require.Equal(t, http.StatusUnauthorized, res.status)
	require.Equal(t, "device_mismatch", res.code(t))
	require.Contains(t, setCookie(t, res, "ekokod_at"), "Max-Age=0")
	require.Contains(t, setCookie(t, res, "ekokod_rt"), "Max-Age=0")

	res = c.do(http.MethodGet, "/auth/me", nil)
	require.Equal(t, http.StatusUnauthorized, res.status)
	require.Equal(t, "session_revoked", res.code(t), "the original device is signed out too")
}

func TestMobileBearerAuthenticatesNonAuthEndpoint(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	app := h.client("ekokod-mobile/1.4 (Android 15)")
	res := app.do(http.MethodPost, "/mobile/auth/login", map[string]any{"email": seed.E2ECompanyReadonlyEmail, "password": testPassword})
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	require.Empty(t, res.header.Values("Set-Cookie"), "mobile login sets no cookies")
	var tokens dto.MobileTokens
	res.json(t, &tokens)
	require.Equal(t, "Bearer", tokens.TokenType)
	require.EqualValues(t, 900, tokens.ExpiresIn)
	require.Equal(t, dto.Role("company_readonly_admin"), tokens.User.Role)

	app.bearer = tokens.AccessToken
	res = app.do(http.MethodGet, "/profile", nil)
	require.Equal(t, http.StatusOK, res.status)

	app.bearer = tokens.AccessToken[:len(tokens.AccessToken)-2] + "xx"
	res = app.do(http.MethodGet, "/profile", nil)
	require.Equal(t, http.StatusUnauthorized, res.status)
	require.Equal(t, "token_invalid", res.code(t))

	app.bearer = tokens.AccessToken
	h.clock.Advance(16 * time.Minute)
	res = app.do(http.MethodGet, "/profile", nil)
	require.Equal(t, "token_expired", res.code(t))

	app.bearer = ""
	res = app.do(http.MethodPost, "/mobile/auth/refresh", map[string]any{"refresh_token": tokens.RefreshToken})
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	var rotated dto.MobileTokens
	res.json(t, &rotated)
	require.NotEqual(t, tokens.RefreshToken, rotated.RefreshToken)
	app.bearer = rotated.AccessToken
	require.Equal(t, http.StatusOK, app.do(http.MethodGet, "/mobile/auth/me", nil).status)

	web := h.client("ekokod-mobile/1.4 (Android 15)")
	res = web.do(http.MethodPost, "/mobile/auth/refresh", map[string]any{"refresh_token": "not-a-token"})
	require.Equal(t, "session_revoked", res.code(t))
}

func TestLoginRateLimited(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(c *config.Config) { c.HTTP.RateLimitAuth = config.RateLimit{Limit: 5, Window: 15 * time.Minute} })
	c := h.client(uaChrome)
	for range 5 {
		res := c.do(http.MethodPost, "/auth/login", map[string]any{"email": seed.E2ECompanyAdminEmail, "password": "Yanlis!Sifre-42"})
		require.Equal(t, http.StatusUnauthorized, res.status)
		require.Equal(t, "invalid_credentials", res.code(t))
	}
	res := c.do(http.MethodPost, "/auth/login", map[string]any{"email": seed.E2ECompanyAdminEmail, "password": testPassword})
	require.Equal(t, http.StatusTooManyRequests, res.status)
	require.Equal(t, "rate_limited", res.code(t))
	require.NotEmpty(t, res.header.Get("Retry-After"))
}

func TestForgotPasswordAlways202(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	c := h.client(uaChrome)
	known := c.do(http.MethodPost, "/auth/forgot-password", map[string]any{"email": seed.E2ECompanyAdminEmail})
	unknown := c.do(http.MethodPost, "/auth/forgot-password", map[string]any{"email": "nobody@e2e.ekokod.test"})
	require.Equal(t, http.StatusAccepted, known.status)
	require.Equal(t, http.StatusAccepted, unknown.status)
	require.Equal(t, string(known.body), string(unknown.body))
	res := c.do(http.MethodPost, "/auth/reset-password", map[string]any{"token": "nope", "password": "Yepyeni!Sifre-77"})
	require.Equal(t, http.StatusBadRequest, res.status)
	require.Equal(t, "reset_token_invalid", res.code(t))
}

func TestProfileUpdateRoles(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	demo := h.client(uaChrome)
	demo.login(h.demoUser(), false)
	res := demo.do(http.MethodPatch, "/profile", map[string]any{"name": "Değişti"})
	require.Equal(t, http.StatusForbidden, res.status)
	res = demo.do(http.MethodPost, "/auth/change-password", map[string]any{"current_password": testPassword, "new_password": "Yepyeni!Sifre-77"})
	require.Equal(t, http.StatusForbidden, res.status)
	require.Equal(t, http.StatusNoContent, demo.do(http.MethodPost, "/auth/logout", nil).status, "demo may still sign out")

	cr := h.client(uaChrome)
	cr.login(seed.E2ECompanyReadonlyEmail, false)
	res = cr.do(http.MethodPatch, "/profile", map[string]any{"name": "Can Demir", "locale": "en", "ui_preferences": "theme=dark"})
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	var me dto.Me
	res.json(t, &me)
	require.Equal(t, dto.Locale("en"), me.Locale)
	require.Equal(t, "theme=dark", *me.UIPreferences)

	res = cr.do(http.MethodPatch, "/profile", map[string]any{"email": seed.E2ECompanyAdminEmail})
	require.Equal(t, http.StatusConflict, res.status)
	require.Equal(t, "email_taken", res.code(t))
	res = cr.do(http.MethodPatch, "/profile", map[string]any{"locale": "de"})
	require.Equal(t, http.StatusUnprocessableEntity, res.status)
	res = cr.do(http.MethodPatch, "/profile", map[string]any{"role": "admin"})
	require.Equal(t, http.StatusBadRequest, res.status, "unknown fields are refused")
	require.Equal(t, "invalid_body", res.code(t))
	res = cr.do(http.MethodPatch, "/profile", map[string]any{"name": "x"}, "Content-Type", "text/plain")
	require.Equal(t, http.StatusUnsupportedMediaType, res.status)
}

func TestSessionsListAndRevokeHTTP(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	phone := h.client(uaSafari)
	phone.login(seed.E2EBuildingAdminEmail, false)
	laptop := h.client(uaChrome)
	laptop.login(seed.E2EBuildingAdminEmail, false)

	var list dto.SessionList
	res := laptop.do(http.MethodGet, "/auth/sessions", nil)
	require.Equal(t, http.StatusOK, res.status)
	res.json(t, &list)
	require.Len(t, list.Items, 2)
	var other dto.Session
	for _, s := range list.Items {
		require.True(t, strings.HasSuffix(s.CreatedAt.Format(time.RFC3339), "+03:00"), "times are rendered in Istanbul")
		if !s.Current {
			other = s
		}
	}
	require.Equal(t, uaSafari, *other.UserAgent)
	require.Equal(t, http.StatusNoContent, laptop.do(http.MethodDelete, "/auth/sessions/"+other.ID.String(), nil).status)
	require.Equal(t, http.StatusUnauthorized, phone.do(http.MethodGet, "/auth/me", nil).status)

	stranger := h.client(uaChrome)
	stranger.login(seed.E2ECompanyAdminEmail, false)
	res = stranger.do(http.MethodDelete, "/auth/sessions/"+list.Items[0].ID.String(), nil)
	require.Equal(t, http.StatusNotFound, res.status)
}

func TestRefreshRotatedKeepsCookies(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tabA := h.client(uaChrome)
	tabA.login(seed.E2ECompanyAdminEmail, true)
	old := tabA.cookie("ekokod_rt").Value
	require.Equal(t, http.StatusNoContent, tabA.do(http.MethodPost, "/auth/refresh", nil).status)

	// A second tab still holding the pre-rotation cookie refreshes within the grace window.
	tabB := h.client(uaChrome)
	tabB.http.Jar.SetCookies(mustURL(t, h.srv.URL), []*http.Cookie{{Name: "ekokod_rt", Value: old, Path: "/", Secure: true}})
	h.clock.Advance(5 * time.Second)
	res := tabB.do(http.MethodPost, "/auth/refresh", nil)
	require.Equal(t, http.StatusUnauthorized, res.status)
	require.Equal(t, "token_rotated", res.code(t))
	require.Empty(t, res.header.Values("Set-Cookie"), "the other tab's fresh cookies must not be cleared")
	require.Equal(t, http.StatusOK, tabA.do(http.MethodGet, "/auth/me", nil).status)
}
