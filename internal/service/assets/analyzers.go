package assets

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Active is R163: the last reading is within ActiveWindow of now.
func Active(a model.Analyzer, now time.Time) bool {
	return a.LastReadingAt != nil && !a.LastReadingAt.Before(now.Add(-ActiveWindow))
}

// AnalyzerListInput narrows GET /analyzers.
type AnalyzerListInput struct {
	BuildingID *uuid.UUID
	Unassigned bool
	Providers  []model.IntegrationProvider
	IsActive   *bool
	Q          string
	Page       store.Page
}

// ListAnalyzers lists analyzers in scope. A text query filters installation
// number, customer name and meter number in memory before paging.
func (s *Service) ListAnalyzers(ctx context.Context, sc store.Scope, in AnalyzerListInput) ([]model.Analyzer, error) {
	f := store.AnalyzerFilter{BuildingID: in.BuildingID, Unassigned: in.Unassigned, Providers: in.Providers, IsActive: in.IsActive}
	if in.Q == "" {
		f.Page = in.Page
		return s.d.Analyzers.List(ctx, sc, f)
	}
	all, err := listAll(func(p store.Page) ([]model.Analyzer, error) {
		f.Page = p
		return s.d.Analyzers.List(ctx, sc, f)
	})
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(in.Q)
	var matched []model.Analyzer
	for _, a := range all {
		fields := []string{a.InstallationNumber, deref(a.CustomerName), deref(a.MeterNumber)}
		for _, v := range fields {
			if strings.Contains(strings.ToLower(v), q) {
				matched = append(matched, a)
				break
			}
		}
	}
	start := min(int(in.Page.Offset), len(matched))
	end := min(start+int(in.Page.Limit), len(matched))
	return matched[start:end], nil
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// GetAnalyzer returns one analyzer.
func (s *Service) GetAnalyzer(ctx context.Context, sc store.Scope, id uuid.UUID) (model.Analyzer, error) {
	return s.d.Analyzers.Get(ctx, sc, id)
}

// AnalyzerInput is PATCH /analyzers/{id}: only the assignable fields (05 §4).
type AnalyzerInput struct {
	BuildingID          *uuid.UUID
	UnassignBuilding    bool
	MeterMultiplier     *decimal.Decimal
	InstalledPowerKw    *decimal.Decimal
	Latitude, Longitude *decimal.Decimal
}

// UpdateAnalyzer changes the assignable fields; a building outside the scope is not found.
func (s *Service) UpdateAnalyzer(ctx context.Context, sc store.Scope, id uuid.UUID, in AnalyzerInput) (model.Analyzer, error) {
	a, err := s.d.Analyzers.Get(ctx, sc, id)
	if err != nil {
		return model.Analyzer{}, err
	}
	if in.BuildingID != nil {
		if _, err := s.d.Buildings.Get(ctx, sc, *in.BuildingID); err != nil {
			return model.Analyzer{}, err
		}
		a.BuildingID = in.BuildingID
	}
	if in.UnassignBuilding {
		a.BuildingID = nil
	}
	if in.MeterMultiplier != nil {
		if !in.MeterMultiplier.IsPositive() {
			return model.Analyzer{}, validation("meter_multiplier", "gt")
		}
		a.MeterMultiplier = *in.MeterMultiplier
	}
	if in.InstalledPowerKw != nil {
		a.InstalledPowerKw = in.InstalledPowerKw
	}
	if in.Latitude != nil {
		a.Latitude = in.Latitude
	}
	if in.Longitude != nil {
		a.Longitude = in.Longitude
	}
	a.UpdatedAt = s.d.Clock.Now()
	return s.d.Analyzers.Update(ctx, sc, a)
}

// RefreshAnalyzer enqueues an on-demand pull (R164) and returns the job id.
func (s *Service) RefreshAnalyzer(ctx context.Context, sc store.Scope, id uuid.UUID, mode string) (string, error) {
	a, err := s.d.Analyzers.Get(ctx, sc, id)
	if err != nil {
		return "", err
	}
	def, err := s.d.Integrations.Definition(ctx, sc, a.Provider, a.ProviderSubtype)
	if errors.Is(err, store.ErrNotFound) {
		return "", ErrIntegrationNotConfigured
	}
	if err != nil {
		return "", err
	}
	cred, err := s.d.Integrations.Credential(ctx, sc, def.ID)
	if errors.Is(err, store.ErrNotFound) {
		return "", ErrIntegrationNotConfigured
	}
	if err != nil {
		return "", err
	}
	task, err := job.NewRefreshAnalyzerTask(job.RefreshAnalyzerPayload{
		CompanyID: sc.CompanyID, CredentialID: cred.ID, AnalyzerID: a.ID, Mode: mode,
	}, job.TaskOptions{MaxRetry: s.d.MaxRetry})
	if err != nil {
		return "", validation("mode", "oneof")
	}
	info, err := s.d.Enqueuer.Enqueue(ctx, task)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return "", ErrRefreshInProgress
	}
	if err != nil {
		return "", err
	}
	return info.ID, nil
}
