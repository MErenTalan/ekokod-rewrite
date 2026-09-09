package job

import (
	"context"
	"fmt"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/hibiken/asynq"
)

// Client enqueues tasks.
type Client struct{ inner *asynq.Client }

// NewClient opens a queue client.
func NewClient(cfg config.Redis) (*Client, error) {
	opt, err := RedisOpt(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{inner: asynq.NewClient(opt)}, nil
}

// Enqueue schedules a task for execution.
func (c *Client) Enqueue(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	info, err := c.inner.EnqueueContext(ctx, task, opts...)
	if err != nil {
		return nil, fmt.Errorf("enqueue %s: %w", task.Type(), err)
	}
	return info, nil
}

// Close releases the client's connections.
func (c *Client) Close() error { return c.inner.Close() }
