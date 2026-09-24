//go:build integration

package auth_test

import (
	"context"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/mail"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	authsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

const (
	uaA      = "Mozilla/5.0 (X11; Linux x86_64) Chrome/140"
	uaB      = "Mozilla/5.0 (Macintosh) Safari/18"
	password = "Guvenli!Sifre-42"
)

var fpSecret = []byte("device-fingerprint-secret-0123456789")

type sentMail struct {
	mu   sync.Mutex
	msgs []mail.Message
	fail error
}

func (m *sentMail) Send(_ context.Context, _ model.SMTPSettings, _ []byte, msg mail.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return m.fail
	}
	m.msgs = append(m.msgs, msg)
	return nil
}

type env struct {
	svc    *authsvc.Service
	pool   *pgxpool.Pool
	tenant testfixtures.Tenant
	other  testfixtures.Tenant
	clock  *clock.Fake
	mail   *sentMail
	hasher auth.Hasher
	smtp   *postgres.SMTPRepository
}

func newEnv(t *testing.T) *env { return newEnvCost(t, bcrypt.MinCost) }

func newEnvCost(t *testing.T, cost int) *env {
	t.Helper()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	e := &env{
		pool: pool, tenant: testfixtures.NewTenant(t, ctx, pool, 1), other: testfixtures.NewTenant(t, ctx, pool, 2),
		clock: clock.NewFake(time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)), mail: &sentMail{},
		hasher: auth.Hasher{Pepper: []byte("pepper-0123456789abcdef-0123456789"), Cost: cost},
	}
	cipher, err := crypto.NewCipher([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)
	e.smtp = postgres.NewSMTPRepository(pool, cipher)
	users := postgres.NewUserRepository(pool)
	for _, u := range []testfixtures.Tenant{e.tenant, e.other} {
		for _, user := range u.Users {
			hash, err := e.hasher.Hash(password)
			require.NoError(t, err)
			require.NoError(t, users.SetPassword(ctx, u.AdminScope, user.ID, hash, e.clock.Now().Add(-time.Hour)))
		}
	}
	e.svc, err = authsvc.New(authsvc.Deps{
		Users: users, Sessions: postgres.NewSessionRepository(pool), Resets: postgres.NewPasswordResetRepository(pool),
		Companies: postgres.NewCompanyRepository(pool), Buildings: postgres.NewBuildingRepository(pool),
		SMTP: e.smtp, Ops: postgres.NewOpsRepository(pool), Audit: postgres.NewAuditRepository(pool),
		AdminAuth: admin.NewAuthRepository(pool), Mail: e.mail, Hasher: e.hasher,
		Tokens:            auth.Tokens{Key: []byte("jwt-signing-key-0123456789abcdef-0123"), TTL: 15 * time.Minute},
		FingerprintSecret: fpSecret, RefreshTTL: 24 * time.Hour, RememberTTL: 720 * time.Hour, HistorySize: 5,
		PublicURL: "https://app.example/", Clock: e.clock, Log: testfixtures.DiscardLogger(),
		Async: func(f func()) { f() },
	})
	require.NoError(t, err)
	return e
}

func (e *env) user(role model.UserRole) model.User { return e.tenant.Users[role] }

func (e *env) login(t *testing.T, role model.UserRole, client authsvc.Client, remember bool, ua string) authsvc.Issued {
	t.Helper()
	ip := netip.MustParseAddr("10.1.2.3")
	issued, err := e.svc.Login(context.Background(), authsvc.LoginInput{
		Email: e.user(role).Email, Password: password, UserAgent: ua, IP: &ip, Remember: remember, Client: client,
	})
	require.NoError(t, err)
	return issued
}

func (e *env) session(t *testing.T, id uuid.UUID) model.Session {
	t.Helper()
	sess, err := postgres.NewSessionRepository(e.pool).Get(context.Background(), e.tenant.AdminScope, id)
	require.NoError(t, err)
	return sess
}

