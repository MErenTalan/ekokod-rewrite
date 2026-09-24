package iso50001_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/iso50001"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/iso50001"
	"github.com/MErenTalan/ekokod-rewrite/internal/storage"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

var today = time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)

type world struct {
	iso            *fakeISO
	files          *fakeFiles
	buildings      *fakeBuildings
	company, other uuid.UUID
	b1, b2, otherB uuid.UUID
	admin, ba, oSc store.Scope
	root           string
}

func newWorld(t *testing.T) *world {
	w := &world{iso: newFakeISO(), files: &fakeFiles{rows: map[uuid.UUID]model.StoredFile{}}, buildings: &fakeBuildings{byID: map[uuid.UUID]model.Building{}},
		company: uuid.New(), other: uuid.New(), b1: uuid.New(), b2: uuid.New(), otherB: uuid.New(), root: t.TempDir()}
	for _, b := range []struct{ id, c uuid.UUID }{{w.b1, w.company}, {w.b2, w.company}, {w.otherB, w.other}} {
		w.iso.owner[b.id] = b.c
		w.buildings.byID[b.id] = model.Building{ID: b.id, CompanyID: b.c, Name: "Merkez"}
	}
	w.admin, w.oSc = store.SystemScope(w.company), store.SystemScope(w.other)
	w.ba = store.Scope{CompanyID: w.company, BuildingIDs: []uuid.UUID{w.b1}}
	return w
}

func (w *world) svc() *iso50001.Service {
	return iso50001.New(iso50001.Deps{ISO: w.iso, Files: w.files, Buildings: w.buildings, Clock: clock.NewFake(today),
		Store: storage.Store{Root: w.root, Max: 1 << 20, Allowed: []string{"application/pdf"}}})
}

func requireValidation(t *testing.T, err error, field, code string) {
	t.Helper()
	var pe *perr.Error
	require.True(t, errors.As(err, &pe), "want validation on %s, got %v", field, err)
	require.Equal(t, []string{code}, pe.Params[field], "%v", pe.Params)
}

func day(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }

func fiveClauses() []domain.ClauseDates {
	return []domain.ClauseDates{
		{ClauseID: "5", Start: day(1, 1), End: day(3, 31)}, {ClauseID: "6", Start: day(4, 1), End: day(8, 31)},
		{ClauseID: "7", Start: day(5, 1), End: day(9, 30)}, {ClauseID: "8", Start: day(7, 1), End: day(10, 31)},
		{ClauseID: "9", Start: day(1, 1), End: day(6, 14)},
	}
}

func TestEmptyProjectBeforeTheFirstWrite(t *testing.T) {
	w := newWorld(t)
	st, err := w.svc().Project(context.Background(), w.ba, w.b1)
	require.NoError(t, err)
	require.Equal(t, 0, st.Progress)
	require.False(t, st.GanttAvailable)
	require.Empty(t, st.Dates)
	require.Len(t, st.Counts, 20, "every sub-clause present, zero counts")
	_, err = w.svc().Project(context.Background(), w.ba, w.b2)
	require.ErrorIs(t, err, store.ErrNotFound, "a building admin outside their building")
	_, err = w.svc().Project(context.Background(), w.admin, w.otherB)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestDatesAndStateReflectNotes(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	s := w.svc()
	for _, c := range []string{"5.1", "5.2", "5.3", "6.1"} {
		_, err := s.CreateNote(ctx, w.ba, uuid.New(), w.b1, c, nil, "Kanıt")
		require.NoError(t, err)
	}
	st, err := s.SetDates(ctx, w.ba, w.b1, fiveClauses())
	require.NoError(t, err)
	require.True(t, st.GanttAvailable)
	require.Equal(t, 20, st.Progress, "4 of 20")
	require.Equal(t, "completed", st.Statuses["5"])
	require.Equal(t, "in_progress", st.Statuses["6"])
	require.Equal(t, "expired", st.Statuses["9"])
	require.Equal(t, 1, st.Counts["5.1"].Notes)
	require.Equal(t, day(1, 1), *st.ProjectStart)

	bad := fiveClauses()
	bad[0].Start = day(4, 1)
	_, err = s.SetDates(ctx, w.ba, w.b1, bad)
	requireValidation(t, err, "clauses.5", "start_after_end")
	_, err = s.SetDates(ctx, w.ba, w.b1, fiveClauses()[:3])
	requireValidation(t, err, "clauses", "incomplete")
	_, err = s.SetDates(ctx, w.ba, w.b2, fiveClauses())
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestNoteRules(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	s := w.svc()
	_, err := s.CreateNote(ctx, w.admin, uuid.New(), w.b1, "5.9", nil, "x")
	requireValidation(t, err, "clause", "unknown")
	_, err = s.CreateNote(ctx, w.admin, uuid.New(), w.b1, "5", nil, "x")
	requireValidation(t, err, "clause", "unknown")
	_, err = s.CreateNote(ctx, w.admin, uuid.New(), w.b1, "5.1", nil, "   ")
	requireValidation(t, err, "body", "required")
	_, err = s.CreateNote(ctx, w.admin, uuid.New(), w.b1, "5.1", nil, strings.Repeat("a", 10_001))
	requireValidation(t, err, "body", "too_long")
	long := strings.Repeat("b", 201)
	_, err = s.CreateNote(ctx, w.admin, uuid.New(), w.b1, "5.1", &long, "x")
	requireValidation(t, err, "title", "too_long")

	title := "  Politika  "
	user := uuid.New()
	n, err := s.CreateNote(ctx, w.admin, user, w.b1, "5.2", &title, "  Enerji politikası yayımlandı.  ")
	require.NoError(t, err)
	require.Equal(t, "Politika", *n.Title)
	require.Equal(t, "Enerji politikası yayımlandı.", n.Body)
	require.Equal(t, user, *n.CreatedBy)

	updated, err := s.UpdateNote(ctx, w.ba, n.ID, "5.2", nil, "Güncel")
	require.NoError(t, err)
	require.Equal(t, "Güncel", updated.Body)
	_, err = s.UpdateNote(ctx, w.oSc, n.ID, "5.2", nil, "x")
	require.ErrorIs(t, err, store.ErrNotFound)

	list, err := s.Notes(ctx, w.ba, w.b1, "5.2")
	require.NoError(t, err)
	require.Len(t, list, 1)
	empty, err := s.Notes(ctx, w.ba, w.b1, "7.1")
	require.NoError(t, err)
	require.Empty(t, empty)

	require.ErrorIs(t, s.DeleteNote(ctx, w.oSc, n.ID), store.ErrNotFound)
	require.NoError(t, s.DeleteNote(ctx, w.ba, n.ID))
}
