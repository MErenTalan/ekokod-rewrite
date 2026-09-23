package report

import (
	"context"
	"encoding/json"
	"errors"
	"net/mail"
	"strings"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// MaxRecipients is R266's cap on one send.
const MaxRecipients = 10

// TaskEnqueuer is the job client seam.
type TaskEnqueuer interface {
	Enqueue(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

func enqueue(ctx context.Context, e TaskEnqueuer, task *asynq.Task, err error) error {
	if err != nil {
		return err
	}
	// Already queued for the same building and period is the same request.
	if _, err := e.Enqueue(ctx, task); err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) && !errors.Is(err, asynq.ErrDuplicateTask) {
		return err
	}
	return nil
}

// Requests is the API's side of reports: generation, delivery and files.
type Requests struct {
	Service  *Service
	Reports  store.ReportRepository
	Enqueuer TaskEnqueuer
	Files    Files
	Clock    clock.Clock
	MaxRetry int
}

// Enqueued is one building's generation.
type Enqueued struct {
	BuildingID, ReportID uuid.UUID
	JobID                string
}

// Enqueue is R263/R264: one report.generate per building. Every building and
// plant is checked first, so a refusal writes and enqueues nothing.
func (q Requests) Enqueue(ctx context.Context, sc store.Scope, r Request) ([]Enqueued, error) {
	p, err := q.Service.Validate(r)
	if err != nil {
		return nil, err
	}
	for _, id := range r.BuildingIDs {
		if _, err := q.Service.d.Buildings.Get(ctx, sc, id); err != nil {
			return nil, err
		}
	}
	if len(r.PlantIDs) > 0 && !sc.AllBuildings {
		return nil, store.ErrNotFound // plants are not a building-scoped principal's to name
	}
	for _, id := range r.PlantIDs {
		if _, err := q.Service.d.Plants.Get(ctx, sc, id); err != nil {
			return nil, err
		}
	}
	out := make([]Enqueued, 0, len(r.BuildingIDs))
	for _, id := range r.BuildingIDs {
		rp, err := q.pending(ctx, sc, id, r, p)
		if err != nil {
			return nil, err
		}
		payload := job.ReportGeneratePayload{CompanyID: sc.CompanyID, BuildingID: id, Type: r.Type, Period: p.key(),
			PlantSelection: r.Selection, PlantIDs: r.PlantIDs}
		task, terr := job.NewReportGenerateTask(payload, job.TaskOptions{MaxRetry: q.MaxRetry})
		if err := enqueue(ctx, q.Enqueuer, task, terr); err != nil {
			return nil, err
		}
		out = append(out, Enqueued{BuildingID: id, ReportID: rp.ID, JobID: job.ReportGenerateTaskID(payload)})
	}
	return out, nil
}

// pending marks the building's report for the period pending, keeping its
// old figures and files readable, or creates it (R264).
func (q Requests) pending(ctx context.Context, sc store.Scope, building uuid.UUID, r Request, p period) (model.Report, error) {
	existing, err := findReport(ctx, q.Reports, sc, building, r.Type, p.key())
	if err != nil {
		return model.Report{}, err
	}
	now := q.Clock.Now().UTC()
	if existing != nil {
		return q.Reports.UpdateStatus(ctx, sc, existing.ID, model.ReportStatusPending, nil, now)
	}
	return q.Reports.Upsert(ctx, sc, model.Report{CompanyID: sc.CompanyID, BuildingID: building, Type: model.ReportType(r.Type),
		Period: p.key(), PlantSelection: model.PlantSelection(r.Selection), Payload: json.RawMessage(`{}`),
		Status: model.ReportStatusPending, CreatedAt: now})
}

func findReport(ctx context.Context, repo store.ReportRepository, sc store.Scope, building uuid.UUID, kind, key string) (*model.Report, error) {
	t := model.ReportType(kind)
	list, err := repo.List(ctx, sc, store.ReportFilter{BuildingID: &building, Type: &t, Period: &key, Page: store.Page{Limit: 1}})
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}

// EnqueueDelivery is R266: validate the recipients, then enqueue a new send.
func (q Requests) EnqueueDelivery(ctx context.Context, sc store.Scope, reportID uuid.UUID, to []string) (string, error) {
	if len(to) == 0 {
		return "", invalid("to", "required")
	}
	if len(to) > MaxRecipients {
		return "", invalid("to", "max")
	}
	for _, addr := range to {
		if strings.ContainsAny(addr, "\r\n") {
			return "", invalid("to", "email")
		}
		if parsed, err := mail.ParseAddress(addr); err != nil || parsed.Address != addr {
			return "", invalid("to", "email")
		}
	}
	if _, err := q.Reports.Get(ctx, sc, reportID); err != nil {
		return "", err
	}
	p := job.ReportDeliverPayload{CompanyID: sc.CompanyID, ReportID: reportID, To: to, RequestID: uuid.New()}
	task, err := job.NewReportDeliverTask(p, job.TaskOptions{MaxRetry: q.MaxRetry})
	if err := enqueue(ctx, q.Enqueuer, task, err); err != nil {
		return "", err
	}
	return job.ReportDeliverTaskID(p), nil
}

// Generator implements job.ReportGenerator.
type Generator struct {
	Service   *Service
	Reports   store.ReportRepository
	Companies store.CompanyRepository
	Ops       store.OpsRepository
	Files     Files
	Clock     clock.Clock
}

// Generate builds, renders, stores and completes one building's report,
// recording a job_runs row under its task id (R264, R267).
func (g Generator) Generate(ctx context.Context, p job.ReportGeneratePayload) error {
	sc := store.SystemScope(p.CompanyID)
	taskID := job.ReportGenerateTaskID(p)
	scopeJSON, _ := json.Marshal(p)
	run, err := g.Ops.StartRun(ctx, sc, model.JobRun{CompanyID: &p.CompanyID, JobType: job.TypeReportGenerate, Scope: scopeJSON,
		StartedAt: g.Clock.Now().UTC(), Status: "running", TaskID: &taskID})
	if err != nil {
		return err
	}
	finish := func(status string, failed int32, errText *string, detail any) error {
		raw, _ := json.Marshal(detail)
		processed := int32(1) - failed
		_, err := g.Ops.FinishRun(ctx, sc, run.ID, status, processed, 0, failed, errText, raw, g.Clock.Now().UTC())
		return err
	}
	rp, genErr := g.generate(ctx, sc, p)
	if genErr == nil {
		return finish("success", 0, nil, map[string]any{"report_id": rp.ID})
	}
	code := ""
	switch {
	case errors.Is(genErr, store.ErrNotFound):
		code = "report_building_not_found"
	}
	text := secret.Redact(genErr.Error(), nil)
	if rp.ID != uuid.Nil {
		if _, err := g.Reports.UpdateStatus(ctx, sc, rp.ID, model.ReportStatusError, &text, g.Clock.Now().UTC()); err != nil {
			genErr = errors.Join(genErr, err)
		}
	}
	if err := finish("failed", 1, &text, map[string]any{"code": code}); err != nil {
		return errors.Join(genErr, err)
	}
	if code != "" || isValidation(genErr) {
		return job.SkipRetry(genErr)
	}
	return genErr
}

func (g Generator) generate(ctx context.Context, sc store.Scope, p job.ReportGeneratePayload) (model.Report, error) {
	req := Request{Type: p.Type, Period: p.Period, Selection: p.PlantSelection, BuildingIDs: []uuid.UUID{p.BuildingID}, PlantIDs: p.PlantIDs}
	existing, err := findReport(ctx, g.Reports, sc, p.BuildingID, p.Type, p.Period)
	if err != nil {
		return model.Report{}, err
	}
	now := g.Clock.Now().UTC()
	rp := model.Report{CompanyID: p.CompanyID, BuildingID: p.BuildingID, Type: model.ReportType(p.Type), Period: p.Period,
		PlantSelection: model.PlantSelection(p.PlantSelection), Payload: json.RawMessage(`{}`), Status: model.ReportStatusPending, CreatedAt: now}
	if existing != nil {
		rp = *existing
	} else if rp, err = g.Reports.Upsert(ctx, sc, rp); err != nil {
		return model.Report{}, err
	}
	built, err := g.Service.Build(ctx, sc, req)
	if err != nil {
		return rp, err
	}
	raw, err := json.Marshal(built)
	if err != nil {
		return rp, err
	}
	names := buildingNames(built)
	rp.Payload, rp.ProcessedAt, rp.PlantSelection = raw, &now, model.PlantSelection(p.PlantSelection)
	for _, format := range []string{FormatPDF, FormatExcel} {
		data, err := g.Files.render(ctx, rp, built, format, "tr")
		if err != nil {
			return rp, err
		}
		path, err := g.Files.write(p.CompanyID, rp.ID, format, data)
		if err != nil {
			return rp, err
		}
		if format == FormatPDF {
			rp.PdfPath = &path
		} else {
			rp.ExcelPath = &path
		}
	}
	subject, body, err := EmailContent(built, names, "tr")
	if err != nil {
		return rp, err
	}
	rp.EmailSubject, rp.EmailBody, rp.Status, rp.ErrorMessage = &subject, &body, model.ReportStatusCompleted, nil
	return g.Reports.Upsert(ctx, sc, rp)
}

func isValidation(err error) bool { return errors.Is(err, perr.Validation) }

// buildingNames are the payload's building names, in its order.
func buildingNames(p domain.Payload) []string {
	var lines []domain.BuildingLine
	switch {
	case p.Monthly != nil:
		lines = p.Monthly.Buildings
	case p.Yearly != nil:
		lines = p.Yearly.Buildings
	}
	names := make([]string, 0, len(lines))
	for _, b := range lines {
		names = append(names, b.Name)
	}
	return names
}

// ArchiveItem is one report with its building's name.
type ArchiveItem struct {
	Report       model.Report
	BuildingName string
}

// Archive is R275: the page of reports in scope and the whole match's count.
func (q Requests) Archive(ctx context.Context, sc store.Scope, f store.ReportFilter) ([]ArchiveItem, int64, error) {
	list, err := q.Reports.List(ctx, sc, f)
	if err != nil {
		return nil, 0, err
	}
	count := f
	count.Page = store.Page{}
	total, err := q.Reports.Count(ctx, sc, count)
	if err != nil {
		return nil, 0, err
	}
	names := map[uuid.UUID]string{}
	out := make([]ArchiveItem, 0, len(list))
	for _, rp := range list {
		name, ok := names[rp.BuildingID]
		if !ok {
			b, err := q.Service.d.Buildings.Get(ctx, sc, rp.BuildingID)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return nil, 0, err
			}
			name = b.Name
			names[rp.BuildingID] = name
		}
		out = append(out, ArchiveItem{Report: rp, BuildingName: name})
	}
	return out, total, nil
}

// Get is one report in scope with its building's name.
func (q Requests) Get(ctx context.Context, sc store.Scope, id uuid.UUID) (ArchiveItem, error) {
	rp, err := q.Reports.Get(ctx, sc, id)
	if err != nil {
		return ArchiveItem{}, err
	}
	b, err := q.Service.d.Buildings.Get(ctx, sc, rp.BuildingID)
	if err != nil {
		return ArchiveItem{}, err
	}
	return ArchiveItem{Report: rp, BuildingName: b.Name}, nil
}

// File is a report's PDF or workbook in the reader's locale (R265).
func (q Requests) File(ctx context.Context, sc store.Scope, id uuid.UUID, format, locale string) ([]byte, string, error) {
	rp, err := q.Reports.Get(ctx, sc, id)
	if err != nil {
		return nil, "", err
	}
	raw, err := q.Files.Read(ctx, rp, format, locale)
	if err != nil {
		return nil, "", err
	}
	return raw, FileName(rp, format), nil
}