func TestLoginIssuesSessionBoundToDevice(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	now := e.clock.Now()

	web := e.login(t, model.UserRoleCompanyAdmin, authsvc.ClientWeb, false, uaA)
	sess := e.session(t, web.Principal.SessionID)
	require.Equal(t, "web", sess.Client)
	require.False(t, sess.Remember)
	require.Equal(t, auth.Fingerprint(fpSecret, uaA), *sess.DeviceFingerprint)
	require.True(t, sess.ExpiresAt.Equal(now.Add(24*time.Hour)))
	require.Equal(t, auth.HashRefreshToken(web.RefreshToken), sess.RefreshTokenHash)
	require.Equal(t, model.UserRoleCompanyAdmin, web.Principal.User.Role)
	require.Equal(t, e.tenant.Company.ID, web.Principal.Company.ID)

	remember := e.login(t, model.UserRoleCompanyAdmin, authsvc.ClientWeb, true, uaA)
	require.True(t, remember.SessionExpires.Equal(now.Add(720*time.Hour)))
	mobile := e.login(t, model.UserRoleCompanyAdmin, authsvc.ClientMobile, false, uaA)
	require.True(t, mobile.Remember)
	require.True(t, mobile.SessionExpires.Equal(now.Add(720*time.Hour)))

	p, err := e.svc.Authenticate(context.Background(), mobile.AccessToken, uaA)
	require.NoError(t, err)
	require.Equal(t, authsvc.ClientMobile, p.Client)

	entries, err := postgres.NewAuditRepository(e.pool).List(context.Background(), e.tenant.AdminScope, store.AuditFilter{})
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	require.Equal(t, "auth.login", entries[0].Action)
}

func TestLoginFailuresAreIndistinguishable(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	_, errUnknown := e.svc.Login(ctx, authsvc.LoginInput{Email: "nobody@example.invalid", Password: password, UserAgent: uaA, Client: authsvc.ClientWeb})
	_, errWrong := e.svc.Login(ctx, authsvc.LoginInput{Email: e.user(model.UserRoleCompanyAdmin).Email, Password: "Yanlis!Sifre-42", UserAgent: uaA, Client: authsvc.ClientWeb})

	inactive := e.user(model.UserRoleBuildingAdmin)
	inactive.IsActive = false
	inactive.UpdatedAt = e.clock.Now()
	_, err := postgres.NewUserRepository(e.pool).Update(ctx, e.tenant.AdminScope, inactive)
	require.NoError(t, err)
	_, errInactive := e.svc.Login(ctx, authsvc.LoginInput{Email: inactive.Email, Password: password, UserAgent: uaA, Client: authsvc.ClientWeb})

	for _, got := range []error{errUnknown, errWrong, errInactive} {
		require.Same(t, authsvc.ErrInvalidCredentials, got)
	}
	_, errCase := e.svc.Login(ctx, authsvc.LoginInput{Email: "  " + strings.ToUpper(e.user(model.UserRoleCompanyAdmin).Email), Password: password, UserAgent: uaA, Client: authsvc.ClientWeb})
	require.NoError(t, errCase, "e-mail lookup is case-insensitive and trimmed")
}

func TestLoginRehashesLegacyHash(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	user := e.user(model.UserRoleCompanyAdmin)
	legacy, err := bcrypt.GenerateFromPassword([]byte("Eski!Parola99x"), bcrypt.MinCost)
	require.NoError(t, err)
	_, err = e.pool.Exec(ctx, `update users set password_hash = $1 where id = $2`, "legacy$"+string(legacy), user.ID)
	require.NoError(t, err)

	_, err = e.svc.Login(ctx, authsvc.LoginInput{Email: user.Email, Password: "Eski!Parola99x", UserAgent: uaA, Client: authsvc.ClientWeb})
	require.NoError(t, err)
	stored, err := postgres.NewUserRepository(e.pool).Get(ctx, e.tenant.AdminScope, user.ID)
	require.NoError(t, err)
	require.False(t, strings.HasPrefix(stored.PasswordHash, "legacy$"))
	ok, rehash := e.hasher.Verify(stored.PasswordHash, "Eski!Parola99x")
	require.True(t, ok)
	require.False(t, rehash)
}

