package iso50001_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/iso50001"
)

func TestCatalogueIsCompleteInBothLocales(t *testing.T) {
	mains := iso50001.Mains()
	require.Len(t, mains, 5)
	subs := 0
	for _, m := range mains {
		require.NotEmpty(t, m.Title["tr"], m.ID)
		require.NotEmpty(t, m.Title["en"], m.ID)
		require.True(t, iso50001.IsMain(m.ID))
		for _, s := range m.Subs {
			subs++
			for _, loc := range []string{"tr", "en"} {
				require.NotEmpty(t, s.Title[loc], s.ID)
				require.NotEmpty(t, s.Description[loc], s.ID)
			}
			got, ok := iso50001.SubByID(s.ID)
			require.True(t, ok)
			require.Equal(t, s.ID, got.ID)
		}
	}
	require.Equal(t, 20, subs, "01 §7.17 lists 3+6+5+3+3")
	require.Equal(t, 20, len(iso50001.SubIDs()))
	_, ok := iso50001.SubByID("5.9")
	require.False(t, ok)
	require.False(t, iso50001.IsMain("5.1"))
	for id, want := range map[string]string{"6.3": "significant-energy-uses", "6.4": "energy-consumption-analysis",
		"6.5": "energy-consumption-analysis", "9.1": "energy-consumption-analysis", "7.5": "regression-analysis-instruction", "5.1": ""} {
		s, _ := iso50001.SubByID(id)
		require.Equal(t, want, s.Template, id)
	}
}

func TestProgressIsDoneOverTwenty(t *testing.T) {
	done := map[string]bool{"5.1": true, "5.2": true, "5.3": true, "6.1": true, "6.2": true, "6.3": true, "6.4": true}
	require.Equal(t, 35, iso50001.Progress(done), "7/20 = 35%")
	require.Equal(t, 0, iso50001.Progress(nil))
	all := map[string]bool{}
	for _, id := range iso50001.SubIDs() {
		all[id] = true
	}
	require.Equal(t, 100, iso50001.Progress(all))
	require.Equal(t, 5, iso50001.Progress(map[string]bool{"9.3": true}), "1/20")
	require.Equal(t, 0, iso50001.Progress(map[string]bool{"nope": true}), "unknown ids never count")
}

func d(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }

func TestGanttStatuses(t *testing.T) {
	today := d(2026, 6, 15)
	dates := map[string]iso50001.DateRange{
		"5": {Start: d(2026, 1, 1), End: d(2026, 3, 31)},
		"6": {Start: d(2026, 4, 1), End: d(2026, 8, 31)},
		"7": {Start: d(2026, 5, 1), End: d(2026, 9, 30)},
		"8": {Start: d(2026, 7, 1), End: d(2026, 10, 31)},
		"9": {Start: d(2026, 1, 1), End: d(2026, 6, 14)},
	}
	done := map[string]bool{"5.1": true, "5.2": true, "5.3": true, "6.1": true, "9.1": true}
	statuses, available, start, end := iso50001.Gantt(dates, done, today)
	require.True(t, available)
	require.Equal(t, map[string]string{
		"5": "completed",   // every sub done, even though past its end
		"6": "in_progress", // in range, some done
		"7": "not_started", // in range, nothing done (Q-G4)
		"8": "not_started", // future
		"9": "expired",     // past its end, not complete
	}, statuses)
	require.Equal(t, d(2026, 1, 1), *start)
	require.Equal(t, d(2026, 10, 31), *end)

	delete(dates, "8")
	statuses, available, _, _ = iso50001.Gantt(dates, done, today)
	require.False(t, available, "every main clause needs both dates")
	_, has := statuses["8"]
	require.False(t, has)
	require.Equal(t, "completed", statuses["5"])

	_, available, start, _ = iso50001.Gantt(nil, nil, today)
	require.False(t, available)
	require.Nil(t, start)
}

func TestValidateDates(t *testing.T) {
	full := []iso50001.ClauseDates{}
	for _, id := range []string{"5", "6", "7", "8", "9"} {
		full = append(full, iso50001.ClauseDates{ClauseID: id, Start: d(2026, 1, 1), End: d(2026, 2, 1)})
	}
	require.Empty(t, iso50001.ValidateDates(full))
	require.Equal(t, map[string]string{"clauses": "incomplete"}, iso50001.ValidateDates(full[:4]))
	bad := append([]iso50001.ClauseDates{}, full...)
	bad[1].Start = d(2026, 3, 1)
	require.Equal(t, map[string]string{"clauses.6": "start_after_end"}, iso50001.ValidateDates(bad))
	missing := append([]iso50001.ClauseDates{}, full...)
	missing[2].End = time.Time{}
	require.Equal(t, map[string]string{"clauses": "incomplete"}, iso50001.ValidateDates(missing))
	dup := append(append([]iso50001.ClauseDates{}, full...), full[0])
	require.Equal(t, map[string]string{"clauses": "invalid"}, iso50001.ValidateDates(dup))
	unknown := append([]iso50001.ClauseDates{}, full...)
	unknown[0].ClauseID = "10"
	require.Equal(t, map[string]string{"clauses": "invalid"}, iso50001.ValidateDates(unknown))
	same := append([]iso50001.ClauseDates{}, full...)
	same[0].End = same[0].Start
	require.Empty(t, iso50001.ValidateDates(same), "a one-day clause is valid")
}
