// Package jobs answers "what happened to the task I enqueued?" for the three
// user-triggered background jobs the screens start (R192). It reads asynq's
// own task state; nothing here writes.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

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
}

// Inspector is the slice of asynq.Inspector this service uses.
type Inspector interface {
	GetTaskInfo(queue, id string) (*asynq.TaskInfo, error)
}

// Deps is what New needs.
type Deps struct {
	Inspector Inspector
	Analyzers store.AnalyzerRepository
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
}

// payloadScope is the part of every watchable payload this service reads.
type payloadScope struct {
	CompanyID  uuid.UUID
	AnalyzerID uuid.UUID
}

// Get returns the job's state, or store.ErrNotFound when it does not exist,
// is not watchable, or belongs to another company or another building's
// analyzer — one indistinguishable answer, so an id is never an oracle.
func (s *Service) Get(ctx context.Context, sc store.Scope, id string) (View, error) {
	if !sc.Valid() {
		return View{}, store.ErrInvalidScope
	}
	info, err := s.find(id)
	if err != nil {
		return View{}, err
	}
	if !watchable[info.Type] {
		return View{}, store.ErrNotFound
	}
	var p payloadScope
	if err := json.Unmarshal(info.Payload, &p); err != nil || p.CompanyID != sc.CompanyID {
		return View{}, store.ErrNotFound
	}
	if p.AnalyzerID != uuid.Nil {
		if _, err := s.d.Analyzers.Get(ctx, sc, p.AnalyzerID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return View{}, store.ErrNotFound
			}
			return View{}, err
		}
	}
	v := View{ID: info.ID, Type: info.Type, Status: statusOf(info.State)}
	if !info.CompletedAt.IsZero() {
		completed := info.CompletedAt
		v.CompletedAt = &completed
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