func TestRefreshRotatesWithAbsoluteExpiry(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	first := e.login(t, model.UserRoleCompanyAdmin, authsvc.ClientWeb, false, uaA)
	e.clock.Advance(time.Hour)

	next, err := e.svc.Refresh(ctx, authsvc.RefreshInput{Token: first.RefreshToken, UserAgent: uaA, Client: authsvc.ClientWeb})
	require.NoError(t, err)
	require.NotEqual(t, first.RefreshToken, next.RefreshToken)
	require.NotEqual(t, first.Principal.SessionID, next.Principal.SessionID)
	require.True(t, next.SessionExpires.Equal(first.SessionExpires), "lifetime is absolute from login")
	old := e.session(t, first.Principal.SessionID)
	require.Equal(t, "rotated", *old.RevokedReason)

	_, err = e.svc.Refresh(ctx, authsvc.RefreshInput{Token: next.RefreshToken, UserAgent: uaA, Client: authsvc.ClientMobile})
	require.Same(t, authsvc.ErrSessionRevoked, err, "a web session cannot be refreshed as mobile")

	e.clock.Advance(24 * time.Hour)
	_, err = e.svc.Refresh(ctx, authsvc.RefreshInput{Token: next.RefreshToken, UserAgent: uaA, Client: authsvc.ClientWeb})
	require.Same(t, authsvc.ErrSessionRevoked, err, "expired")
}

func TestRefreshGraceThenReuseDetection(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	first := e.login(t, model.UserRoleCompanyAdmin, authsvc.ClientWeb, false, uaA)
	other := e.login(t, model.UserRoleCompanyAdmin, authsvc.ClientWeb, false, uaB)
	next, err := e.svc.Refresh(ctx, authsvc.RefreshInput{Token: first.RefreshToken, UserAgent: uaA, Client: authsvc.ClientWeb})
	require.NoError(t, err)

	e.clock.Advance(10 * time.Second)
	_, err = e.svc.Refresh(ctx, authsvc.RefreshInput{Token: first.RefreshToken, UserAgent: uaA, Client: authsvc.ClientWeb})
	require.Same(t, authsvc.ErrTokenRotated, err)
	require.Nil(t, e.session(t, next.Principal.SessionID).RevokedAt, "inside the grace window nothing is revoked")

	e.clock.Advance(21 * time.Second)
	_, err = e.svc.Refresh(ctx, authsvc.RefreshInput{Token: first.RefreshToken, UserAgent: uaA, Client: authsvc.ClientWeb})
	require.Same(t, authsvc.ErrSessionRevoked, err)
	require.Equal(t, "reuse_detected", *e.session(t, next.Principal.SessionID).RevokedReason)
	require.Equal(t, "reuse_detected", *e.session(t, other.Principal.SessionID).RevokedReason)
}

func TestRefreshDeviceMismatchRevokes(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	first := e.login(t, model.UserRoleCompanyAdmin, authsvc.ClientWeb, false, uaA)
	_, err := e.svc.Refresh(ctx, authsvc.RefreshInput{Token: first.RefreshToken, UserAgent: uaB, Client: authsvc.ClientWeb})
	require.Same(t, authsvc.ErrDeviceMismatch, err)
	require.Equal(t, "device_mismatch", *e.session(t, first.Principal.SessionID).RevokedReason)
	_, err = e.svc.Refresh(ctx, authsvc.RefreshInput{Token: first.RefreshToken, UserAgent: uaA, Client: authsvc.ClientWeb})
	require.Same(t, authsvc.ErrSessionRevoked, err)
}

func TestAuthenticateDeviceMismatchRevokes(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	first := e.login(t, model.UserRoleCompanyAdmin, authsvc.ClientWeb, false, uaA)
	_, err := e.svc.Authenticate(ctx, first.AccessToken, uaB)
	require.Same(t, authsvc.ErrDeviceMismatch, err)
	require.Equal(t, "device_mismatch", *e.session(t, first.Principal.SessionID).RevokedReason)
	_, err = e.svc.Authenticate(ctx, first.AccessToken, uaA)
	require.Same(t, authsvc.ErrSessionRevoked, err)
}

