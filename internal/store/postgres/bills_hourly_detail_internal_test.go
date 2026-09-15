//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// billProofPostgresImage/DB/User/Password mirror internal/testfixtures'
// unexported constants of the same values. This file cannot import
// testfixtures: testfixtures imports this package (postgres) to run
// migrations, so package postgres importing testfixtures back would be an
// import cycle — which is also exactly why this proof has to live here, in
// package postgres, rather than in the postgres_test external test package
// every other integration test in this directory uses: only same-package
// code can reach BillRepository's unexported q field, which is what lets
// this test call the generated query directly and bypass
// BillRepository.requireVisible entirely.
const (
	billProofPostgresImage = "timescale/timescaledb:2.30.0-pg16"
	billProofDB            = "ekokod"
	billProofUser          = "ekokod"
	billProofPassword      = "ekokod"
)

// newBillProofPool boots a fresh, migrated TimescaleDB container and returns
// a pool over it. It is a deliberately minimal, single-purpose duplicate of
// testfixtures.StartPostgres + postgres.NewPool — seeing exactly one
// container per run of this test, reaped by the ryuk sidecar like every
// other testcontainers-backed test in this repository when the test binary
// exits.
func newBillProofPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, billProofPostgresImage,
		tcpostgres.WithDatabase(billProofDB),
		tcpostgres.WithUsername(billProofUser),
		tcpostgres.WithPassword(billProofPassword),
		tcpostgres.WithSQLDriver("pgx"),
		testcontainers.WithWaitStrategyAndDeadline(180*time.Second,
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			wait.ForListeningPort("5432/tcp").WithStartupTimeout(180*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	logger := slog.New(slog.DiscardHandler)
	require.NoError(t, MigrateUp(ctx, dsn, logger), "migrating the fixture database")

	pool, err := NewPool(ctx, config.DB{URL: dsn, MaxConns: 5, MinConns: 1}, logger)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

// insertBillProofCompany inserts the one companies row a bill needs. Every
// other companies column is nullable or defaulted (migrations/00002_tenancy.sql).
func insertBillProofCompany(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, name string) {
	t.Helper()
	_, err := pool.Exec(ctx, `insert into companies (id, name) values ($1, $2)`, id, name)
	require.NoError(t, err)
}

// TestBillHourlyDetailInsertRefusesForeignBillEvenWithoutGoPreCheck proves the
// SQL-level guard on BillHourlyDetailInsert itself, independently of
// BillRepository.requireVisible — the Go-side pre-check ReplaceHourlyDetail
// normally runs before ever reaching this query. It calls
// BillRepository's unexported q.BillHourlyDetailInsert directly, with
// company A's scope but company B's bill id, so requireVisible is never
// consulted at all.
//
// This is the same class of proof fix round 1 used for
// AlarmAnalyzerInsert/AlarmChannelInsert/BillMemberInsert (see
// task-11a-report.md, "Important 1") — BillHourlyDetailInsert was the one
// sibling round 1 left as :exec, which is exactly the defect this round
// fixes: an insert whose WHERE EXISTS guard matches zero rows still reports
// err == nil for :exec, so a caller checking only err reads a
// silently-guarded no-op as success.
//
// Before the fix: this test's first assertion FAILS (err is nil; zero rows
// were inserted but the call claimed success). After the fix (:one
// RETURNING true): the guard miss surfaces as pgx.ErrNoRows and the
// assertion passes.
func TestBillHourlyDetailInsertRefusesForeignBillEvenWithoutGoPreCheck(t *testing.T) {
	ctx := context.Background()
	pool := newBillProofPool(t)

	companyA := uuid.New()
	companyB := uuid.New()
	insertBillProofCompany(t, ctx, pool, companyA, "Bill Proof Tenant A")
	insertBillProofCompany(t, ctx, pool, companyB, "Bill Proof Tenant B")

	scopeB := store.Scope{CompanyID: companyB, AllBuildings: true}
	repo := NewBillRepository(pool)

	now := time.Now().UTC()
	billB, err := repo.Create(ctx, scopeB, model.Bill{
		CompanyID: companyB, Scope: model.BillScopeCompany,
		PeriodKey:   "2026-01",
		PeriodStart: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), DaysInPeriod: 31,
		IndexStart: json.RawMessage(`{}`), IndexEnd: json.RawMessage(`{}`),
		GenerationUsage: model.GenerationUsageNone,
		Status:          model.BillStatusDraft, ComputedAt: now, CreatedAt: now, UpdatedAt: now,
	}, nil, nil)
	require.NoError(t, err)

	one := decimal.RequireFromString("1.0000")

	// Bypasses requireVisible entirely: calls the generated query directly
	// with company A's company/AllBuildings scope but company B's bill id.
	ok, err := repo.q.BillHourlyDetailInsert(ctx, sqlcgen.BillHourlyDetailInsertParams{
		BillID:       billB.ID,
		Ts:           tariffTimestamptz(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		Consumption:  decimalToNumeric(one),
		Ptf:          decimalToNumeric(one),
		Yekdem:       decimalToNumeric(one),
		Kbk:          decimalToNumeric(one),
		UnitPrice:    decimalToNumeric(one),
		Cost:         decimalToNumeric(one),
		CompanyID:    companyA,
		AllBuildings: true,
		BuildingIds:  nil,
	})
	require.False(t, ok, "a guard-miss must not report success")
	require.Error(t, err, "a guard-miss must be a caller-visible error, not a silent no-op")
	require.True(t, errors.Is(err, pgx.ErrNoRows),
		"a guard-miss on BillHourlyDetailInsert should surface as pgx.ErrNoRows")

	// And the row must genuinely not have been written.
	rows, err := repo.HourlyDetail(ctx, scopeB, billB.ID)
	require.NoError(t, err)
	require.Empty(t, rows, "the guard-missed insert must not have stored anything")
}
