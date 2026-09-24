package report

import (
	"context"
	"fmt"
	"strconv"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Dispatcher implements job.ReportDispatcher (R268): every building with an
// analyzer gets the previous Istanbul month's or year's report.
type Dispatcher struct {
	Billable store.AdminBillingRepository
	Enqueuer TaskEnqueuer
	Clock    clock.Clock
	MaxRetry int
}

// Dispatch enqueues one report.generate per building.
func (d Dispatcher) Dispatch(ctx context.Context, kind string) error {
	local := d.Clock.Now().In(istanbul)
	var key string
	switch kind {
	case domain.TypeMonthly:
		key = local.AddDate(0, 0, 1-local.Day()).AddDate(0, -1, 0).Format("2006-01")
	case domain.TypeYearly:
		key = strconv.Itoa(local.Year() - 1)
	default:
		return fmt.Errorf("report.dispatch: unknown kind %q", kind)
	}
	buildings, err := d.Billable.BillableBuildings(ctx)
	if err != nil {
		return err
	}
	for _, b := range buildings {
		p := job.ReportGeneratePayload{CompanyID: b.CompanyID, BuildingID: b.BuildingID, Type: kind, Period: key, PlantSelection: domain.SelectionAll}
		task, terr := job.NewReportGenerateTask(p, job.TaskOptions{MaxRetry: d.MaxRetry})
		if err := enqueue(ctx, d.Enqueuer, task, terr); err != nil {
			return err
		}
	}
	return nil
}
