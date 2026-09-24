package seed

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
)

// ScaleEmail is the scale dataset's company admin, the load test's login (F15b R466).
const ScaleEmail = "scale@olcek.ekokod.test"

var scaleNS = uuid.MustParse("3b8f2c1e-5a47-4d0b-9e61-7c2f0a9d4e58")

func scaleID(kind string, i int) uuid.UUID {
	return uuid.NewSHA1(scaleNS, fmt.Appendf(nil, "%s/%d", kind, i))
}

// ScaleOptions size the dataset (Q-M1: 2× production = 200 analyzers, 40 buildings, 400 days).
type ScaleOptions struct {
	Analyzers, Buildings, Days int
	Password                   string
}

// ScaleResult counts what Scale wrote.
type ScaleResult struct {
	CompanyID uuid.UUID
	Readings  int64
}

// Scale builds the load-test dataset: one company, its buildings with a
// tariff each, OSOS analyzers spread over them, and hourly load-profile
// readings generated in SQL up to `now`. Deterministic ids; a rerun converges.
func Scale(ctx context.Context, pool *pgxpool.Pool, hasher auth.Hasher, o ScaleOptions, now time.Time) (ScaleResult, error) {
	if o.Analyzers < 1 || o.Buildings < 1 || o.Days < 1 || o.Password == "" {
		return ScaleResult{}, fmt.Errorf("seed scale: analyzers, buildings, days and a password are required")
	}
	company := scaleID("company", 0)
	hash, err := hasher.Hash(o.Password)
	if err != nil {
		return ScaleResult{}, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return ScaleResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	stmts := []struct {
		sql  string
		args []any
	}{
		{`insert into companies (id, name) values ($1, 'Ölçek Test A.Ş.') on conflict (id) do nothing`, []any{company}},
		{`insert into users (id, company_id, name, email, password_hash, role, is_active) values ($1, $2, 'Ölçek Yöneticisi', $3, $4, 'company_admin', true)
			on conflict (id) do update set password_hash = excluded.password_hash`, []any{scaleID("user", 0), company, ScaleEmail, hash}},
	}
	for _, s := range stmts {
		if _, err := tx.Exec(ctx, s.sql, s.args...); err != nil {
			return ScaleResult{}, fmt.Errorf("seed scale: %w", err)
		}
	}
	for b := range o.Buildings {
		id := scaleID("building", b)
		if _, err := tx.Exec(ctx, `insert into buildings (id, company_id, name) values ($1, $2, $3) on conflict (id) do nothing`,
			id, company, fmt.Sprintf("Ölçek Binası %03d", b+1)); err != nil {
			return ScaleResult{}, fmt.Errorf("seed scale: building: %w", err)
		}
		if _, err := tx.Exec(ctx, `insert into tariffs (id, company_id, building_id, effective_from, voltage_level, user_group, price_type, term,
				supply_company, single_time_price, distribution_cost, reactive_power_price, vat_rate)
			values ($1, $2, $3, date '2020-01-01', 'lv', 'commercial', 'single_time', 'monomial', 'private', 3.2, 1.1, 0.5, 20)
			on conflict (id) do nothing`, scaleID("tariff", b), company, id); err != nil {
			return ScaleResult{}, fmt.Errorf("seed scale: tariff: %w", err)
		}
	}
	analyzers := make([]uuid.UUID, o.Analyzers)
	for a := range o.Analyzers {
		analyzers[a] = scaleID("analyzer", a)
		if _, err := tx.Exec(ctx, `insert into analyzers (id, company_id, building_id, provider, provider_subtype, installation_number, is_active, installed_power_kw)
				values ($1, $2, $3, 'osos', 'Olcek', $4, true, 50) on conflict (id) do nothing`,
			analyzers[a], company, scaleID("building", a%o.Buildings), fmt.Sprintf("OLCEK-%05d", a+1)); err != nil {
			return ScaleResult{}, fmt.Errorf("seed scale: analyzer: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ScaleResult{}, err
	}
	end := now.UTC().Truncate(time.Hour)
	start := end.AddDate(0, 0, -o.Days)
	// The bulk statements outlive the pool's statement timeout: one session without it.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return ScaleResult{}, err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "set statement_timeout = 0"); err != nil {
		return ScaleResult{}, err
	}
	var written int64
	for i, a := range analyzers {
		// A cumulative index with its own base load per analyzer.
		tag, err := conn.Exec(ctx, `insert into meter_readings (analyzer_id, ts, kind, active_import, reactive_inductive_import,
				reactive_capacitive_import, source_provider, multiplier_applied)
			select $1, g, 'load_profile', $4::numeric * 1000 + extract(epoch from g - $2::timestamptz) / 3600 * (5 + $4::int % 7),
				extract(epoch from g - $2::timestamptz) / 3600 * 0.8, 0, 'osos', 1
			from generate_series($2::timestamptz, $3::timestamptz, interval '1 hour') g
			on conflict (analyzer_id, ts, kind) do nothing`, a, start, end, i+1)
		if err != nil {
			return ScaleResult{}, fmt.Errorf("seed scale: readings: %w", err)
		}
		written += tag.RowsAffected()
	}
	for _, view := range []string{"consumption_hourly", "consumption_daily", "consumption_monthly", "consumption_yearly"} {
		if _, err := conn.Exec(ctx, "call refresh_continuous_aggregate($1::regclass, null, null)", view); err != nil {
			return ScaleResult{}, fmt.Errorf("seed scale: refresh %s: %w", view, err)
		}
	}
	return ScaleResult{CompanyID: company, Readings: written}, nil
}
