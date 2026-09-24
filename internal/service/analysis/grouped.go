package analysis

import (
	"context"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/grouping"
	domainlp "github.com/MErenTalan/ekokod-rewrite/internal/domain/loadprofile"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// MaxGroupedDays bounds a grouped request: a year plus a day, so a "this year
// against last year" comparison fits and nothing unbounded gets through.
const MaxGroupedDays = 366

// GroupedInput is one grouped-consumption query (R193).
type GroupedInput struct {
	Subject
	From, To        time.Time // inclusive Istanbul days
	By              grouping.By
	ComparePrevious bool
}

// GroupedPeriod is one period's buckets and their statistics.
type GroupedPeriod struct {
	From, To   time.Time
	Buckets    []grouping.Bucket
	Statistics grouping.Statistics
}

// Grouped is the grouped read, optionally with the period before it.
type Grouped struct {
	By       grouping.By
	Current  GroupedPeriod
	Previous *GroupedPeriod
}

// Grouped buckets the subject's daily consumption (R193). Day types come from
// the company calendar through the load-profile service, so the Consumption
// screen and the Load Profile screen always agree on what a weekend is (R137).
func (s *Service) Grouped(ctx context.Context, sc store.Scope, in GroupedInput) (Grouped, error) {
	if !grouping.Valid(in.By) {
		return Grouped{}, ErrInvalidParameters.WithParams(map[string]any{"group_by": []string{"oneof"}})
	}
	if in.To.Before(in.From) {
		return Grouped{}, ErrInvalidParameters.WithParams(map[string]any{"to": []string{"gtefield"}})
	}
	days := int(in.To.Sub(in.From).Hours()/24) + 1
	if days > MaxGroupedDays {
		return Grouped{}, ErrInvalidParameters.WithParams(map[string]any{"from": []string{"range_too_wide"}})
	}

	from, to := in.From, in.To
	if in.ComparePrevious {
		from = in.From.AddDate(0, 0, -days)
	}
	cfg, err := s.d.Profiles.CalendarConfig(ctx, sc, store.TimeRange{From: from, To: to.AddDate(0, 0, 1)})
	if err != nil {
		return Grouped{}, mapErr(err)
	}

	current, err := s.groupPeriod(ctx, sc, in, in.From, in.To, cfg)
	if err != nil {
		return Grouped{}, err
	}
	out := Grouped{By: in.By, Current: current}
	if in.ComparePrevious {
		previous, perr := s.groupPeriod(ctx, sc, in, from, in.From.AddDate(0, 0, -1), cfg)
		if perr != nil {
			return Grouped{}, perr
		}
		out.Previous = &previous
	}
	return out, nil
}

func (s *Service) groupPeriod(ctx context.Context, sc store.Scope, in GroupedInput, from, to time.Time,
	cfg domainlp.Config) (GroupedPeriod, error) {
	rows, _, err := s.Rows(ctx, sc, SeriesInput{Subject: in.Subject, Level: energy.Daily, From: from, To: to})
	if err != nil {
		return GroupedPeriod{}, err
	}
	days := make([]grouping.Day, 0, len(rows))
	for _, row := range rows {
		days = append(days, grouping.Day{
			Date:       row.Window.From,
			Active:     row.Values[energy.ActiveImport],
			Inductive:  row.Values[energy.ReactiveInductiveImport],
			Capacitive: row.Values[energy.ReactiveCapacitiveImport],
			Partial:    row.Partial || len(row.Suspect) > 0,
		})
	}
	buckets := grouping.Group(days, in.By, cfg)
	return GroupedPeriod{From: from, To: to, Buckets: buckets, Statistics: grouping.Stats(buckets)}, nil
}
