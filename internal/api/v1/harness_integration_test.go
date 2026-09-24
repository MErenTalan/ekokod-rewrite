//go:build integration

package v1_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/MErenTalan/ekokod-rewrite/internal/api"
	"github.com/MErenTalan/ekokod-rewrite/internal/apiwire"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/buildinfo"
	"github.com/MErenTalan/ekokod-rewrite/internal/credentials"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/mail"
	"github.com/MErenTalan/ekokod-rewrite/internal/ml"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

const (
	testPassword = "Guvenli!Sifre-42"
	uaChrome     = "Mozilla/5.0 (X11; Linux x86_64) Chrome/140"
	uaSafari     = "Mozilla/5.0 (Macintosh) Safari/18"
)

type capturedMail struct {
	mu   sync.Mutex
	msgs []mail.Message
	fail error
}

func (c *capturedMail) Send(_ context.Context, _ model.SMTPSettings, password []byte, m mail.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail != nil {
		return fmt.Errorf("%w (password %s)", c.fail, password)
	}
	c.msgs = append(c.msgs, m)
	return nil
}

// recordingEnqueuer records tasks and refuses a repeated (type, payload) like asynq.Unique.
type recordingEnqueuer struct {
	mu    sync.Mutex
	tasks []*asynq.Task
	seen  map[string]bool
}

func (e *recordingEnqueuer) Enqueue(_ context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := task.Type() + string(task.Payload())
	if e.seen[key] {
		return nil, asynq.ErrDuplicateTask
	}
	e.seen[key] = true
	e.tasks = append(e.tasks, task)
	return &asynq.TaskInfo{ID: fmt.Sprintf("job-%d", len(e.tasks)), Type: task.Type()}, nil
}

func (e *recordingEnqueuer) snapshot() []*asynq.Task {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]*asynq.Task(nil), e.tasks...)
}

// fakeProviders answers every verification by the credential's username:
// "config-broken" → ErrConfig, "auth-broken" → ErrAuth, anything else succeeds.
type fakeProviders struct {
	exchanged sync.Map
}

func (f *fakeProviders) Verifier(integration.Provider) (credentials.Verifier, error) { return f, nil }

func (f *fakeProviders) Verify(_ context.Context, creds integration.Credentials) error {
	switch creds.Username {
	case "config-broken":
		return &integration.Error{Kind: integration.ErrConfig, Provider: creds.Provider, Op: "verify"}
	case "auth-broken":
		return &integration.Error{Kind: integration.ErrAuth, Provider: creds.Provider, Op: "verify", HTTPStatus: 401}
	}
	return nil
}

func (f *fakeProviders) AuthorizeURL(_ integration.Credentials, redirectURI string) (string, error) {
	return "https://web3.isolarcloud.example/#/authorized-app?redirectUrl=" + url.QueryEscape(redirectURI), nil
}

