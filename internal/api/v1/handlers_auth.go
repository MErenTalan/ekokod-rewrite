package v1

import (
	"errors"
	"net/http"
	"net/netip"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/kit"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	authsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/auth"
)

var (
	webWriters  = auth.Roles(roleA, roleCA, roleCR, roleBA, roleBR) // R150 self-service writes, demo excluded
	authSelfTag = "auth"
	mobileTag   = "mobile"
	profileTag  = "profile"
)

func authRoutes() []Route {
	return []Route{
		{Method: http.MethodPost, Pattern: "/auth/login", OperationID: "auth.login", Tag: authSelfTag, Access: Public,
			Summary: "Sign in with e-mail and password; sets the session cookies.", AuthLimit: AuthLimitByEmail,
			Request: dto.LoginRequest{}, Response: dto.Me{}, Status: http.StatusOK, Handler: (*Handlers).login},
		{Method: http.MethodPost, Pattern: "/auth/refresh", OperationID: "auth.refresh", Tag: authSelfTag, Access: Public,
			Summary: "Rotate the refresh cookie and issue a new access cookie.", SelfService: true,
			Status: http.StatusNoContent, Handler: (*Handlers).refresh},
		{Method: http.MethodPost, Pattern: "/auth/logout", OperationID: "auth.logout", Tag: authSelfTag, Access: Authenticated,
			Summary: "Revoke the current session.", SelfService: true, NoIdempotency: true, Entity: "session",
			Status: http.StatusNoContent, Handler: (*Handlers).logout},
		{Method: http.MethodPost, Pattern: "/auth/logout-all", OperationID: "auth.logout_all", Tag: authSelfTag, Access: Authenticated,
			Summary: "Revoke every session of the current user.", SelfService: true, NoIdempotency: true, Entity: "session",
			Status: http.StatusNoContent, Handler: (*Handlers).logoutAll},
		{Method: http.MethodGet, Pattern: "/auth/me", OperationID: "auth.me", Tag: authSelfTag, Access: Authenticated,
			Summary: "The current principal with its permissions.", Response: dto.Me{}, Status: http.StatusOK, Handler: (*Handlers).me},
		{Method: http.MethodPost, Pattern: "/auth/forgot-password", OperationID: "auth.forgot_password", Tag: authSelfTag, Access: Public,
			Summary: "Send a password reset link; always 202.", AuthLimit: AuthLimitByEmail,
			Request: dto.ForgotPasswordRequest{}, Response: dto.Empty{}, Status: http.StatusAccepted, Handler: (*Handlers).forgotPassword},
		{Method: http.MethodPost, Pattern: "/auth/reset-password", OperationID: "auth.reset_password", Tag: authSelfTag, Access: Public,
			Summary: "Set a new password with a reset token.", AuthLimit: AuthLimitByIP,
			Request: dto.ResetPasswordRequest{}, Status: http.StatusNoContent, Handler: (*Handlers).resetPassword},
		{Method: http.MethodPost, Pattern: "/auth/change-password", OperationID: "auth.change_password", Tag: authSelfTag,
			Access: RoleGated, Roles: webWriters, SelfService: true, NoIdempotency: true, Entity: "user",
			Summary: "Change the current user's password; other sessions are revoked.",
			Request: dto.ChangePasswordRequest{}, Status: http.StatusNoContent, Handler: (*Handlers).changePassword},
		{Method: http.MethodGet, Pattern: "/auth/sessions", OperationID: "auth.sessions.list", Tag: authSelfTag, Access: Authenticated,
			Summary: "Active sessions of the current user.", Response: dto.SessionList{}, Status: http.StatusOK, Handler: (*Handlers).sessions},
		{Method: http.MethodDelete, Pattern: "/auth/sessions/{id}", OperationID: "auth.sessions.revoke", Tag: authSelfTag,
			Access: Authenticated, SelfService: true, NoIdempotency: true, Entity: "session",
			Summary: "Revoke one of the current user's sessions.", Request: dto.IDPath{}, Status: http.StatusNoContent,
			Handler: (*Handlers).revokeSession},
		{Method: http.MethodGet, Pattern: "/profile", OperationID: "profile.get", Tag: profileTag, Access: Authenticated,
			Summary: "The current user's profile.", Response: dto.Me{}, Status: http.StatusOK, Handler: (*Handlers).me},
		{Method: http.MethodPatch, Pattern: "/profile", OperationID: "profile.update", Tag: profileTag, Access: RoleGated,
			Roles: webWriters, SelfService: true, Entity: "user",
			Summary: "Update the current user's name, e-mail, phone, locale and UI preferences.",
			Request: dto.ProfileUpdateRequest{}, Response: dto.Me{}, Status: http.StatusOK, Handler: (*Handlers).updateProfile},
		{Method: http.MethodPost, Pattern: "/mobile/auth/login", OperationID: "mobile.login", Tag: mobileTag, Access: Public,
			Summary: "Mobile sign-in; returns bearer and refresh tokens.", AuthLimit: AuthLimitByEmail,
			Request: dto.MobileLoginRequest{}, Response: dto.MobileTokens{}, Status: http.StatusOK, Handler: (*Handlers).mobileLogin},
		{Method: http.MethodPost, Pattern: "/mobile/auth/refresh", OperationID: "mobile.refresh", Tag: mobileTag, Access: Public,
			Summary: "Rotate mobile tokens.", SelfService: true,
			Request: dto.MobileRefreshRequest{}, Response: dto.MobileTokens{}, Status: http.StatusOK, Handler: (*Handlers).mobileRefresh},
		{Method: http.MethodGet, Pattern: "/mobile/auth/me", OperationID: "mobile.me", Tag: mobileTag, Access: Authenticated,
			Summary: "The current principal for the mobile app.", Response: dto.Me{}, Status: http.StatusOK, Handler: (*Handlers).me},
	}
}

