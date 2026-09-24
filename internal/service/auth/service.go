// Package auth is the authentication service: logins, refresh rotation,
// device binding, password changes and resets, and per-request scope
// resolution (05 §2, R139–R149, R158).
package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/mail"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Error values the API maps to its envelope.
var (
	ErrInvalidCredentials = perr.New("invalid_credentials", 401, "errors.auth.invalidCredentials")
	ErrTokenExpired       = perr.New("token_expired", 401, "errors.auth.tokenExpired")
	ErrTokenInvalid       = perr.New("token_invalid", 401, "errors.auth.tokenInvalid")
	ErrTokenRotated       = perr.New("token_rotated", 401, "errors.auth.tokenRotated")
	ErrSessionRevoked     = perr.New("session_revoked", 401, "errors.auth.sessionRevoked")
	ErrDeviceMismatch     = perr.New("device_mismatch", 401, "errors.auth.deviceMismatch")
	ErrResetTokenInvalid  = perr.New("reset_token_invalid", 400, "errors.auth.resetTokenInvalid")
)

// Client is the kind of client a session belongs to.
type Client string

// The two clients (R141).
const (
	ClientWeb    Client = "web"
	ClientMobile Client = "mobile"
)

const (
	rotationGrace   = 30 * time.Second
	resetTokenTTL   = time.Hour
	sessionPageSize = 200
	sendTimeout     = 30 * time.Second
)

// Deps is everything Service needs.
type Deps struct {
	Users     store.UserRepository
	Sessions  store.SessionRepository
	Resets    store.PasswordResetRepository
	Companies store.CompanyRepository
	Buildings store.BuildingRepository
	SMTP      store.SMTPRepository
	Ops       store.OpsRepository
	Audit     store.AuditRepository
	AdminAuth store.AdminAuthRepository

	Mail              mail.Sender
	Hasher            auth.Hasher
	Tokens            auth.Tokens
	FingerprintSecret []byte

	RefreshTTL, RememberTTL time.Duration
	HistorySize             int
	PublicURL               string
	Clock                   clock.Clock
	Log                     *slog.Logger
	// Async runs work that must not change a response's timing (reset e-mail);
	// nil means a goroutine.
	Async func(func())
}

// Service implements authentication.
type Service struct{ d Deps }

// New validates deps.
func New(d Deps) (*Service, error) {
	switch {
	case d.Users == nil, d.Sessions == nil, d.Resets == nil, d.Companies == nil, d.Buildings == nil,
		d.SMTP == nil, d.Ops == nil, d.Audit == nil, d.AdminAuth == nil, d.Mail == nil, d.Clock == nil, d.Log == nil:
		return nil, errors.New("authsvc: missing dependency")
	case len(d.Tokens.Key) == 0 || d.Tokens.TTL <= 0 || len(d.FingerprintSecret) == 0 || len(d.Hasher.Pepper) == 0:
		return nil, errors.New("authsvc: missing key material")
	case d.RefreshTTL <= 0 || d.RememberTTL < d.RefreshTTL:
		return nil, errors.New("authsvc: invalid session lifetimes")
	}
	if d.Async == nil {
		d.Async = func(f func()) { go f() }
	}
	if d.Tokens.Now == nil {
		d.Tokens.Now = d.Clock.Now
	}
	return &Service{d: d}, nil
}

// Principal is the authenticated caller.
type Principal struct {
	User      model.User
	Company   model.Company
	SessionID uuid.UUID
	Client    Client
}

// Issued is the result of a login or refresh.
type Issued struct {
	AccessToken    string
	AccessExpires  time.Time
	RefreshToken   string
	SessionExpires time.Time
	Remember       bool
	Principal      Principal
}

// accountScope is the scope for reading and writing the caller's own account
// rows (users, sessions); those tables never narrow by building.
func accountScope(companyID uuid.UUID) store.Scope { return store.SystemScope(companyID) }

func (s *Service) newSession(ctx context.Context, user model.User, company model.Company, client Client, remember bool, ua string, ip *netip.Addr) (Issued, error) {
	now := s.d.Clock.Now()
	ttl := s.d.RefreshTTL
	if client == ClientMobile {
		remember = true
	}
	if remember {
		ttl = s.d.RememberTTL
	}
	plain, hash, err := auth.NewRefreshToken()
	if err != nil {
		return Issued{}, err
	}
	fp := auth.Fingerprint(s.d.FingerprintSecret, ua)
	sess, err := s.d.Sessions.Create(ctx, accountScope(user.CompanyID), model.Session{
		UserID: user.ID, RefreshTokenHash: hash, DeviceFingerprint: &fp, UserAgent: optional(ua), IP: ip,
		ExpiresAt: now.Add(ttl), CreatedAt: now, Client: string(client), Remember: remember,
	})
	if err != nil {
		return Issued{}, err
	}
	return s.issue(user, company, sess, plain)
}

func (s *Service) issue(user model.User, company model.Company, sess model.Session, refreshPlain string) (Issued, error) {
	token, exp, err := s.d.Tokens.Issue(user.ID, sess.ID, user.CompanyID, sess.Client)
	if err != nil {
		return Issued{}, err
	}
	return Issued{
		AccessToken: token, AccessExpires: exp, RefreshToken: refreshPlain, SessionExpires: sess.ExpiresAt,
		Remember:  sess.Remember,
		Principal: Principal{User: user, Company: company, SessionID: sess.ID, Client: Client(sess.Client)},
	}, nil
}

func optional(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func (s *Service) warn(ctx context.Context, msg string, err error) {
	s.d.Log.WarnContext(ctx, msg, slog.String("error", err.Error()))
}
