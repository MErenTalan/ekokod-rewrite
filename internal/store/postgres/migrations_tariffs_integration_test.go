//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestTariffConstraintsRejectIncompleteRows proves the four check constraints
// in 04-data-model.md §5 actually fire. Each is a business rule: a
// single_time tariff with no price, or a PTF tariff with no KBK, would
// produce a silently wrong invoice in F4.
func TestTariffConstraintsRejectIncompleteRows(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	var companyID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`insert into companies (name) values ('acme') returning id`).Scan(&companyID))

	insert := func(t *testing.T, overrides string, args ...any) error {
		t.Helper()
		_, err := pool.Exec(ctx, `insert into tariffs (
			company_id, effective_from, voltage_level, user_group, price_type,
			term, supply_company, distribution_cost, reactive_power_price, vat_rate`+
			overrides, append([]any{companyID}, args...)...)
		return err
	}

	t.Run("single_time without a price", func(t *testing.T) {
		err := insert(t, `) values ($1,'2026-01-01','lv','residential','single_time',
			'monomial','incumbent',0,0,20)`)
		require.ErrorContains(t, err, "single_time_needs_price")
	})

	t.Run("multi_time without all three band prices", func(t *testing.T) {
		err := insert(t, `, t1_price, t2_price) values ($1,'2026-01-01','lv','residential',
			'multi_time','monomial','incumbent',0,0,20,1,1)`)
		require.ErrorContains(t, err, "multi_time_needs_prices")
	})

	t.Run("subtract_from_total without a generation price", func(t *testing.T) {
		err := insert(t, `, single_time_price, generation_usage) values ($1,'2026-01-01','lv',
			'residential','single_time','monomial','incumbent',0,0,20,1,'subtract_from_total')`)
		require.ErrorContains(t, err, "subtract_from_total_needs_price")
	})

	t.Run("ptf without kbk_energy", func(t *testing.T) {
		err := insert(t, `, single_time_price, use_ptf_yekdem) values ($1,'2026-01-01','lv',
			'residential','single_time','monomial','incumbent',0,0,20,1,true)`)
		require.ErrorContains(t, err, "ptf_needs_energy_kbk")
	})
}

// TestVatRateHasNoDefault guards the spec's explicit "REQUIRED, no default"
// on tariffs.vat_rate: a defaulted VAT rate would silently price every
// invoice at whatever the default happened to be.
func TestVatRateHasNoDefault(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	var def *string
	require.NoError(t, pool.QueryRow(ctx,
		`select column_default from information_schema.columns
		 where table_name = 'tariffs' and column_name = 'vat_rate'`).Scan(&def))
	require.Nil(t, def, "tariffs.vat_rate must have no default")
}