func (h *Handlers) clientIP(r *http.Request) *netip.Addr {
	ip, err := netip.ParseAddr(h.ClientIP(r))
	if err != nil {
		return nil
	}
	return &ip
}

func meOf(p authsvc.Principal) dto.Me {
	perms := auth.PermissionsFor(p.User.Role)
	out := make([]dto.Permission, len(perms))
	for i, perm := range perms {
		out[i] = dto.Permission(perm)
	}
	return dto.Me{
		ID: p.User.ID, Name: p.User.Name, Email: p.User.Email, Phone: p.User.Phone, Role: dto.Role(p.User.Role),
		Locale: dto.Locale(p.User.Locale), UIPreferences: p.User.UIPreferences,
		Company: dto.CompanyRef{ID: p.Company.ID, Name: p.Company.Name}, Permissions: out, SessionID: p.SessionID,
	}
}

func principal(r *http.Request) authsvc.Principal {
	p, _ := mw.PrincipalFrom(r.Context())
	return p
}

func (h *Handlers) login(w http.ResponseWriter, r *http.Request) {
	var req dto.LoginRequest
	if err := kit.Bind(r, &req); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	issued, err := h.Auth.Login(r.Context(), authsvc.LoginInput{
		Email: req.Email, Password: req.Password, UserAgent: r.UserAgent(), IP: h.clientIP(r),
		Remember: req.RememberMe, Client: authsvc.ClientWeb,
	})
	if err != nil {
		kit.WriteError(w, r, err)
		return
	}
	h.setCookies(w, issued)
	kit.WriteJSON(w, http.StatusOK, meOf(issued.Principal))
}

func (h *Handlers) setCookies(w http.ResponseWriter, issued authsvc.Issued) {
	kit.SetAuthCookies(w, issued.AccessToken, issued.AccessExpires, issued.RefreshToken, issued.SessionExpires, issued.Remember, h.Clock.Now())
}

func (h *Handlers) refresh(w http.ResponseWriter, r *http.Request) {
	if err := kit.Bind(r, &struct{}{}); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	token := ""
	if c, err := r.Cookie(kit.RefreshCookie); err == nil {
		token = c.Value
	}
	issued, err := h.Auth.Refresh(r.Context(), authsvc.RefreshInput{
		Token: token, UserAgent: r.UserAgent(), IP: h.clientIP(r), Client: authsvc.ClientWeb,
	})
	if err != nil {
		// A concurrent tab already rotated: its fresh cookies must survive (R141).
		if !errors.Is(err, authsvc.ErrTokenRotated) {
			kit.ClearAuthCookies(w)
		}
		kit.WriteError(w, r, err)
		return
	}
	h.setCookies(w, issued)
	kit.NoContent(w)
}

func (h *Handlers) logout(w http.ResponseWriter, r *http.Request) {
	if err := h.Auth.Logout(r.Context(), principal(r)); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	kit.ClearAuthCookies(w)
	kit.NoContent(w)
}

func (h *Handlers) logoutAll(w http.ResponseWriter, r *http.Request) {
	if err := h.Auth.LogoutAll(r.Context(), principal(r)); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	kit.ClearAuthCookies(w)
	kit.NoContent(w)
}

