//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgnum"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// TestBillHourlyDetailInsertRefusesForeignBillEvenWithoutGoPreCheck proves the
// SQL-level guard on BillHourlyDetailInsert itself, independently of
// BillRepository.requireVisible — the Go-side pre-check ReplaceHourlyDetail
// normally runs before ever reaching this query. It calls
// BillRepository's exported-for-tests Queries() to reach
// q.BillHourlyDetailInsert directly, with company A's scope but company B's
// bill id, so requireVisible is never consulted at all.
//
// F2 Task 0P2, step 4: this test used to live in package postgres (that
// package cannot import testfixtures — testfixtures imports postgres for
// MigrateUp, so the reverse import would cycle) and duplicated ~20 lines of
// testfixtures.StartPostgres + postgres.NewPool locally, booting its own
// container. export_test.go now exposes BillRepository.Queries() for
// exactly this purpose, so the test moves to the external postgres_test
// package everything else in this directory uses and shares
// testfixtures.NewIsolatedDB's one container instead of booting its own.
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
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)

	companyA := uuid.New()
	companyB := uuid.New()
	_, err := pool.Exec(ctx, `insert into companies (id, name) values ($1, $2)`, companyA, "Bill Proof Tenant A")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `insert into companies (id, name) values ($1, $2)`, companyB, "Bill Proof Tenant B")
	require.NoError(t, err)

	scopeB := store.Scope{CompanyID: companyB, AllBuildings: true}
	repo := postgres.NewBillRepository(pool)

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

	one := pgnum.DecimalToNumeric(decimal.RequireFromString("1.0000"))

	// Bypasses requireVisible entirely: calls the generated query directly
	// with company A's company/AllBuildings scope but company B's bill id.
	ok, err := repo.Queries().BillHourlyDetailInsert(ctx, sqlcgen.BillHourlyDetailInsertParams{
		BillID:       billB.ID,
		Ts:           pgtype.Timestamptz{Time: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		Consumption:  one,
		Ptf:          one,
		Yekdem:       one,
		Kbk:          one,
		UnitPrice:    one,
		Cost:         one,
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
