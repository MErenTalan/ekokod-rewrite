//go:build integration

package postgres_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
	"github.com/stretchr/testify/require"
)

// TestIntegrationCredentialSecretsAreBytea guards the spec's requirement
// that provider secrets are stored as AES-256-GCM ciphertext. A text column
// here would invite storing a plaintext password, and the API must never
// return these at all.
//
// This is the two-named-table half; TestNoPlaintextSecretColumns below is
// the schema-wide half that replaced this function's old blocklist tail
// check (F1 final review pass B, I4).
func TestIntegrationCredentialSecretsAreBytea(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)

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
}

// plaintextSecretColumnPattern matches a column name that LOOKS like it
// holds a live credential: password, secret, token (and its compounds
// client_secret/access_token/refresh_token), api_key/apikey, passwd, tgt
// (EPİAŞ's ticket-granting-ticket, coming in F2), private_key.
//
// F1 final review pass B, I4 (ledger Task 5's deferred minor, re-rated
// Important): the guard this replaces was
// `table_name in ('integration_credentials','smtp_settings') and column_name
// in ('password','secret','api_key','token')` — two named tables and four
// exact names. It could not see a plaintext credential column on any OTHER
// table, and could not see `client_secret`, `access_token`,
// `refresh_token`, or `password_plain` on the two tables it DID look at,
// because none of those strings is an exact match for 'password', 'secret',
// 'api_key' or 'token'. F2 adds OAuth token exchange/refresh for iSolar and
// an EPİAŞ TGT cache within weeks (F2 plan, migrations 00012-00013) — this
// widens to catch a plaintext column in either one before it merges, not
// after a reviewer happens to notice.
var plaintextSecretColumnPattern = regexp.MustCompile(`(?i)secret|token|password|passwd|api_?key|tgt|private_key`)

// secretColumnKey identifies one column by (table, column) for
// plaintextSecretColumnAllowlist.
type secretColumnKey struct {
	table, column string
}

// plaintextSecretColumnAllowlist names every column in the CURRENT schema
// that matches plaintextSecretColumnPattern by name but is not a live,
// recoverable credential, together with the reason it is safe as plain
// text. Every entry here is either a ONE-WAY HASH (bcrypt/sha-256: nothing
// can turn it back into the credential it was derived from, so binding
// rule 10's "stored as AES-256-GCM ciphertext" does not apply — that rule
// governs credentials the system must later DECRYPT, and a hash is never
// decrypted) or a plain timestamp whose name happens to contain the
// substring "password" or "token".
//
// This is an EXPLICIT, PER-COLUMN allow-list, not a suffix rule such as
// "anything ending in _hash is fine": a table/column pair is one line to
// add, reviewable in a diff, and cannot silently widen to cover a future
// column that merely happens to share a naming convention.
var plaintextSecretColumnAllowlist = map[secretColumnKey]string{
	{"users", "password_hash"}: "bcrypt hash (one-way); the plaintext password is never stored",
	{"users", "password_changed_at"}: "a timestamp of WHEN the password last changed, not the password " +
		"itself; matches the pattern only because its name contains \"password\"",
	{"user_password_history", "password_hash"}: "bcrypt hash (one-way); the plaintext password is never stored",
	{"sessions", "refresh_token_hash"}: "sha-256 hash of the refresh token (one-way); the raw token itself " +
		"is never stored, only ever compared by re-hashing an incoming one",
	{"integration_credentials", "token_expires_at"}: "a timestamp of when the OAuth token expires, not the " +
		"token itself; matches the pattern only because its name contains \"token\"",
}

// TestNoPlaintextSecretColumns is the schema-wide guard described on
// plaintextSecretColumnPattern above. It runs against a freshly migrated
// database from the SHARED template
// (testfixtures.NewIsolatedDB — the old TestIntegrationCredentialSecretsAreBytea
// booted its own container for this, which this migration also stops
// doing), scans every column in the public schema, and requires that any
// column whose name matches the pattern is either bytea (this schema's
// AES-256-GCM ciphertext convention) or named in
// plaintextSecretColumnAllowlist with a stated reason. Anything else —
// including a brand-new table this test has never heard of — fails by
// default.
func TestNoPlaintextSecretColumns(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)

	rows, err := pool.Query(ctx, `
		select table_name, column_name, data_type
		from information_schema.columns
		where table_schema = 'public'
		order by table_name, column_name`)
	require.NoError(t, err)
	defer rows.Close()

	inspected := 0
	for rows.Next() {
		var table, column, dataType string
		require.NoError(t, rows.Scan(&table, &column, &dataType))
		inspected++

		if !plaintextSecretColumnPattern.MatchString(column) {
			continue
		}
		if dataType == "bytea" {
			continue // this schema's AES-256-GCM ciphertext convention
		}
		if reason, ok := plaintextSecretColumnAllowlist[secretColumnKey{table, column}]; ok {
			require.NotEmpty(t, reason, "%s.%s: an allow-list entry must carry a reason", table, column)
			continue
		}
		t.Errorf("%s.%s (%s) looks like a credential column by name (matches %q) but is neither bytea nor on "+
			"plaintextSecretColumnAllowlist: a secret/token/password-shaped column must be AES-256-GCM ciphertext "+
			"(bytea), or a one-way hash added to the allow-list with a stated reason — never plaintext",
			table, column, dataType, plaintextSecretColumnPattern.String())
	}
	require.NoError(t, rows.Err())

	// Anti-vacuity floor, same shape as the float and scope guards: a walk
	// that silently stopped querying information_schema, or queried the
	// wrong schema, would find nothing to flag and pass for the wrong
	// reason. Measured well above 100 columns across 00001-00011's tables;
	// the floor leaves headroom for a column being dropped while still
	// catching a broken query.
	require.GreaterOrEqual(t, inspected, 100,
		"inspected implausibly few columns (%d): the guard walked far less of the public schema than it did "+
			"when this floor was measured and would pass whatever the schema said", inspected)
}

// TestOperationalMessagesUsesBigserial pins operational_messages.id to a
// bigint identity column, not a uuid — the spec calls this out explicitly
// because every other table in this migration uses uuid primary keys.
func TestOperationalMessagesUsesBigserial(t *testing.T) {
	dsn := testfixtures.StartPostgresUnmigrated(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, testfixtures.DiscardLogger()))
	pool := testfixtures.NewPool(t, dsn)

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
	dsn := testfixtures.StartPostgresUnmigrated(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, testfixtures.DiscardLogger()))
	pool := testfixtures.NewPool(t, dsn)

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
	dsn := testfixtures.StartPostgresUnmigrated(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, testfixtures.DiscardLogger()))
	pool := testfixtures.NewPool(t, dsn)

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
	dsn := testfixtures.StartPostgresUnmigrated(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, testfixtures.DiscardLogger()))
	pool := testfixtures.NewPool(t, dsn)

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
	dsn := testfixtures.StartPostgresUnmigrated(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, testfixtures.DiscardLogger()))
	pool := testfixtures.NewPool(t, dsn)

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