func (h *Handlers) me(w http.ResponseWriter, r *http.Request) {
	if err := kit.Bind(r, &struct{}{}); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	kit.WriteJSON(w, http.StatusOK, meOf(principal(r)))
}

func (h *Handlers) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var req dto.ForgotPasswordRequest
	if err := kit.Bind(r, &req); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	if err := h.Auth.ForgotPassword(r.Context(), req.Email); err != nil {
		h.Log.WarnContext(r.Context(), "api: forgot password failed", "error", err.Error())
	}
	kit.WriteJSON(w, http.StatusAccepted, dto.Empty{})
}

func (h *Handlers) resetPassword(w http.ResponseWriter, r *http.Request) {
	var req dto.ResetPasswordRequest
	if err := kit.Bind(r, &req); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	if err := h.Auth.ResetPassword(r.Context(), req.Token, req.Password); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	kit.NoContent(w)
}

func (h *Handlers) changePassword(w http.ResponseWriter, r *http.Request) {
	var req dto.ChangePasswordRequest
	if err := kit.Bind(r, &req); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	if err := h.Auth.ChangePassword(r.Context(), principal(r), req.CurrentPassword, req.NewPassword); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	kit.NoContent(w)
}

func (h *Handlers) sessions(w http.ResponseWriter, r *http.Request) {
	if err := kit.Bind(r, &struct{}{}); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	list, err := h.Auth.Sessions(r.Context(), principal(r))
	if err != nil {
		kit.WriteError(w, r, err)
		return
	}
	out := dto.SessionList{Items: make([]dto.Session, 0, len(list))}
	for _, s := range list {
		out.Items = append(out.Items, dto.Session{
			ID: s.ID, Client: string(s.Client), UserAgent: s.UserAgent, IP: s.IP, CreatedAt: dto.T(s.CreatedAt),
			LastUsedAt: dto.TP(s.LastUsedAt), ExpiresAt: dto.T(s.ExpiresAt), Current: s.Current,
		})
	}
	kit.WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) revokeSession(w http.ResponseWriter, r *http.Request) {
	var req dto.IDPath
	if err := kit.Bind(r, &req); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	if err := h.Auth.RevokeSession(r.Context(), principal(r), req.ID); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	kit.NoContent(w)
}

func (h *Handlers) updateProfile(w http.ResponseWriter, r *http.Request) {
	var req dto.ProfileUpdateRequest
	if err := kit.Bind(r, &req); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	p := principal(r)
	var locale *string
	if req.Locale != nil {
		l := string(*req.Locale)
		locale = &l
	}
	user, err := h.Auth.UpdateProfile(r.Context(), p, authsvc.ProfileInput{
		Name: req.Name, Email: req.Email, Phone: req.Phone, Locale: locale, UIPreferences: req.UIPreferences,
	})
	if err != nil {
		kit.WriteError(w, r, err)
		return
	}
	p.User = user
	kit.WriteJSON(w, http.StatusOK, meOf(p))
}

func (h *Handlers) mobileTokens(issued authsvc.Issued) dto.MobileTokens {
	return dto.MobileTokens{
		AccessToken: issued.AccessToken, RefreshToken: issued.RefreshToken, TokenType: "Bearer",
		ExpiresIn: int64(issued.AccessExpires.Sub(h.Clock.Now()) / time.Second), User: meOf(issued.Principal),
	}
}

func (h *Handlers) mobileLogin(w http.ResponseWriter, r *http.Request) {
	var req dto.MobileLoginRequest
	if err := kit.Bind(r, &req); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	issued, err := h.Auth.Login(r.Context(), authsvc.LoginInput{
		Email: req.Email, Password: req.Password, UserAgent: r.UserAgent(), IP: h.clientIP(r), Client: authsvc.ClientMobile,
	})
	if err != nil {
		kit.WriteError(w, r, err)
		return
	}
	kit.WriteJSON(w, http.StatusOK, h.mobileTokens(issued))
}

func (h *Handlers) mobileRefresh(w http.ResponseWriter, r *http.Request) {
	var req dto.MobileRefreshRequest
	if err := kit.Bind(r, &req); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	issued, err := h.Auth.Refresh(r.Context(), authsvc.RefreshInput{
		Token: req.RefreshToken, UserAgent: r.UserAgent(), IP: h.clientIP(r), Client: authsvc.ClientMobile,
	})
	if err != nil {
		kit.WriteError(w, r, err)
		return
	}
	kit.WriteJSON(w, http.StatusOK, h.mobileTokens(issued))
}
