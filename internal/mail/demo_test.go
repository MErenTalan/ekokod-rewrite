package mail_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/mail"
)

type spySender struct{ sent int }

func (s *spySender) Send(context.Context, model.SMTPSettings, []byte, mail.Message) error {
	s.sent++
	return nil
}

func TestGuardDemoDropsDemoMail(t *testing.T) {
	demoID, realID := uuid.New(), uuid.New()
	var checkErr error
	isDemo := func(_ context.Context, id uuid.UUID) (bool, error) { return id == demoID, checkErr }
	spy := &spySender{}
	s := mail.GuardDemo(spy, isDemo)
	msg := mail.Message{To: []string{"ops@example.com"}, Subject: "x", Text: "y"}

	require.NoError(t, s.Send(t.Context(), model.SMTPSettings{CompanyID: demoID}, nil, msg))
	require.Equal(t, 0, spy.sent, "R544: demo mail never leaves")

	require.NoError(t, s.Send(t.Context(), model.SMTPSettings{CompanyID: realID}, nil, msg))
	require.Equal(t, 1, spy.sent)

	checkErr = errors.New("db down")
	require.Error(t, s.Send(t.Context(), model.SMTPSettings{CompanyID: realID}, nil, msg), "fail closed")
	require.Equal(t, 1, spy.sent)
}
