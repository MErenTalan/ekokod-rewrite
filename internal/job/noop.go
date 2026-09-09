package job

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
)

// NoopPayload is the payload of the system.noop task, which exists to prove the
// enqueue → execute round trip end to end.
type NoopPayload struct {
	Message string `json:"message"`
}

// NewNoopTask builds a system.noop task.
func NewNoopTask(message string) (*asynq.Task, error) {
	payload, err := json.Marshal(NoopPayload{Message: message})
	if err != nil {
		return nil, fmt.Errorf("encode noop payload: %w", err)
	}
	return asynq.NewTask(TypeNoop, payload, asynq.Queue(QueueLow)), nil
}

// DecodeNoop reads a system.noop payload.
func DecodeNoop(task *asynq.Task) (NoopPayload, error) {
	var payload NoopPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return NoopPayload{}, fmt.Errorf("decode noop payload: %w", err)
	}
	return payload, nil
}

// Noop handles system.noop.
func (h *Handlers) Noop(ctx context.Context, task *asynq.Task) error {
	payload, err := DecodeNoop(task)
	if err != nil {
		return err
	}
	h.Log.InfoContext(ctx, "noop task executed", slog.String("message", payload.Message))
	return nil
}
