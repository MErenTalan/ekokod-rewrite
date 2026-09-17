package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// TypeIntegrationRefreshAnalyzer is the on-demand pull for one analyzer (R164).
const TypeIntegrationRefreshAnalyzer = "integration.refresh_analyzer"

// Refresh modes (01 §7.2 "Refresh hourly values" / "Refresh energy values").
const (
	RefreshModeHourly = "hourly"
	RefreshModeEnergy = "energy"
)

// RefreshAnalyzerPayload asks for one analyzer's readings to be pulled now.
type RefreshAnalyzerPayload struct {
	CompanyID, CredentialID, AnalyzerID uuid.UUID
	Mode                                string
}

// NewRefreshAnalyzerTask builds the task; one refresh per analyzer and mode per minute.
func NewRefreshAnalyzerTask(p RefreshAnalyzerPayload, o TaskOptions) (*asynq.Task, error) {
	if p.Mode != RefreshModeHourly && p.Mode != RefreshModeEnergy {
		return nil, fmt.Errorf("job: unknown refresh mode %q", p.Mode)
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	opts := append([]asynq.Option{asynq.Timeout(5 * time.Minute), asynq.Unique(time.Minute)}, integMaxRetryOptions(o)...)
	return asynq.NewTask(TypeIntegrationRefreshAnalyzer, raw, opts...), nil
}

// DecodeRefreshAnalyzer reads the payload.
func DecodeRefreshAnalyzer(t *asynq.Task) (RefreshAnalyzerPayload, error) {
	var p RefreshAnalyzerPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return p, fmt.Errorf("%w: %w", asynq.SkipRetry, err)
	}
	if p.CompanyID == uuid.Nil || p.AnalyzerID == uuid.Nil || p.CredentialID == uuid.Nil {
		return p, fmt.Errorf("%w: incomplete refresh payload", asynq.SkipRetry)
	}
	return p, nil
}

// AnalyzerRefresher fans an on-demand refresh out into fetch tasks.
type AnalyzerRefresher interface {
	RefreshAnalyzer(ctx context.Context, p RefreshAnalyzerPayload) error
}

func (h *Handlers) handleRefreshAnalyzer(ctx context.Context, task *asynq.Task) error {
	p, err := DecodeRefreshAnalyzer(task)
	if err != nil {
		return err
	}
	if err := h.AnalyzerRefresh.RefreshAnalyzer(ctx, p); err != nil && !errors.Is(err, asynq.SkipRetry) {
		return ClassifyForRetry(err)
	}
	return nil
}
