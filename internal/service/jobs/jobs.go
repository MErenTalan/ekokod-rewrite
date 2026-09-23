// Package jobs answers "what happened to the task I enqueued?" for the three
// user-triggered background jobs the screens start (R192). It reads asynq's
// own task state; nothing here writes.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Status is the coarse state a screen shows.
type Status string

// The four statuses (R192).
const (
	Queued    Status = "queued"
	Running   Status = "running"
	Succeeded Status = "succeeded"
	Failed    Status = "failed"
)

// View is one job as the API returns it. The provider's own error text is
// never carried: it may quote a provider response (R177's spirit).
type View struct {
	ID          string
	Type        string
	Status      Status
	CompletedAt *time.Time
	// ErrorCode is one of billing's R113 codes when a bill computation
	// failed, and "" otherwise. It is a closed set, so it can be shown to an
	// operator; the worker's own error text still never leaves (R237).
	ErrorCode string
}

// Inspector is the slice of asynq.Inspector this service uses.
type Inspector interface {
	GetTaskInfo(queue, id string) (*asynq.TaskInfo, error)
}

// Deps is what New needs.
type Deps struct {
	Inspector Inspector
	Analyzers store.AnalyzerRepository
	// Buildings resolves the subject of a building-scope bill computation.
	Buildings store.BuildingRepository
	// Ops reads the job_runs row that explains a failure (R237).
	Ops store.OpsRepository
	// Reports resolves the report a delivery sends (R267).
	Reports store.ReportRepository
	// Plants resolves a plant sync's plant (R288).
	Plants store.PlantRepository
	// Queues are searched in order; default: critical, default, low.
	Queues []string
}

// Service reads job state.
type Service struct{ d Deps }

// New validates deps.
func New(d Deps) (*Service, error) {
	if d.Inspector == nil || d.Analyzers == nil {
		return nil, errors.New("jobs: Inspector and Analyzers are required")
	}
	if len(d.Queues) == 0 {
		d.Queues = []string{job.QueueCritical, job.QueueDefault, job.QueueLow}
	}
	return &Service{d: d}, nil
}

// watchable is the allow-list: only the jobs a screen can start are
// observable, so an id from anywhere else answers exactly like an unknown one.
var watchable = map[string]bool{
	job.TypeIntegrationRefreshAnalyzer: true,
	job.TypeIntegrationSyncAnalyzers:   true,
	job.TypeIntegrationBackfill:        true,
	// R237: the bills screen enqueues this one and must be able to say why it
	// failed — §7.10 lists the sentences.
	job.TypeBillingGenerate: true,
	// R267: the reports screen watches generation and delivery.
	job.TypeReportGenerate: true,
	job.TypeReportDeliver:  true,
	// R288: the solar screen's "update data" watches its plant sync.
	job.TypeSolarSyncPlant: true,
}

// billingCodes is R113's closed set. A code outside it is not repeated to a
// caller: the point of the field is that everything in it is safe to show.
var closedCodes = map[string]bool{
	"tariff_not_found": true, "no_consumption_data": true, "unresolved_anomaly": true,
	"period_not_closed": true, "ptf_data_missing": true,
	"billing_parameters_missing": true, "billing_parameters_invalid": true,
	// The reports' own (R266, R267).
	"report_building_not_found": true, "report_not_ready": true,
	"smtp_not_configured": true, "delivery_failed": true,
	// The iSolar sync's (R288).
	"isolar_auth": true, "isolar_unavailable": true, "isolar_not_linked": true, "credential_missing": true,
}

// payloadScope is the part of every watchable payload this service reads.
// Backfill names its analyzers in the plural, the other two in the singular.
type payloadScope struct {
	CompanyID   uuid.UUID
	AnalyzerID  uuid.UUID
	AnalyzerIDs []uuid.UUID
}

// analyzers is every analyzer the job touches. Empty means the job covers the
// whole company (a credential sync, a backfill over everything it finds).
func (p payloadScope) analyzers() []uuid.UUID {
	if p.AnalyzerID != uuid.Nil {
		return append([]uuid.UUID{p.AnalyzerID}, p.AnalyzerIDs...)
	}
	return p.AnalyzerIDs
}

// Get returns the job's state, or store.ErrNotFound when it does not exist,
// is not watchable, belongs to another company, touches an analyzer outside
// the scope, or covers the whole company while the caller sees only some of
// it — one indistinguishable answer, so an id is never an oracle.
func (s *Service) Get(ctx context.Context, sc store.Scope, id string) (View, error) {
	if !sc.Valid() {
		return View{}, store.ErrInvalidScope
	}
	info, err := s.find(id)
	if errors.Is(err, store.ErrNotFound) {
		return s.fromRun(ctx, sc, id)
	}
	if err != nil {
		return View{}, err
	}
	if !watchable[info.Type] {
		return View{}, store.ErrNotFound
	}
	// The payload families are decoded separately: the integration tasks
	// carry bare Go field names, billing and report tasks snake_case tags,
	// and one struct cannot answer to both spellings.
	if err := s.visible(ctx, sc, info); err != nil {
		return View{}, err
	}
	v := View{ID: info.ID, Type: info.Type, Status: statusOf(info.State)}
	if !info.CompletedAt.IsZero() {
		completed := info.CompletedAt
		v.CompletedAt = &completed
	}
	if v.Status == Failed {
		if id, ok := runTaskID(info); ok {
			v.ErrorCode = s.failureCode(ctx, sc, id)
		}
	}
	return v, nil
}

