package mail

import (
	"context"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// GuardDemo drops every message of a demo company and reports success (R544),
// so a visitor's alarm recipients or report addresses never receive mail.
// A failed demo check refuses the send.
func GuardDemo(inner Sender, isDemo func(context.Context, uuid.UUID) (bool, error)) Sender {
	return demoSender{inner: inner, isDemo: isDemo}
}

type demoSender struct {
	inner  Sender
	isDemo func(context.Context, uuid.UUID) (bool, error)
}

func (s demoSender) Send(ctx context.Context, cfg model.SMTPSettings, password []byte, m Message) error {
	demo, err := s.isDemo(ctx, cfg.CompanyID)
	if err != nil {
		return err
	}
	if demo {
		return nil
	}
	return s.inner.Send(ctx, cfg, password, m)
}
