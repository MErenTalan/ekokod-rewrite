package billing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/render/hourlyxlsx"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Requests is the API's side of billing (R179): compute enqueueing, PDFs and
// the hourly workbook.
type Requests struct {
	Service  *Service
	Renderer PDFRenderer
	Enqueuer TaskEnqueuer
	// Tasks lets a recompute replace a finished task that still holds the
	// bill's deterministic id (an archived failure); nil keeps the dedupe.
	Tasks    job.TaskInspector
	MaxRetry int
	Location *time.Location
	// Plants fills the dashboard's plant section (R290); nil leaves it unavailable.
	Plants PlantSection
}

// PlantSection is the solar module's side of the dashboard.
type PlantSection interface {
	BillPlants(ctx context.Context, sc store.Scope, year, month int) (billing.DashboardPlants, error)
}

// ComputeRequest is POST /bills/compute.
type ComputeRequest struct {
	Scope                    model.BillScope
	BuildingIDs, AnalyzerIDs []uuid.UUID
	PeriodKey                string
	Force                    bool
}

// Compute enqueues billing.generate for each visible target and returns the
// deterministic job ids. A company bill needs a whole-company scope.
func (q Requests) Compute(ctx context.Context, sc store.Scope, req ComputeRequest) ([]string, error) {
	if _, err := billing.Period(req.PeriodKey, 1, q.Location); err != nil {
		return nil, perr.Validation.WithParams(map[string]any{"period": []string{"invalid"}})
	}
	var subjects []uuid.UUID
	switch req.Scope {
	case model.BillScopeCompany:
		if _, all := sc.BuildingFilter(); !all {
			return nil, perr.Forbidden
		}
		subjects = []uuid.UUID{sc.CompanyID}
	case model.BillScopeBuilding:
		for _, id := range req.BuildingIDs {
			if _, err := q.Service.deps.Buildings.Get(ctx, sc, id); err != nil {
				return nil, err
			}
		}
		subjects = req.BuildingIDs
	case model.BillScopeAnalyzer:
		for _, id := range req.AnalyzerIDs {
			if _, err := q.Service.deps.Analyzers.Get(ctx, sc, id); err != nil {
				return nil, err
			}
		}
		subjects = req.AnalyzerIDs
	default:
		return nil, perr.Validation.WithParams(map[string]any{"scope": []string{"oneof"}})
	}
	if len(subjects) == 0 {
		return nil, perr.Validation.WithParams(map[string]any{"targets": []string{"required"}})
	}
	ids := make([]string, 0, len(subjects))
	for _, subject := range subjects {
		p := job.BillingGeneratePayload{CompanyID: sc.CompanyID, Scope: req.Scope, SubjectID: subject, PeriodKey: req.PeriodKey, Force: req.Force}
		task, err := job.NewBillingGenerateTask(p, job.TaskOptions{MaxRetry: q.MaxRetry})
		if err != nil {
			return nil, err
		}
		if err := job.EnqueueReplacingFinished(ctx, q.Enqueuer, q.Tasks, task, job.BillingGenerateTaskID(p)); err != nil {
			return nil, err
		}
		ids = append(ids, job.BillingGenerateTaskID(p))
	}
	return ids, nil
}

// PDF returns the stored invoice PDF, or renders it now and enqueues the
// render job so it is stored for next time; a GET never writes (R179).
func (q Requests) PDF(ctx context.Context, sc store.Scope, id uuid.UUID) ([]byte, model.Bill, error) {
	b, err := q.Service.deps.Bills.Get(ctx, sc, id)
	if err != nil {
		return nil, model.Bill{}, err
	}
	if b.PdfPath != nil {
		if raw, ok := q.stored(b); ok {
			return raw, b, nil
		}
	}
	p := job.BillingRenderPayload{CompanyID: b.CompanyID, BillID: b.ID}
	pdf, _, err := q.Renderer.Bytes(ctx, p)
	if err != nil {
		return nil, model.Bill{}, err
	}
	task, terr := job.NewBillingRenderTask(p, job.TaskOptions{MaxRetry: q.MaxRetry})
	if err := enqueue(ctx, q.Enqueuer, task, terr); err != nil {
		return nil, model.Bill{}, err
	}
	return pdf, b, nil
}

// stored reads a recorded PDF path only when it resolves inside the company's bill directory.
func (q Requests) stored(b model.Bill) ([]byte, bool) {
	dir := filepath.Join(q.Renderer.Root, "bills", b.CompanyID.String()) + string(filepath.Separator)
	path := filepath.Clean(*b.PdfPath)
	if !strings.HasPrefix(path, dir) {
		return nil, false
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || err != nil {
		return nil, false
	}
	return raw, true
}

// HourlyXLSX renders the priced hours of a PTF bill.
func (q Requests) HourlyXLSX(ctx context.Context, sc store.Scope, id uuid.UUID, locale string) ([]byte, model.Bill, error) {
	b, err := q.Service.deps.Bills.Get(ctx, sc, id)
	if err != nil {
		return nil, model.Bill{}, err
	}
	rows, err := q.Service.HourlyDetail(ctx, sc, id)
	if err != nil {
		return nil, model.Bill{}, err
	}
	out, err := hourlyxlsx.Render(b, rows, locale)
	return out, b, err
}