// runTaskID is the task id the worker wrote on its job_runs row, derived
// from the payload the same way the worker derives it.
func runTaskID(info *asynq.TaskInfo) (string, bool) {
	switch info.Type {
	case job.TypeBillingGenerate:
		var p job.BillingGeneratePayload
		err := json.Unmarshal(info.Payload, &p)
		return job.BillingGenerateTaskID(p), err == nil
	case job.TypeReportGenerate:
		var p job.ReportGeneratePayload
		err := json.Unmarshal(info.Payload, &p)
		return job.ReportGenerateTaskID(p), err == nil
	case job.TypeReportDeliver:
		var p job.ReportDeliverPayload
		err := json.Unmarshal(info.Payload, &p)
		return job.ReportDeliverTaskID(p), err == nil
	case job.TypeSolarSyncPlant:
		var p job.SolarSyncPayload
		err := json.Unmarshal(info.Payload, &p)
		return job.SolarSyncTaskID(p), err == nil
	}
	return "", false
}

// visible answers ErrNotFound for anything the caller may not watch.
func (s *Service) visible(ctx context.Context, sc store.Scope, info *asynq.TaskInfo) error {
	switch info.Type {
	case job.TypeBillingGenerate:
		var p job.BillingGeneratePayload
		if err := json.Unmarshal(info.Payload, &p); err != nil || p.CompanyID != sc.CompanyID {
			return store.ErrNotFound
		}
		return s.billingSubjectVisible(ctx, sc, p)
	case job.TypeReportGenerate:
		var p job.ReportGeneratePayload
		if err := json.Unmarshal(info.Payload, &p); err != nil || p.CompanyID != sc.CompanyID {
			return store.ErrNotFound
		}
		return s.buildingVisible(ctx, sc, p.BuildingID)
	case job.TypeReportDeliver:
		var p job.ReportDeliverPayload
		if err := json.Unmarshal(info.Payload, &p); err != nil || p.CompanyID != sc.CompanyID {
			return store.ErrNotFound
		}
		return s.reportVisible(ctx, sc, p.ReportID)
	case job.TypeSolarSyncPlant:
		var p job.SolarSyncPayload
		if err := json.Unmarshal(info.Payload, &p); err != nil || p.CompanyID != sc.CompanyID {
			return store.ErrNotFound
		}
		return s.plantVisible(ctx, sc, p.PlantID)
	}
	var p payloadScope
	if err := json.Unmarshal(info.Payload, &p); err != nil || p.CompanyID != sc.CompanyID {
		return store.ErrNotFound
	}
	ids := p.analyzers()
	// A job that names no analyzer covers the whole company, so only a
	// principal who may see the whole company may watch it.
	if len(ids) == 0 && !sc.AllBuildings {
		return store.ErrNotFound
	}
	for _, id := range ids {
		if _, err := s.d.Analyzers.Get(ctx, sc, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) buildingVisible(ctx context.Context, sc store.Scope, id uuid.UUID) error {
	if s.d.Buildings == nil {
		return store.ErrNotFound
	}
	_, err := s.d.Buildings.Get(ctx, sc, id)
	return err
}

// plantVisible: plants are company-level, so a building-scoped caller never sees one.
func (s *Service) plantVisible(ctx context.Context, sc store.Scope, id uuid.UUID) error {
	if s.d.Plants == nil || !sc.AllBuildings {
		return store.ErrNotFound
	}
	_, err := s.d.Plants.Get(ctx, sc, id)
	return err
}

func (s *Service) reportVisible(ctx context.Context, sc store.Scope, id uuid.UUID) error {
	if s.d.Reports == nil {
		return store.ErrNotFound
	}
	_, err := s.d.Reports.Get(ctx, sc, id)
	return err
}

// billingSubjectVisible answers exactly like an unknown id for a subject the
// caller may not see, so a job id is never an oracle.
func (s *Service) billingSubjectVisible(ctx context.Context, sc store.Scope, p job.BillingGeneratePayload) error {
	switch p.Scope {
	case model.BillScopeCompany:
		if !sc.AllBuildings {
			return store.ErrNotFound
		}
		return nil
	case model.BillScopeBuilding:
		if s.d.Buildings == nil {
			return store.ErrNotFound
		}
		_, err := s.d.Buildings.Get(ctx, sc, p.SubjectID)
		if errors.Is(err, store.ErrNotFound) {
			return store.ErrNotFound
		}
		return err
	case model.BillScopeAnalyzer:
		_, err := s.d.Analyzers.Get(ctx, sc, p.SubjectID)
		if errors.Is(err, store.ErrNotFound) {
			return store.ErrNotFound
		}
		return err
	default:
		return store.ErrNotFound
	}
}

// failureCode reads the run the worker wrote and returns its closed code,
// or "" — never the run's error text (R192).
func (s *Service) failureCode(ctx context.Context, sc store.Scope, taskID string) string {
	if s.d.Ops == nil {
		return ""
	}
	run, err := s.d.Ops.RunByTaskID(ctx, sc, taskID)
	if err != nil {
		return ""
	}
	return closedCode(run.Detail)
}

// closedCode returns the run's code only when it is in the closed set.
func closedCode(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var detail struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(raw, &detail); err != nil || !closedCodes[detail.Code] {
		return ""
	}
	return detail.Code
}

// goneVisible re-derives visibility from the deterministic id of a watchable
// task whose worker records job_runs.task_id. It exists because such tasks
// carry no asynq.Retention (R53): once finished, the id is all that is left.
var goneVisible = map[string]func(s *Service, ctx context.Context, sc store.Scope, parts []string) (string, error){
	// billing.generate:<scope>:<subject>:<period>
	job.TypeBillingGenerate: func(s *Service, ctx context.Context, sc store.Scope, parts []string) (string, error) {
		if len(parts) != 4 {
			return "", store.ErrNotFound
		}
		subject, err := uuid.Parse(parts[2])
		if err != nil {
			return "", store.ErrNotFound
		}
		p := job.BillingGeneratePayload{CompanyID: sc.CompanyID, Scope: model.BillScope(parts[1]), SubjectID: subject, PeriodKey: parts[3]}
		return job.TypeBillingGenerate, s.billingSubjectVisible(ctx, sc, p)
	},
	// report.generate:<building>:<type>:<period>
	job.TypeReportGenerate: func(s *Service, ctx context.Context, sc store.Scope, parts []string) (string, error) {
		building, err := uuid.Parse(partAt(parts, 1, 4))
		if err != nil {
			return "", store.ErrNotFound
		}
		return job.TypeReportGenerate, s.buildingVisible(ctx, sc, building)
	},
	// report.deliver:<report>:<request>
	job.TypeReportDeliver: func(s *Service, ctx context.Context, sc store.Scope, parts []string) (string, error) {
		report, err := uuid.Parse(partAt(parts, 1, 3))
		if err != nil {
			return "", store.ErrNotFound
		}
		return job.TypeReportDeliver, s.reportVisible(ctx, sc, report)
	},
	// isolar.sync_plant:<plant>:<backfill_days>
	job.TypeSolarSyncPlant: func(s *Service, ctx context.Context, sc store.Scope, parts []string) (string, error) {
		plant, err := uuid.Parse(partAt(parts, 1, 3))
		if err != nil {
			return "", store.ErrNotFound
		}
		return job.TypeSolarSyncPlant, s.plantVisible(ctx, sc, plant)
	},
}

// partAt is parts[i] when the id has exactly n parts, else "".
func partAt(parts []string, i, n int) string {
	if len(parts) != n {
		return ""
	}
	return parts[i]
}

// fromRun answers for a finished task from its newest job_runs row (R267).
// Visibility is checked first, and the run lookup is company-scoped, so the
// answer for a foreign or invisible id stays the plain 404.
func (s *Service) fromRun(ctx context.Context, sc store.Scope, id string) (View, error) {
	parts := strings.Split(id, ":")
	visible, ok := goneVisible[parts[0]]
	if !ok || s.d.Ops == nil {
		return View{}, store.ErrNotFound
	}
	jobType, err := visible(s, ctx, sc, parts)
	if err != nil {
		return View{}, err
	}
	run, err := s.d.Ops.RunByTaskID(ctx, sc, id)
	if err != nil {
		return View{}, err
	}
	v := View{ID: id, Type: jobType, Status: Running, CompletedAt: run.FinishedAt}
	switch run.Status {
	case "success":
		v.Status = Succeeded
	case "failed":
		v.Status = Failed
		v.ErrorCode = closedCode(run.Detail)
	}
	return v, nil
}

func (s *Service) find(id string) (*asynq.TaskInfo, error) {
	for _, queue := range s.d.Queues {
		info, err := s.d.Inspector.GetTaskInfo(queue, id)
		switch {
		case err == nil:
			return info, nil
		case errors.Is(err, asynq.ErrTaskNotFound), errors.Is(err, asynq.ErrQueueNotFound):
			continue
		default:
			return nil, fmt.Errorf("jobs: inspect %s: %w", queue, err)
		}
	}
	return nil, store.ErrNotFound
}

func statusOf(state asynq.TaskState) Status {
	switch state {
	case asynq.TaskStateActive, asynq.TaskStateRetry:
		return Running
	case asynq.TaskStateCompleted:
		return Succeeded
	case asynq.TaskStateArchived:
		return Failed
	default: // pending, scheduled, aggregating
		return Queued
	}
}
