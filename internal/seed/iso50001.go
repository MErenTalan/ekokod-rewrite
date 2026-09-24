package seed

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

// ensureISO50001 gives A1 a planned project: clause 5 done and past, 6
// running, 7–9 ahead, and three notes (F11a). Files are uploaded by the e2e
// spec itself, through the real handler. Idempotent.
func ensureISO50001(ctx context.Context, pool *pgxpool.Pool, f Fixtures, now time.Time) error {
	sc := store.SystemScope(f.CompanyA)
	repo := postgres.NewISO50001Repository(pool)
	p, err := repo.EnsureProject(ctx, sc, f.BuildingA1)
	if err != nil {
		return fmt.Errorf("seed iso50001 project: %w", err)
	}
	ist, _ := time.LoadLocation("Europe/Istanbul")
	y, m, _ := now.In(ist).Date()
	month := func(k int) time.Time { return time.Date(y, m+time.Month(k), 1, 0, 0, 0, 0, time.UTC) }
	var dates []model.ISO50001ClauseDate
	for i, span := range [][2]int{{-4, -2}, {-1, 2}, {1, 4}, {3, 6}, {5, 8}} {
		start, end := month(span[0]), month(span[1]).AddDate(0, 0, -1)
		dates = append(dates, model.ISO50001ClauseDate{ProjectID: p.ID, ClauseID: fmt.Sprint(5 + i), StartDate: &start, EndDate: &end})
	}
	if err := repo.ReplaceClauseDates(ctx, sc, p.ID, dates); err != nil {
		return fmt.Errorf("seed iso50001 dates: %w", err)
	}
	existing, err := repo.Notes(ctx, sc, p.ID, nil)
	if err != nil || len(existing) > 0 {
		return err
	}
	for _, n := range []struct{ clause, title, body string }{
		{"5.1", "Üst yönetim taahhüdü", "Genel müdür EnYS taahhüt yazısını imzaladı."},
		{"5.2", "Enerji politikası", "Enerji politikası yayımlandı ve panoya asıldı."},
		{"6.1", "Risk kaydı", "Enerji riskleri ve fırsatları tablosu hazırlandı."},
	} {
		title := n.title
		if _, err := repo.CreateNote(ctx, sc, model.ISO50001Note{ProjectID: p.ID, ClauseID: n.clause, Title: &title, Body: n.body}); err != nil {
			return fmt.Errorf("seed iso50001 note %s: %w", n.clause, err)
		}
	}
	return nil
}
