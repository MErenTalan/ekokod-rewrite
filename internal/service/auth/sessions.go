package auth

import (
	"context"
	"net/netip"
	"time"

	"github.com/google/uuid"

	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// SessionView is one active session as the owner sees it.
type SessionView struct {
	ID                   uuid.UUID
	Client               Client
	UserAgent            *string
	IP                   *netip.Addr
	CreatedAt, ExpiresAt time.Time
	LastUsedAt           *time.Time
	Current              bool
}

// Logout revokes the caller's current session.
func (s *Service) Logout(ctx context.Context, p Principal) error {
	return s.d.Sessions.RevokeWithReason(ctx, accountScope(p.User.CompanyID), p.SessionID, "logout", s.d.Clock.Now())
}

// LogoutAll revokes every session of the caller.
func (s *Service) LogoutAll(ctx context.Context, p Principal) error {
	_, err := s.d.Sessions.RevokeAllForUserWithReason(ctx, accountScope(p.User.CompanyID), p.User.ID, "logout_all", nil, s.d.Clock.Now())
	return err
}

// Sessions lists the caller's active sessions, newest first.
func (s *Service) Sessions(ctx context.Context, p Principal) ([]SessionView, error) {
	now := s.d.Clock.Now()
	var out []SessionView
	for offset := int32(0); ; offset += sessionPageSize {
		page, err := s.d.Sessions.List(ctx, accountScope(p.User.CompanyID), store.SessionFilter{
			UserID: &p.User.ID, ActiveAt: &now, Page: store.Page{Limit: sessionPageSize, Offset: offset},
		})
		if err != nil {
			return nil, err
		}
		for _, sess := range page {
			out = append(out, SessionView{
				ID: sess.ID, Client: Client(sess.Client), UserAgent: sess.UserAgent, IP: sess.IP,
				CreatedAt: sess.CreatedAt, ExpiresAt: sess.ExpiresAt, LastUsedAt: sess.LastUsedAt,
				Current: sess.ID == p.SessionID,
			})
		}
		if len(page) < sessionPageSize {
			return out, nil
		}
	}
}

// RevokeSession revokes one of the caller's own sessions; any other id is not found.
func (s *Service) RevokeSession(ctx context.Context, p Principal, id uuid.UUID) error {
	sc := accountScope(p.User.CompanyID)
	sess, err := s.d.Sessions.Get(ctx, sc, id)
	if err != nil {
		return err
	}
	if sess.UserID != p.User.ID {
		return perr.NotFound
	}
	return s.d.Sessions.RevokeWithReason(ctx, sc, id, "logout", s.d.Clock.Now())
}