func TestAuthenticateUsesLiveRoleAndRejectsRevoked(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	users := postgres.NewUserRepository(e.pool)
	first := e.login(t, model.UserRoleCompanyAdmin, authsvc.ClientWeb, false, uaA)

	u := e.user(model.UserRoleCompanyAdmin)
	u.Role = model.UserRoleCompanyReadonlyAdmin
	u.UpdatedAt = e.clock.Now()
	_, err := users.Update(ctx, e.tenant.AdminScope, u)
	require.NoError(t, err)
	p, err := e.svc.Authenticate(ctx, first.AccessToken, uaA)
	require.NoError(t, err)
	require.Equal(t, model.UserRoleCompanyReadonlyAdmin, p.User.Role, "role is read from the database, not the token")
	require.NotNil(t, e.session(t, first.Principal.SessionID).LastUsedAt)

	e.clock.Advance(16 * time.Minute)
	_, err = e.svc.Authenticate(ctx, first.AccessToken, uaA)
	require.Same(t, authsvc.ErrTokenExpired, err)
	e.clock.Advance(-16 * time.Minute)

	_, err = e.svc.Authenticate(ctx, first.AccessToken+"x", uaA)
	require.Same(t, authsvc.ErrTokenInvalid, err)

	u.IsActive = false
	_, err = users.Update(ctx, e.tenant.AdminScope, u)
	require.NoError(t, err)
	_, err = e.svc.Authenticate(ctx, first.AccessToken, uaA)
	require.Same(t, authsvc.ErrSessionRevoked, err)

	second := e.login(t, model.UserRoleBuildingAdmin, authsvc.ClientWeb, false, uaA)
	require.NoError(t, e.svc.Logout(ctx, second.Principal))
	_, err = e.svc.Authenticate(ctx, second.AccessToken, uaA)
	require.Same(t, authsvc.ErrSessionRevoked, err)
}

func TestSessionsListAndRevokeOwnOnly(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	a := e.login(t, model.UserRoleCompanyAdmin, authsvc.ClientWeb, false, uaA)
	b := e.login(t, model.UserRoleCompanyAdmin, authsvc.ClientMobile, false, uaB)
	someoneElse := e.login(t, model.UserRoleBuildingAdmin, authsvc.ClientWeb, false, uaA)

	list, err := e.svc.Sessions(ctx, a.Principal)
	require.NoError(t, err)
	require.Len(t, list, 2)
	current := 0
	for _, s := range list {
		if s.Current {
			current++
			require.Equal(t, a.Principal.SessionID, s.ID)
		}
	}
	require.Equal(t, 1, current)

	require.ErrorIs(t, e.svc.RevokeSession(ctx, a.Principal, someoneElse.Principal.SessionID), perr.NotFound)
	require.NoError(t, e.svc.RevokeSession(ctx, a.Principal, b.Principal.SessionID))
	list, err = e.svc.Sessions(ctx, a.Principal)
	require.NoError(t, err)
	require.Len(t, list, 1)

	require.NoError(t, e.svc.LogoutAll(ctx, a.Principal))
	_, err = e.svc.Authenticate(ctx, a.AccessToken, uaA)
	require.Same(t, authsvc.ErrSessionRevoked, err)
}

func TestChangePasswordRevokesOthersKeepsCurrent(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	current := e.login(t, model.UserRoleCompanyAdmin, authsvc.ClientWeb, false, uaA)
	other := e.login(t, model.UserRoleCompanyAdmin, authsvc.ClientMobile, false, uaB)

	err := e.svc.ChangePassword(ctx, current.Principal, "wrong", "Yepyeni!Sifre-77")
	require.ErrorIs(t, err, perr.Validation)
	require.Equal(t, map[string]any{"current_password": []string{"invalid"}}, asErr(t, err).Params)

	err = e.svc.ChangePassword(ctx, current.Principal, password, "kisa")
	require.ErrorIs(t, err, perr.Validation)
	require.Contains(t, asErr(t, err).Params["new_password"], "password_too_short")

	require.NoError(t, e.svc.ChangePassword(ctx, current.Principal, password, "Yepyeni!Sifre-77"))
	require.Nil(t, e.session(t, current.Principal.SessionID).RevokedAt)
	require.Equal(t, "password_change", *e.session(t, other.Principal.SessionID).RevokedReason)
	_, err = e.svc.Login(ctx, authsvc.LoginInput{Email: e.user(model.UserRoleCompanyAdmin).Email, Password: "Yepyeni!Sifre-77", UserAgent: uaA, Client: authsvc.ClientWeb})
	require.NoError(t, err)

	demo := e.login(t, model.UserRoleDemo, authsvc.ClientWeb, false, uaA)
	require.ErrorIs(t, e.svc.ChangePassword(ctx, demo.Principal, password, "Yepyeni!Sifre-77"), perr.Forbidden)
}