func (f *fakeProviders) ExchangeCode(_ context.Context, _ integration.Credentials, code, _ string) (isolar.Token, error) {
	f.exchanged.Store(code, true)
	return isolar.Token{AccessToken: integration.NewSecret([]byte("isolar-access-" + code)), ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func (f *fakeProviders) Refresh(context.Context, integration.Credentials) (isolar.Token, error) {
	return isolar.Token{}, errors.New("not used")
}

type harness struct {
	providers *fakeProviders
	enq       *recordingEnqueuer
	t         *testing.T
	srv       *httptest.Server
	clock     *clock.Fake
	mail      *capturedMail
	ml        *fakeMLService
	fx        seed.Fixtures
	pool      *pgxpool.Pool
	cfg       *config.Config
	hasher    auth.Hasher
}

func newHarness(t *testing.T, tune ...func(*config.Config)) *harness {
	t.Helper()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	cfg := &config.Config{
		HTTP: config.HTTP{
			PublicURL:     "https://app.example",
			RateLimitAPI:  config.RateLimit{Limit: 100000, Window: time.Minute},
			RateLimitAuth: config.RateLimit{Limit: 1000, Window: 15 * time.Minute},
		},
		Redis:   testfixtures.SharedRedisConfig(t),
		Storage: config.Storage{Root: t.TempDir(), UploadMax: 31457280, AllowedTypes: config.DefaultUploadTypes()},
		Security: config.Security{
			EncryptionKey:           []byte("0123456789abcdef0123456789abcdef"),
			JWTSigningKey:           []byte("jwt-signing-key-0123456789abcdef-0123"),
			PasswordPepper:          []byte("password-pepper-0123456789abcdef-012"),
			DeviceFingerprintSecret: []byte("device-fingerprint-0123456789abcdef-0"),
			AccessTokenTTL:          15 * time.Minute, RefreshTokenTTL: 24 * time.Hour, RefreshTokenRememberTTL: 720 * time.Hour,
			BcryptCost: bcrypt.MinCost, PasswordHistorySize: 5,
		},
	}
	for _, f := range tune {
		f(cfg)
	}
	h := &harness{ml: &fakeMLService{err: ml.ErrUnavailable}, providers: &fakeProviders{}, enq: &recordingEnqueuer{seen: map[string]bool{}}, t: t, clock: clock.NewFake(time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)), mail: &capturedMail{}, pool: pool, cfg: cfg,
		hasher: auth.Hasher{Pepper: cfg.Security.PasswordPepper, Cost: cfg.Security.BcryptCost}}
	fx, err := seed.E2EFixtures(ctx, pool, h.hasher, testPassword, h.clock.Now())
	require.NoError(t, err)
	h.fx = fx
	built, err := apiwire.Build(ctx, cfg, pool, testfixtures.DiscardLogger(), apiwire.Options{
		Clock: h.clock, Mail: h.mail, RedisPrefix: "test:" + uuid.NewString() + ":", Async: func(f func()) { f() }, Enqueuer: h.enq,
		Verifiers: h.providers, ISolar: h.providers, Solar: fakeSolar{}, ML: h.ml,
	})
	require.NoError(t, err)
	t.Cleanup(built.Close)
	h.srv = httptest.NewTLSServer(api.NewRouter(api.Deps{Cfg: cfg, Log: testfixtures.DiscardLogger(), Build: buildinfo.Get(), V1: built.V1}))
	t.Cleanup(h.srv.Close)
	return h
}

// client is one browser or app: its own cookie jar and User-Agent.
type client struct {
	h      *harness
	http   *http.Client
	ua     string
	bearer string
}

func (h *harness) client(ua string) *client {
	jar, err := cookiejar.New(nil)
	require.NoError(h.t, err)
	// The server's own client transport already trusts httptest's certificate.
	transport := h.srv.Client().Transport.(*http.Transport).Clone()
	return &client{h: h, ua: ua, http: &http.Client{Jar: jar, Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

type response struct {
	status int
	header http.Header
	body   []byte
}

func (r response) json(t *testing.T, v any) {
	t.Helper()
	require.NoError(t, json.Unmarshal(r.body, v), string(r.body))
}

func (r response) code(t *testing.T) string {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	r.json(t, &env)
	return env.Error.Code
}

func (c *client) do(method, path string, body any, headers ...string) response {
	c.h.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(c.h.t, err)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, c.h.srv.URL+"/api/v1"+path, reader)
	require.NoError(c.h.t, err)
	req.Header.Set("User-Agent", c.ua)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := c.http.Do(req)
	require.NoError(c.h.t, err)
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	require.NoError(c.h.t, err)
	return response{status: res.StatusCode, header: res.Header, body: raw}
}

func (c *client) login(email string, remember bool) response {
	c.h.t.Helper()
	res := c.do(http.MethodPost, "/auth/login", map[string]any{"email": email, "password": testPassword, "remember_me": remember})
	require.Equal(c.h.t, http.StatusOK, res.status, string(res.body))
	return res
}

func (c *client) cookie(name string) *http.Cookie {
	u, _ := url.Parse(c.h.srv.URL)
	for _, ck := range c.http.Jar.Cookies(u) {
		if ck.Name == name {
			return ck
		}
	}
	return nil
}

func setCookie(t *testing.T, res response, name string) string {
	t.Helper()
	for _, line := range res.header.Values("Set-Cookie") {
		if strings.HasPrefix(line, name+"=") {
			return line
		}
	}
	t.Fatalf("no Set-Cookie for %s in %v", name, res.header.Values("Set-Cookie"))
	return ""
}

// demoUser creates a demo-role user in company B for role-specific tests.
func (h *harness) demoUser() string {
	h.t.Helper()
	hash, err := h.hasher.Hash(testPassword)
	require.NoError(h.t, err)
	_, err = postgres.NewUserRepository(h.pool).Create(context.Background(), store.SystemScope(h.fx.CompanyB), model.User{
		CompanyID: h.fx.CompanyB, Name: "Demo", Email: "demo@b.e2e.ekokod.test", PasswordHash: hash,
		Role: model.UserRoleDemo, IsActive: true, Locale: "tr",
	})
	require.NoError(h.t, err)
	return "demo@b.e2e.ekokod.test"
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}

// as logs in a fresh client for email.
func (h *harness) as(email string) *client {
	h.t.Helper()
	c := h.client(uaChrome)
	c.login(email, false)
	return c
}
