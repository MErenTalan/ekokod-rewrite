//go:build integration

package mw_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func redisClient(t *testing.T) *goredis.Client {
	t.Helper()
	cfg := testfixtures.SharedRedisConfig(t)
	opts, err := goredis.ParseURL(cfg.URL)
	require.NoError(t, err)
	c := goredis.NewClient(opts)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestAuthLimitPerIPAndEmail(t *testing.T) {
	limiter := mw.AuthLimiter{
		Redis: redisClient(t), Limit: config.RateLimit{Limit: 5, Window: 15 * time.Minute},
		ClientIP: func(r *http.Request) string { return r.Header.Get("X-Test-IP") }, Prefix: "test:" + uuid.NewString() + ":",
	}
	var bodies []string
	h := limiter.Middleware("auth.login", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		w.WriteHeader(http.StatusUnauthorized)
	}))
	attempt := func(ip, email string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(`{"email":"`+email+`","password":"x"}`))
		req.Header.Set("X-Test-IP", ip)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	for range 5 {
		require.Equal(t, 401, attempt("1.1.1.1", "ali@example.com").Code)
	}
	rec := attempt("1.1.1.1", "ALI@example.com")
	require.Equal(t, 429, rec.Code, "e-mail is case-insensitive in the key")
	retry, err := strconv.Atoi(rec.Header().Get("Retry-After"))
	require.NoError(t, err)
	require.Greater(t, retry, 800)
	require.Equal(t, `{"email":"ali@example.com","password":"x"}`, bodies[0], "the body reaches the handler intact")
	require.Equal(t, 401, attempt("1.1.1.1", "veli@example.com").Code)
	require.Equal(t, 401, attempt("2.2.2.2", "ali@example.com").Code)
}

func TestIdempotencyReplayMismatchInProgress(t *testing.T) {
	idem := mw.Idempotency{Redis: redisClient(t), Prefix: "test:" + uuid.NewString() + ":"}
	p := principal(model.UserRoleCompanyAdmin)
	var calls atomic.Int32
	status := http.StatusCreated
	release := make(chan struct{})
	h := idem.Middleware("buildings.create")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if r.Header.Get("X-Block") != "" {
			<-release
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"call":`+strconv.Itoa(int(n))+`}`)
	}))
	send := func(key, body string, extra ...string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(mw.WithPrincipal(context.Background(), p), http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Idempotency-Key", key)
		if len(extra) > 0 {
			req.Header.Set("X-Block", "1")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	first := send("k1", `{"name":"a"}`)
	require.Equal(t, 201, first.Code)
	again := send("k1", `{"name":"a"}`)
	require.Equal(t, 201, again.Code)
	require.Equal(t, first.Body.String(), again.Body.String())
	require.Equal(t, "true", again.Header().Get("Idempotent-Replay"))
	require.Equal(t, int32(1), calls.Load())

	mismatch := send("k1", `{"name":"b"}`)
	require.Equal(t, 422, mismatch.Code)
	require.Equal(t, "idempotency_key_mismatch", code(t, mismatch))

	status = http.StatusInternalServerError
	require.Equal(t, 500, send("k2", `{}`).Code)
	status = http.StatusCreated
	require.Equal(t, 201, send("k2", `{}`).Code, "a 5xx is not stored, so a retry runs again")
	require.Equal(t, int32(3), calls.Load())

	done := make(chan *httptest.ResponseRecorder)
	go func() { done <- send("k3", `{}`, "block") }()
	require.Eventually(t, func() bool { return calls.Load() == 4 }, 5*time.Second, 10*time.Millisecond)
	busy := send("k3", `{}`)
	require.Equal(t, 409, busy.Code)
	require.Equal(t, "idempotency_in_progress", code(t, busy))
	close(release)
	require.Equal(t, 201, (<-done).Code)

	other := principal(model.UserRoleCompanyAdmin)
	req := httptest.NewRequestWithContext(mw.WithPrincipal(context.Background(), other), http.MethodPost, "/", strings.NewReader(`{"name":"a"}`))
	req.Header.Set("Idempotency-Key", "k1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Empty(t, rec.Header().Get("Idempotent-Replay"), "keys are per user")
}
