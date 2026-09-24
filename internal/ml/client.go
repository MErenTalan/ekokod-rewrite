// Package ml is the Go side of the ML service contract (03 §6.3, F13b R370):
// a small HTTP client that turns every way the service can be missing into
// ErrUnavailable, so callers degrade instead of failing.
package ml

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	// ErrUnavailable means the service could not be reached or did not answer usefully.
	ErrUnavailable = errors.New("ml: service unavailable")
	// ErrRejected means the service refused the request (422): a caller bug, not an outage.
	ErrRejected = errors.New("ml: request rejected")
)

const maxBody = 8 << 20

// Client calls the ML service. The zero value is not usable; use New.
type Client struct {
	base    string
	key     string
	http    *http.Client
	backoff []time.Duration
}

// Option adjusts a Client.
type Option func(*Client)

// WithBackoff sets the waits before the two retries (tests use tiny ones).
func WithBackoff(first, second time.Duration) Option {
	return func(c *Client) { c.backoff = []time.Duration{first, second} }
}

// New builds a client; an empty base URL yields a client that is always unavailable.
func New(baseURL, apiKey string, timeout time.Duration, opts ...Option) *Client {
	c := &Client{
		base:    strings.TrimRight(baseURL, "/"),
		key:     apiKey,
		http:    &http.Client{Timeout: timeout},
		backoff: []time.Duration{250 * time.Millisecond, 750 * time.Millisecond},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Forecast is POST /v1/forecast.
func (c *Client) Forecast(ctx context.Context, req ForecastRequest) (ForecastResponse, error) {
	var out ForecastResponse
	return out, c.call(ctx, http.MethodPost, "/v1/forecast", req, &out)
}

// Anomaly is POST /v1/anomaly.
func (c *Client) Anomaly(ctx context.Context, req AnomalyRequest) (AnomalyResponse, error) {
	var out AnomalyResponse
	return out, c.call(ctx, http.MethodPost, "/v1/anomaly", req, &out)
}

// Models is GET /v1/models.
func (c *Client) Models(ctx context.Context) ([]Model, error) {
	var out struct {
		Items []Model `json:"items"`
	}
	err := c.call(ctx, http.MethodGet, "/v1/models", nil, &out)
	return out.Items, err
}

func retryable(status int) bool {
	return status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

func (c *Client) call(ctx context.Context, method, path string, in, out any) error {
	if c.base == "" {
		return fmt.Errorf("%w: EKOKOD_ML_URL is not set", ErrUnavailable)
	}
	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			return fmt.Errorf("ml: encode %s: %w", path, err)
		}
	}
	var last error
	for attempt := 0; attempt <= len(c.backoff); attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("%w: %w", ErrUnavailable, ctx.Err())
			case <-time.After(c.backoff[attempt-1]):
			}
		}
		status, data, err := c.once(ctx, method, path, body)
		switch {
		case err != nil:
			last = err // network error or timeout: retry
			continue
		case status == http.StatusUnprocessableEntity:
			return fmt.Errorf("%w: %s", ErrRejected, bytes.TrimSpace(data))
		case retryable(status):
			last = fmt.Errorf("status %d", status)
			continue
		case status < 200 || status > 299:
			return fmt.Errorf("%w: %s %s: status %d", ErrUnavailable, method, path, status)
		}
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("%w: decode %s: %w", ErrUnavailable, path, err)
		}
		return nil
	}
	return fmt.Errorf("%w: %s %s: %w", ErrUnavailable, method, path, last)
}

func (c *Client) once(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = res.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(res.Body, maxBody))
	return res.StatusCode, data, err
}