func TestChangePasswordRejectsHistory(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	s := e.login(t, model.UserRoleCompanyAdmin, authsvc.ClientWeb, false, uaA)
	passwords := []string{password, "Birinci!Sifre-01", "Ikinci!Sifre-02", "Ucuncu!Sifre-03", "Dorduncu!Sifre-04", "Besinci!Sifre-05", "Altinci!Sifre-06"}
	for i := 1; i < len(passwords); i++ {
		e.clock.Advance(time.Minute)
		require.NoError(t, e.svc.ChangePassword(ctx, s.Principal, passwords[i-1], passwords[i]), passwords[i])
	}
	// current = Altinci; history (newest first, 5) = Besinci … Birinci; the original is 6 back.
	err := e.svc.ChangePassword(ctx, s.Principal, passwords[6], passwords[1])
	require.ErrorIs(t, err, perr.Validation)
	require.Equal(t, []string{"password_reused"}, asErr(t, err).Params["new_password"])
	require.NoError(t, e.svc.ChangePassword(ctx, s.Principal, passwords[6], password), "six changes back is allowed again")
}

func asErr(t *testing.T, err error) *perr.Error {
	t.Helper()
	var e *perr.Error
	require.ErrorAs(t, err, &e)
	return e
}

func (e *env) configureSMTP(t *testing.T) {
	t.Helper()
	_, err := e.smtp.Upsert(context.Background(), e.tenant.AdminScope, model.SMTPSettings{
		CompanyID: e.tenant.Company.ID, Host: "smtp.example", Port: 465, Secure: true, Username: "u", FromAddress: "no-reply@example.com",
	}, []byte("smtp-password"))
	require.NoError(t, err)
}

func TestForgotPasswordSendsOnlyForActiveUsers(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	e.configureSMTP(t)

	require.NoError(t, e.svc.ForgotPassword(ctx, "nobody@example.invalid"))
	require.NoError(t, e.svc.ForgotPassword(ctx, e.user(model.UserRoleDemo).Email))
	inactive := e.user(model.UserRoleBuildingReadonlyAdmin)
	inactive.IsActive = false
	_, err := postgres.NewUserRepository(e.pool).Update(ctx, e.tenant.AdminScope, inactive)
	require.NoError(t, err)
	require.NoError(t, e.svc.ForgotPassword(ctx, inactive.Email))
	require.Empty(t, e.mail.msgs)

	require.NoError(t, e.svc.ForgotPassword(ctx, e.user(model.UserRoleCompanyAdmin).Email))
	require.Len(t, e.mail.msgs, 1)
	msg := e.mail.msgs[0]
	require.Equal(t, []string{e.user(model.UserRoleCompanyAdmin).Email}, msg.To)
	require.Contains(t, msg.Text, "https://app.example/auth/reset-password?token=")
	require.Equal(t, "Şifre sıfırlama", msg.Subject)

	// Another tenant without SMTP settings: nothing sent, a scrubbed operational message recorded.
	require.NoError(t, e.svc.ForgotPassword(ctx, e.other.Users[model.UserRoleCompanyAdmin].Email))
	require.Len(t, e.mail.msgs, 1)
	msgs, err := postgres.NewOpsRepository(e.pool).ListMessages(ctx, e.other.AdminScope, store.MessageFilter{})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Equal(t, "error", msgs[0].Status)
	require.Equal(t, "auth", msgs[0].Category)
	require.NotContains(t, msgs[0].Message, "token")
	require.NotContains(t, msgs[0].Message, "reset-password")
}

func resetTokenFrom(t *testing.T, msg mail.Message) string {
	t.Helper()
	_, after, ok := strings.Cut(msg.Text, "token=")
	require.True(t, ok)
	return strings.Fields(after)[0]
}

