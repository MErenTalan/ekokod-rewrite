package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/mail"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

func validation(field string, codes []string) error {
	return perr.Validation.WithParams(map[string]any{field: codes})
}

func (s *Service) policy(ctx context.Context, user model.User, password string) ([]string, error) {
	sc := accountScope(user.CompanyID)
	history, err := s.d.Users.PasswordHistory(ctx, sc, user.ID, int32(s.d.HistorySize))
	if err != nil {
		return nil, err
	}
	company, err := s.d.Companies.Get(ctx, sc, user.CompanyID)
	if err != nil {
		return nil, err
	}
	hashes := make([]string, 0, len(history))
	for _, h := range history {
		hashes = append(hashes, h.PasswordHash)
	}
	return auth.CheckPolicy(s.d.Hasher, auth.PolicyInput{
		Password: password, Name: user.Name, Email: user.Email, CompanyName: company.Name,
		CurrentHash: user.PasswordHash, History: hashes,
	}), nil
}

// ChangePassword verifies the current password, applies R146, and revokes
// every other session (R149).
func (s *Service) ChangePassword(ctx context.Context, p Principal, current, next string) error {
	if p.User.Role == model.UserRoleDemo {
		return perr.Forbidden
	}
	sc := accountScope(p.User.CompanyID)
	user, err := s.d.Users.Get(ctx, sc, p.User.ID)
	if err != nil {
		return err
	}
	if ok, _ := s.d.Hasher.Verify(user.PasswordHash, current); !ok {
		return validation("current_password", []string{"invalid"})
	}
	codes, err := s.policy(ctx, user, next)
	if err != nil {
		return err
	}
	if len(codes) > 0 {
		return validation("new_password", codes)
	}
	hash, err := s.d.Hasher.Hash(next)
	if err != nil {
		return err
	}
	now := s.d.Clock.Now()
	if err := s.d.Users.SetPassword(ctx, sc, user.ID, hash, now); err != nil {
		return err
	}
	_, err = s.d.Sessions.RevokeAllForUserWithReason(ctx, sc, user.ID, "password_change", &p.SessionID, now)
	return err
}

// ForgotPassword issues a reset link for an active non-demo user. It never
// reveals whether the address exists: every path returns nil and the e-mail
// is sent off the request path (R148).
func (s *Service) ForgotPassword(ctx context.Context, email string) error {
	user, err := s.d.AdminAuth.UserByEmail(ctx, strings.TrimSpace(email))
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !user.IsActive || user.Role == model.UserRoleDemo {
		return nil
	}
	sc := accountScope(user.CompanyID)
	now := s.d.Clock.Now()
	plain, hash, err := auth.NewRefreshToken()
	if err != nil {
		return err
	}
	if _, err := s.d.Resets.Create(ctx, sc, model.PasswordReset{
		UserID: user.ID, TokenHash: hash, ExpiresAt: now.Add(resetTokenTTL), CreatedAt: now,
	}); err != nil {
		return err
	}
	link := strings.TrimSuffix(s.d.PublicURL, "/") + "/auth/reset-password?token=" + url.QueryEscape(plain)
	bg := context.WithoutCancel(ctx)
	s.d.Async(func() { s.sendReset(bg, user, link) })
	return nil
}

func (s *Service) sendReset(ctx context.Context, user model.User, link string) {
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	sc := accountScope(user.CompanyID)
	settings, err := s.d.SMTP.Get(ctx, sc)
	if err != nil {
		s.resetFailed(ctx, user, "şirketin SMTP ayarları yok", err)
		return
	}
	password, err := s.d.SMTP.OpenPassword(ctx, sc)
	if err != nil {
		s.resetFailed(ctx, user, "SMTP parolası okunamadı", err)
		return
	}
	msg := mail.PasswordReset(user.Locale, user.Name, link)
	msg.To = []string{user.Email}
	if err := s.d.Mail.Send(ctx, settings, password, msg); err != nil {
		s.resetFailed(ctx, user, "e-posta gönderilemedi", err)
	}
}

// resetFailed records the failure without the token or link (R148).
func (s *Service) resetFailed(ctx context.Context, user model.User, reason string, cause error) {
	companyID, userID := user.CompanyID, user.ID
	related := "user"
	_, err := s.d.Ops.AppendMessage(ctx, accountScope(user.CompanyID), model.OperationalMessage{
		CompanyID: &companyID, Kind: "system", Category: "auth", Status: "error",
		Message:     fmt.Sprintf("Şifre sıfırlama e-postası gönderilemedi (%s): %s", user.Email, reason),
		RelatedType: &related, RelatedID: &userID, CreatedAt: s.d.Clock.Now(),
	})
	s.d.Log.WarnContext(ctx, "authsvc: password reset e-mail not sent", slog.String("reason", reason),
		slog.String("error", cause.Error()), slog.String("user_id", userID.String()))
	if err != nil {
		s.warn(ctx, "authsvc: record reset failure", err)
	}
}

// ResetPassword consumes a reset token and sets a new password (R148).
func (s *Service) ResetPassword(ctx context.Context, token, password string) error {
	if token == "" {
		return ErrResetTokenInvalid
	}
	reset, user, err := s.d.AdminAuth.PasswordResetByTokenHash(ctx, auth.HashRefreshToken(token))
	if errors.Is(err, store.ErrNotFound) {
		return ErrResetTokenInvalid
	}
	if err != nil {
		return err
	}
	now := s.d.Clock.Now()
	if reset.UsedAt != nil || !now.Before(reset.ExpiresAt) || !user.IsActive {
		return ErrResetTokenInvalid
	}
	codes, err := s.policy(ctx, user, password)
	if err != nil {
		return err
	}
	if len(codes) > 0 {
		return validation("password", codes)
	}
	hash, err := s.d.Hasher.Hash(password)
	if err != nil {
		return err
	}
	sc := accountScope(user.CompanyID)
	if err := s.d.Resets.MarkUsed(ctx, sc, reset.ID, now); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return ErrResetTokenInvalid
		}
		return err
	}
	if err := s.d.Users.SetPassword(ctx, sc, user.ID, hash, now); err != nil {
		return err
	}
	_, err = s.d.Sessions.RevokeAllForUserWithReason(ctx, sc, user.ID, "password_change", nil, now)
	return err
}
