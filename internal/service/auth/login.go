package auth

import (
	"context"
	"errors"
	"net/netip"
	"strings"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// LoginInput is one login attempt.
type LoginInput struct {
	Email, Password, UserAgent string
	IP                         *netip.Addr
	Remember                   bool
	Client                     Client
}

// Login verifies credentials and opens a device-bound session (R141, R142, R147).
func (s *Service) Login(ctx context.Context, in LoginInput) (Issued, error) {
	user, err := s.d.AdminAuth.UserByEmail(ctx, strings.TrimSpace(in.Email))
	if errors.Is(err, store.ErrNotFound) {
		s.d.Hasher.DummyVerify(in.Password)
		return Issued{}, ErrInvalidCredentials
	}
	if err != nil {
		return Issued{}, err
	}
	ok, rehash := s.d.Hasher.Verify(user.PasswordHash, in.Password)
	if !ok || !user.IsActive {
		return Issued{}, ErrInvalidCredentials
	}
	sc := accountScope(user.CompanyID)
	now := s.d.Clock.Now()
	if rehash {
		if hash, err := s.d.Hasher.Hash(in.Password); err == nil {
			if err := s.d.Users.SetPassword(ctx, sc, user.ID, hash, now); err != nil {
				s.warn(ctx, "authsvc: legacy rehash failed", err)
			}
		}
	}
	company, err := s.d.Companies.Get(ctx, sc, user.CompanyID)
	if err != nil {
		return Issued{}, err
	}
	issued, err := s.newSession(ctx, user, company, in.Client, in.Remember, in.UserAgent, in.IP)
	if err != nil {
		return Issued{}, err
	}
	if err := s.d.Users.RecordLogin(ctx, sc, user.ID, now); err != nil {
		s.warn(ctx, "authsvc: record login failed", err)
	}
	companyID, userID := user.CompanyID, user.ID
	if _, err := s.d.Audit.Append(ctx, sc, model.AuditEntry{
		CompanyID: &companyID, UserID: &userID, Action: "auth.login", EntityType: "session",
		EntityID: &issued.Principal.SessionID, IP: in.IP, CreatedAt: now,
	}); err != nil {
		s.warn(ctx, "authsvc: audit login failed", err)
	}
	return issued, nil
}

// RefreshInput is one refresh request.
type RefreshInput struct {
	Token, UserAgent string
	IP               *netip.Addr
	Client           Client
}

// Refresh rotates a refresh token (R141) after checking the device (R142).
func (s *Service) Refresh(ctx context.Context, in RefreshInput) (Issued, error) {
	if in.Token == "" {
		return Issued{}, ErrSessionRevoked
	}
	sess, user, err := s.d.AdminAuth.SessionByRefreshTokenHash(ctx, auth.HashRefreshToken(in.Token))
	if errors.Is(err, store.ErrNotFound) {
		return Issued{}, ErrSessionRevoked
	}
	if err != nil {
		return Issued{}, err
	}
	sc := accountScope(user.CompanyID)
	now := s.d.Clock.Now()
	if sess.Client != string(in.Client) {
		return Issued{}, ErrSessionRevoked
	}
	if sess.RevokedAt != nil {
		if sess.RevokedReason != nil && *sess.RevokedReason == "rotated" {
			if now.Sub(*sess.RevokedAt) <= rotationGrace {
				return Issued{}, ErrTokenRotated
			}
			if _, err := s.d.Sessions.RevokeAllForUserWithReason(ctx, sc, user.ID, "reuse_detected", nil, now); err != nil {
				return Issued{}, err
			}
		}
		return Issued{}, ErrSessionRevoked
	}
	if !now.Before(sess.ExpiresAt) || !user.IsActive {
		return Issued{}, ErrSessionRevoked
	}
	if !s.sameDevice(sess, in.UserAgent) {
		if err := s.d.Sessions.RevokeWithReason(ctx, sc, sess.ID, "device_mismatch", now); err != nil {
			return Issued{}, err
		}
		return Issued{}, ErrDeviceMismatch
	}
	company, err := s.d.Companies.Get(ctx, sc, user.CompanyID)
	if errors.Is(err, store.ErrNotFound) {
		return Issued{}, ErrSessionRevoked
	}
	if err != nil {
		return Issued{}, err
	}
	plain, hash, err := auth.NewRefreshToken()
	if err != nil {
		return Issued{}, err
	}
	next, err := s.d.Sessions.Rotate(ctx, sc, sess.ID, model.Session{
		RefreshTokenHash: hash, DeviceFingerprint: sess.DeviceFingerprint, UserAgent: optional(in.UserAgent),
		IP: in.IP, ExpiresAt: sess.ExpiresAt, Client: sess.Client, Remember: sess.Remember, CreatedAt: now,
	}, now)
	if errors.Is(err, store.ErrConflict) {
		return Issued{}, ErrTokenRotated
	}
	if err != nil {
		return Issued{}, err
	}
	return s.issue(user, company, next, plain)
}

func (s *Service) sameDevice(sess model.Session, userAgent string) bool {
	return sess.DeviceFingerprint != nil && *sess.DeviceFingerprint == auth.Fingerprint(s.d.FingerprintSecret, userAgent)
}

// Authenticate verifies an access token and reloads its session and user (R140).
func (s *Service) Authenticate(ctx context.Context, accessToken, userAgent string) (Principal, error) {
	claims, err := s.d.Tokens.Parse(accessToken)
	switch {
	case errors.Is(err, auth.ErrTokenExpired):
		return Principal{}, ErrTokenExpired
	case err != nil:
		return Principal{}, ErrTokenInvalid
	}
	sc := accountScope(claims.CompanyID)
	now := s.d.Clock.Now()
	sess, err := s.d.Sessions.Get(ctx, sc, claims.SessionID)
	if errors.Is(err, store.ErrNotFound) {
		return Principal{}, ErrSessionRevoked
	}
	if err != nil {
		return Principal{}, err
	}
	if sess.UserID != claims.UserID || sess.Client != claims.Audience {
		return Principal{}, ErrTokenInvalid
	}
	if sess.RevokedAt != nil || !now.Before(sess.ExpiresAt) {
		return Principal{}, ErrSessionRevoked
	}
	if !s.sameDevice(sess, userAgent) {
		if err := s.d.Sessions.RevokeWithReason(ctx, sc, sess.ID, "device_mismatch", now); err != nil {
			return Principal{}, err
		}
		return Principal{}, ErrDeviceMismatch
	}
	user, err := s.d.Users.Get(ctx, sc, claims.UserID)
	if errors.Is(err, store.ErrNotFound) {
		return Principal{}, ErrSessionRevoked
	}
	if err != nil {
		return Principal{}, err
	}
	if !user.IsActive {
		return Principal{}, ErrSessionRevoked
	}
	company, err := s.d.Companies.Get(ctx, sc, user.CompanyID)
	if errors.Is(err, store.ErrNotFound) {
		return Principal{}, ErrSessionRevoked
	}
	if err != nil {
		return Principal{}, err
	}
	if err := s.d.Sessions.Touch(ctx, sc, sess.ID, now); err != nil {
		s.warn(ctx, "authsvc: touch session failed", err)
	}
	return Principal{User: user, Company: company, SessionID: sess.ID, Client: Client(sess.Client)}, nil
}
