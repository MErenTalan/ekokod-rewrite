package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/render/invoicepdf"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// TaskEnqueuer is the job client seam.
type TaskEnqueuer interface {
	Enqueue(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

func enqueue(ctx context.Context, e TaskEnqueuer, task *asynq.Task, err error) error {
	if err != nil {
		return err
	}
	if _, err := e.Enqueue(ctx, task); err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) {
		return err
	}
	return nil
}

// Dispatcher implements job.BillingDispatcher.
type Dispatcher struct {
	Billable store.AdminBillingRepository
	Bills    store.BillRepository
	Enqueuer TaskEnqueuer
	Clock    clock.Clock
	MaxRetry int
}

// Dispatch enqueues generate for each billable building's last closed periods
// and a company generate per lookback key (the minimum key across its
// buildings), then a render for every live bill still missing its PDF (M-9).
func (d Dispatcher) Dispatch(ctx context.Context) error {
	buildings, err := d.Billable.BillableBuildings(ctx)
	if err != nil {
		return err
	}
	opts := job.TaskOptions{MaxRetry: d.MaxRetry}
	now := d.Clock.Now()
	companyKeys := map[uuid.UUID][]string{}
	var companies []uuid.UUID
	for _, b := range buildings {
		keys := LookbackKeys(b.CutoffDay, now)
		for _, key := range keys {
			p := job.BillingGeneratePayload{CompanyID: b.CompanyID, Scope: model.BillScopeBuilding, SubjectID: b.BuildingID, PeriodKey: key}
			task, err := job.NewBillingGenerateTask(p, opts)
			if err := enqueue(ctx, d.Enqueuer, task, err); err != nil {
				return err
			}
		}
		current, seen := companyKeys[b.CompanyID]
		if !seen {
			companies = append(companies, b.CompanyID)
			companyKeys[b.CompanyID] = keys
			continue
		}
		for i := range current {
			current[i] = min(current[i], keys[i])
		}
	}
	for _, company := range companies {
		for _, key := range companyKeys[company] {
			p := job.BillingGeneratePayload{CompanyID: company, Scope: model.BillScopeCompany, SubjectID: company, PeriodKey: key}
			task, err := job.NewBillingGenerateTask(p, opts)
			if err := enqueue(ctx, d.Enqueuer, task, err); err != nil {
				return err
			}
		}
		bills, err := listAll(func(p store.Page) ([]model.Bill, error) {
			return d.Bills.List(ctx, store.SystemScope(company), store.BillFilter{Page: p})
		})
		if err != nil {
			return err
		}
		for _, b := range bills {
			if b.PdfPath != nil || b.Status == model.BillStatusSuperseded {
				continue
			}
			task, err := job.NewBillingRenderTask(job.BillingRenderPayload{CompanyID: company, BillID: b.ID}, opts)
			if err := enqueue(ctx, d.Enqueuer, task, err); err != nil {
				return err
			}
		}
	}
	return nil
}

// LookbackKeys are the newest-first job.BillingDispatchLookbackPeriods closed
// period keys for a cut-off day (I-8).
func LookbackKeys(cutoffDay int, now time.Time) []string {
	latest := domain.LatestClosedPeriodKey(cutoffDay, now, consumption.SettleDelayMonthly, istanbul)
	month, err := time.Parse("2006-01", latest)
	if err != nil {
		return []string{latest}
	}
	keys := make([]string, 0, job.BillingDispatchLookbackPeriods)
	for i := range job.BillingDispatchLookbackPeriods {
		keys = append(keys, month.AddDate(0, -i, 0).Format("2006-01"))
	}
	return keys
}

var istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

// JobGenerator implements job.BillingGenerator.
type JobGenerator struct {
	Service  *Service
	Ops      store.OpsRepository
	Enqueuer TaskEnqueuer
	Clock    clock.Clock
	MaxRetry int
}

// Generate runs one bill with a job_runs record. A ComputeError fails the run
// and skips retries (the data will not fix itself in the retry window);
// RecomputeFlagged is always on so a flagged bill is retried (I-8).
func (g JobGenerator) Generate(ctx context.Context, p job.BillingGeneratePayload) error {
	sc := store.SystemScope(p.CompanyID)
	scopeJSON, _ := json.Marshal(p)
	run, err := g.Ops.StartRun(ctx, sc, model.JobRun{CompanyID: &p.CompanyID, JobType: job.TypeBillingGenerate, Scope: scopeJSON,
		StartedAt: g.Clock.Now().UTC(), Status: "running"})
	if err != nil {
		return err
	}
	res, genErr := g.Service.Generate(ctx, sc, GenerateRequest{Scope: p.Scope, SubjectID: p.SubjectID, PeriodKey: p.PeriodKey, Force: p.Force, RecomputeFlagged: true})
	finish := func(status string, processed, failed int32, errText *string, detail []byte) error {
		_, err := g.Ops.FinishRun(ctx, sc, run.ID, status, processed, 0, failed, errText, detail, g.Clock.Now().UTC())
		return err
	}
	if genErr != nil {
		text := genErr.Error()
		var ce *ComputeError
		detail, _ := json.Marshal(map[string]any{"error": text})
		if errors.As(genErr, &ce) {
			detail, _ = json.Marshal(map[string]any{"code": ce.Code, "detail": ce.Detail})
		}
		if err := finish("failed", 0, 1, &text, detail); err != nil {
			return errors.Join(genErr, err)
		}
		if ce != nil || errors.Is(genErr, ErrInvalidRequest) || errors.Is(genErr, store.ErrNotFound) {
			return job.SkipRetry(genErr)
		}
		return genErr
	}
	detail, _ := json.Marshal(map[string]any{"bill_id": res.Bill.ID, "created": res.Created, "status": res.Bill.Status})
	if err := finish("success", 1, 0, nil, detail); err != nil {
		return err
	}
	if !res.Created {
		return nil
	}
	task, err := job.NewBillingRenderTask(job.BillingRenderPayload{CompanyID: p.CompanyID, BillID: res.Bill.ID}, job.TaskOptions{MaxRetry: g.MaxRetry})
	return enqueue(ctx, g.Enqueuer, task, err)
}

// PDFRenderer implements job.BillingRenderer.
type PDFRenderer struct {
	Bills     store.BillRepository
	Companies store.CompanyRepository
	Buildings store.BuildingRepository
	Analyzers store.AnalyzerRepository
	Root      string
}

// Bytes renders a bill's PDF without storing it.
func (r PDFRenderer) Bytes(ctx context.Context, p job.BillingRenderPayload) ([]byte, model.Bill, error) {
	sc := store.SystemScope(p.CompanyID)
	b, err := r.Bills.Get(ctx, sc, p.BillID)
	if err != nil {
		return nil, model.Bill{}, err
	}
	lines, err := r.Bills.Lines(ctx, sc, b.ID)
	if err != nil {
		return nil, model.Bill{}, err
	}
	members, err := r.Bills.Members(ctx, sc, b.ID)
	if err != nil {
		return nil, model.Bill{}, err
	}
	company, err := r.Companies.Get(ctx, sc, p.CompanyID)
	if err != nil {
		return nil, model.Bill{}, err
	}
	doc := invoicepdf.Document{Bill: b, Lines: lines, CompanyName: company.Name}
	if b.BuildingID != nil {
		building, err := r.Buildings.Get(ctx, sc, *b.BuildingID)
		if err != nil {
			return nil, model.Bill{}, err
		}
		doc.BuildingName = building.Name
	}
	ids := make([]uuid.UUID, len(members))
	for i, m := range members {
		ids[i] = m.AnalyzerID
	}
	if len(ids) > 0 {
		analyzers, err := r.Analyzers.List(ctx, sc, store.AnalyzerFilter{IDs: ids, IncludeDeleted: true, Page: store.Page{Limit: 500}})
		if err != nil {
			return nil, model.Bill{}, err
		}
		slices.SortFunc(analyzers, func(a, b model.Analyzer) int { return compareStrings(a.InstallationNumber, b.InstallationNumber) })
		for _, a := range analyzers {
			name := ""
			if a.MeteringPointName != nil {
				name = *a.MeteringPointName
			}
			doc.Members = append(doc.Members, invoicepdf.Member{AnalyzerID: a.ID, Name: name, InstallationNumber: a.InstallationNumber})
		}
	}
	pdf, err := invoicepdf.Render(doc)
	if err != nil {
		return nil, model.Bill{}, job.SkipRetry(err)
	}
	return pdf, b, nil
}

// Render writes <Root>/bills/<company>/<bill>.pdf atomically (temp + rename)
// and records the path.
func (r PDFRenderer) Render(ctx context.Context, p job.BillingRenderPayload) error {
	sc := store.SystemScope(p.CompanyID)
	pdf, b, err := r.Bytes(ctx, p)
	if err != nil {
		return err
	}
	dir := filepath.Join(r.Root, "bills", p.CompanyID.String())
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	final := filepath.Join(dir, b.ID.String()+".pdf")
	tmp, err := os.CreateTemp(dir, ".render-*.pdf")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(pdf); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o640); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), final); err != nil {
		return fmt.Errorf("billing: store pdf: %w", err)
	}
	return r.Bills.SetPDFPath(ctx, sc, b.ID, final)
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