func TestResetPasswordSingleUse(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	e.configureSMTP(t)
	s := e.login(t, model.UserRoleCompanyAdmin, authsvc.ClientWeb, false, uaA)
	require.NoError(t, e.svc.ForgotPassword(ctx, e.user(model.UserRoleCompanyAdmin).Email))
	token := resetTokenFrom(t, e.mail.msgs[0])

	err := e.svc.ResetPassword(ctx, token, "zayif")
	require.ErrorIs(t, err, perr.Validation)
	require.NoError(t, e.svc.ResetPassword(ctx, token, "Sifirlandi!Yeni-88"), "a policy failure does not burn the token")
	require.Equal(t, "password_change", *e.session(t, s.Principal.SessionID).RevokedReason)
	require.Same(t, authsvc.ErrResetTokenInvalid, e.svc.ResetPassword(ctx, token, "Baska!Sifre-99x"))
	require.Same(t, authsvc.ErrResetTokenInvalid, e.svc.ResetPassword(ctx, "unknown", "Baska!Sifre-99x"))

	require.NoError(t, e.svc.ForgotPassword(ctx, e.user(model.UserRoleCompanyAdmin).Email))
	late := resetTokenFrom(t, e.mail.msgs[1])
	e.clock.Advance(61 * time.Minute)
	require.Same(t, authsvc.ErrResetTokenInvalid, e.svc.ResetPassword(ctx, late, "Baska!Sifre-99x"))
}

func TestResolveScope(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	principal := func(role model.UserRole) authsvc.Principal {
		return authsvc.Principal{User: e.user(role), Company: e.tenant.Company}
	}
	own, otherCo := e.tenant.Company.ID, e.other.Company.ID
	unknown := uuid.New()

	sc, err := e.svc.ResolveScope(ctx, principal(model.UserRoleAdmin), nil)
	require.NoError(t, err)
	require.Equal(t, store.Scope{CompanyID: own, AllBuildings: true}, sc)
	sc, err = e.svc.ResolveScope(ctx, principal(model.UserRoleAdmin), &otherCo)
	require.NoError(t, err)
	require.Equal(t, store.Scope{CompanyID: otherCo, AllBuildings: true}, sc)
	_, err = e.svc.ResolveScope(ctx, principal(model.UserRoleAdmin), &unknown)
	require.ErrorIs(t, err, perr.NotFound)

	for _, role := range []model.UserRole{model.UserRoleCompanyAdmin, model.UserRoleCompanyReadonlyAdmin, model.UserRoleDemo} {
		_, err = e.svc.ResolveScope(ctx, principal(role), &otherCo)
		require.ErrorIs(t, err, perr.NotFound, role)
		sc, err = e.svc.ResolveScope(ctx, principal(role), &own)
		require.NoError(t, err)
		require.Equal(t, store.Scope{CompanyID: own, AllBuildings: true}, sc, role)
	}

	ba := e.user(model.UserRoleBuildingAdmin)
	br := e.user(model.UserRoleBuildingReadonlyAdmin)
	_, err = e.pool.Exec(ctx, `update buildings set responsible_user_id = $1 where id = $2`, ba.ID, e.tenant.Buildings[1].ID)
	require.NoError(t, err)
	sc, err = e.svc.ResolveScope(ctx, principal(model.UserRoleBuildingAdmin), nil)
	require.NoError(t, err)
	require.Equal(t, store.Scope{CompanyID: own, BuildingIDs: []uuid.UUID{e.tenant.Buildings[1].ID}}, sc)
	_, err = e.svc.ResolveScope(ctx, principal(model.UserRoleBuildingAdmin), &otherCo)
	require.ErrorIs(t, err, perr.NotFound)

	sc, err = e.svc.ResolveScope(ctx, authsvc.Principal{User: br}, nil)
	require.NoError(t, err)
	require.False(t, sc.AllBuildings)
	require.NotNil(t, sc.BuildingIDs, "no buildings is an empty grant, never nil")
	require.Empty(t, sc.BuildingIDs)
}

func TestLoginUnknownEmailSpendsBcryptTime(t *testing.T) {
	t.Parallel()
	e := newEnvCost(t, 10)
	ctx := context.Background()
	elapsed := func(email string) time.Duration {
		best := time.Hour
		for range 3 {
			start := time.Now()
			_, _ = e.svc.Login(ctx, authsvc.LoginInput{Email: email, Password: "Yanlis!Sifre-42", UserAgent: uaA, Client: authsvc.ClientWeb})
			best = min(best, time.Since(start))
		}
		return best
	}
	wrong := elapsed(e.user(model.UserRoleCompanyAdmin).Email)
	unknown := elapsed("nobody@example.invalid")
	require.Greater(t, unknown, wrong/2, "an unknown e-mail must cost a bcrypt comparison too (R147): unknown=%s wrong=%s", unknown, wrong)
}
