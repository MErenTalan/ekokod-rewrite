//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/stretchr/testify/require"
)

// TestIntegrationCredentialSecretsAreBytea guards the spec's requirement
// that provider secrets are stored as AES-256-GCM ciphertext. A text column
// here would invite storing a plaintext password, and the API must never
// return these at all.
func TestIntegrationCredentialSecretsAreBytea(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	for table, columns := range map[string][]string{
		"integration_credentials": {"secret_enc", "extra_enc"},
		"smtp_settings":           {"password_enc"},
	} {
		for _, column := range columns {
			var dataType string
			require.NoError(t, pool.QueryRow(ctx,
				`select data_type from information_schema.columns
				 where table_name = $1 and column_name = $2`, table, column).Scan(&dataType),
				"%s.%s must exist", table, column)
			require.Equal(t, "bytea", dataType, "%s.%s must hold ciphertext", table, column)
		}
	}

	var plaintext int
	require.NoError(t, pool.QueryRow(ctx,
		`select count(*) from information_schema.columns
		 where table_name in ('integration_credentials','smtp_settings')
		   and column_name in ('password','secret','api_key','token')`).Scan(&plaintext))
	require.Zero(t, plaintext, "no plaintext secret column may exist")
}

// TestOperationalMessagesUsesBigserial pins operational_messages.id to a
// bigint identity column, not a uuid — the spec calls this out explicitly
// because every other table in this migration uses uuid primary keys.
func TestOperationalMessagesUsesBigserial(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	var dataType string
	require.NoError(t, pool.QueryRow(ctx,
		`select data_type from information_schema.columns
		 where table_name = 'operational_messages' and column_name = 'id'`,
	).Scan(&dataType))
	require.Equal(t, "bigint", dataType, "operational_messages.id must be bigserial (bigint identity)")
}

// TestJobRunsJSONBColumns pins job_runs.scope and .detail to jsonb, as the
// brief calls out explicitly.
func TestJobRunsJSONBColumns(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	for _, column := range []string{"scope", "detail"} {
		var dataType string
		require.NoError(t, pool.QueryRow(ctx,
			`select data_type from information_schema.columns
			 where table_name = 'job_runs' and column_name = $1`, column,
		).Scan(&dataType))
		require.Equal(t, "jsonb", dataType, "job_runs.%s must be jsonb", column)
	}
}

// TestCompanyWeekendDaysRejectsOutOfRangeDay proves the day_of_week check
// constraint is present and enforced by Postgres, not just documented.
func TestCompanyWeekendDaysRejectsOutOfRangeDay(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	companyID, _ := seedCompanyAndBuilding(t, ctx, pool)

	_, err := pool.Exec(ctx,
		`insert into company_weekend_days (company_id, day_of_week) values ($1, $2)`,
		companyID, 7)
	require.Error(t, err, "day_of_week outside 0-6 must be rejected")

	_, err = pool.Exec(ctx,
		`insert into company_weekend_days (company_id, day_of_week) values ($1, $2)`,
		companyID, 0)
	require.NoError(t, err, "day_of_week 0 (Sunday) must be accepted")
}

// TestCompanyVacationsRejectsInvertedRange proves the named valid_range check
// on company_vacations is present and enforced.
func TestCompanyVacationsRejectsInvertedRange(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	companyID, _ := seedCompanyAndBuilding(t, ctx, pool)

	_, err := pool.Exec(ctx,
		`insert into company_vacations (company_id, start_date, end_date) values ($1, $2, $3)`,
		companyID, "2026-02-10", "2026-02-01")
	require.Error(t, err, "start_date after end_date must be rejected")

	_, err = pool.Exec(ctx,
		`insert into company_vacations (company_id, start_date, end_date) values ($1, $2, $3)`,
		companyID, "2026-02-01", "2026-02-10")
	require.NoError(t, err, "start_date <= end_date must be accepted")
}

// TestOperationsTablesRoundTrip exercises a representative insert into every
// table this migration creates, proving foreign keys against companies and
// users, defaults and the operational_messages / job_runs shapes all hold
// together end to end.
func TestOperationsTablesRoundTrip(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	companyID, _ := seedCompanyAndBuilding(t, ctx, pool)

	var fileID string
	require.NoError(t, pool.QueryRow(ctx,
		`insert into stored_files (company_id, owner_type, original_name, stored_path, content_type, size_bytes, checksum)
		 values ($1, 'report', 'a.pdf', '/tmp/a.pdf', 'application/pdf', 123, 'deadbeef')
		 returning id`, companyID).Scan(&fileID))
	require.NotEmpty(t, fileID)

	var definitionID string
	require.NoError(t, pool.QueryRow(ctx,
		`insert into integration_definitions (provider, subtype) values ('osos', 'default') returning id`,
	).Scan(&definitionID))

	var credentialID string
	require.NoError(t, pool.QueryRow(ctx,
		`insert into integration_credentials (company_id, definition_id, username, secret_enc)
		 values ($1, $2, 'svc-user', $3) returning id`,
		companyID, definitionID, []byte{0xde, 0xad, 0xbe, 0xef}).Scan(&credentialID))
	require.NotEmpty(t, credentialID)

	_, err := pool.Exec(ctx,
		`insert into smtp_settings (company_id, host, port, username, password_enc, from_address)
		 values ($1, 'smtp.example.com', 587, 'svc', $2, 'noreply@example.com')`,
		companyID, []byte{0x01, 0x02})
	require.NoError(t, err)

	var eventID string
	require.NoError(t, pool.QueryRow(ctx,
		`insert into calendar_events (company_id, title, starts_at, ends_at)
		 values ($1, 'Maintenance', now(), now() + interval '1 hour') returning id`,
		companyID).Scan(&eventID))
	require.NotEmpty(t, eventID)

	_, err = pool.Exec(ctx,
		`insert into company_weekend_days (company_id, day_of_week) values ($1, 6)`, companyID)
	require.NoError(t, err)

	var vacationID string
	require.NoError(t, pool.QueryRow(ctx,
		`insert into company_vacations (company_id, start_date, end_date) values ($1, '2026-07-01', '2026-07-15') returning id`,
		companyID).Scan(&vacationID))
	require.NotEmpty(t, vacationID)

	var jobRunID string
	require.NoError(t, pool.QueryRow(ctx,
		`insert into job_runs (company_id, job_type, scope, detail) values ($1, 'bill-generation', '{}', '{"note":"ok"}') returning id`,
		companyID).Scan(&jobRunID))
	require.NotEmpty(t, jobRunID)

	var messageID int64
	require.NoError(t, pool.QueryRow(ctx,
		`insert into operational_messages (company_id, kind, category, status, message)
		 values ($1, 'job', 'bill-generation', 'success', 'done') returning id`,
		companyID).Scan(&messageID))
	require.Positive(t, messageID)
}
