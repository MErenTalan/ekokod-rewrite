package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// ErrEmailTaken is returned when another live user already has the e-mail.
var ErrEmailTaken = perr.New("email_taken", 409, "errors.users.emailTaken")

// ProfileInput is a partial update of the caller's own profile; nil keeps a field.
type ProfileInput struct {
	Name, Email, Phone, Locale, UIPreferences *string
}

// UpdateProfile changes the caller's own name, e-mail, phone, locale and UI
// preferences (05 §3, R169). The shared demo account cannot be changed (R150).
func (s *Service) UpdateProfile(ctx context.Context, p Principal, in ProfileInput) (model.User, error) {
	if p.User.Role == model.UserRoleDemo {
		return model.User{}, perr.Forbidden
	}
	sc := accountScope(p.User.CompanyID)
	user, err := s.d.Users.Get(ctx, sc, p.User.ID)
	if err != nil {
		return model.User{}, err
	}
	if in.Name != nil {
		user.Name = strings.TrimSpace(*in.Name)
	}
	if in.Email != nil {
		user.Email = strings.TrimSpace(*in.Email)
	}
	if in.Phone != nil {
		user.Phone = emptyToNil(*in.Phone)
	}
	if in.Locale != nil {
		user.Locale = *in.Locale
	}
	if in.UIPreferences != nil {
		user.UIPreferences = emptyToNil(*in.UIPreferences)
	}
	user.UpdatedAt = s.d.Clock.Now()
	updated, err := s.d.Users.Update(ctx, sc, user)
	if errors.Is(err, store.ErrConflict) {
		return model.User{}, ErrEmailTaken
	}
	return updated, err
}

func emptyToNil(v string) *string {
	if v = strings.TrimSpace(v); v == "" {
		return nil
	}
	return &v
}
