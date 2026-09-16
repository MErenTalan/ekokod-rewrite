package consumption

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// MaxPeriodWindows bounds PeriodRequest.Windows: a year of cut-off periods
// plus one.
const MaxPeriodWindows = 13

// MinPeriodWindowWidth is the narrowest explicit window PeriodRequest accepts.
const MinPeriodWindowWidth = time.Hour

// PeriodRequest asks for invoice-grade consumption over explicit contiguous
// windows (R107).
type PeriodRequest struct {
	AnalyzerIDs []uuid.UUID
	// Windows are sorted and contiguous (Windows[i].To == Windows[i+1].From),
	// 1..MaxPeriodWindows of them, each at least MinPeriodWindowWidth wide.
	Windows []energy.Window
}

// PeriodConsumption is Consumption at Monthly semantics over req.Windows
// (R107, R111): each boundary instant resolves once, so adjacent windows
// telescope, and a window is present only once settled.
func (b *Billing) PeriodConsumption(ctx context.Context, sc store.Scope, req PeriodRequest) ([]Row, error) {
	rows, _, err := b.periodConsumptionWithGaps(ctx, sc, req)
	return rows, err
}

// PeriodConsumptionAndRecord additionally writes suspect-period and
// missing_readings anomalies for req.Windows, exactly as ConsumptionAndRecord
// does for Monthly buckets.
func (b *Billing) PeriodConsumptionAndRecord(ctx context.Context, sc store.Scope, req PeriodRequest) ([]Row, error) {
	if b.deps.Locker == nil {
		return nil, errRequired("BillingDeps.Locker")
	}
	rows, gapsByAnalyzer, err := b.periodConsumptionWithGaps(ctx, sc, req)
	if err != nil {
		return nil, err
	}
	if err := b.recordRowsAndGaps(ctx, sc, rows, gapsByAnalyzer); err != nil {
		return nil, err
	}
	return rows, nil
}

func (b *Billing) periodConsumptionWithGaps(ctx context.Context, sc store.Scope, req PeriodRequest) ([]Row, map[uuid.UUID][]bucketGap, error) {
	if err := validatePeriodRequest(sc, req); err != nil {
		return nil, nil, err
	}
	// Istanbul-located windows keep Row.Window identical to Monthly buckets.
	windows := make([]energy.Window, len(req.Windows))
	for i, w := range req.Windows {
		windows[i] = energy.Window{From: w.From.In(istanbul), To: w.To.In(istanbul)}
	}
	return b.consumptionOverWindows(ctx, sc, req.AnalyzerIDs, windowMode{level: energy.Monthly, explicit: true}, windows)
}

// validatePeriodRequest runs every PeriodRequest check before any I/O.
func validatePeriodRequest(sc store.Scope, req PeriodRequest) error {
	if !sc.Valid() {
		return ErrInvalidRequest
	}
	if len(req.AnalyzerIDs) == 0 || len(req.AnalyzerIDs) > MaxAnalyzersPerRequest || hasDuplicateAnalyzerID(req.AnalyzerIDs) {
		return ErrInvalidRequest
	}
	if len(req.Windows) == 0 || len(req.Windows) > MaxPeriodWindows {
		return ErrInvalidRequest
	}
	for i, w := range req.Windows {
		if err := validPeriodWindow(w); err != nil {
			return err
		}
		if i > 0 && !req.Windows[i-1].To.Equal(w.From) {
			return ErrInvalidRequest
		}
	}
	if req.Windows[len(req.Windows)-1].To.Sub(req.Windows[0].From) > MaxRequestSpan {
		return ErrInvalidRequest
	}
	if len(req.AnalyzerIDs)*len(req.Windows) > MaxCells {
		return ErrInvalidRequest
	}
	return nil
}

// validPeriodWindow rejects a reversed, empty, sub-hour or over-span window.
func validPeriodWindow(w energy.Window) error {
	if !w.Valid() || w.To.Sub(w.From) < MinPeriodWindowWidth || w.To.Sub(w.From) > MaxRequestSpan {
		return ErrInvalidRequest
	}
	return nil
}
