package admin

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// ReconcileRepository reads what `migrate legacy reconcile` compares (F14c):
// read-only, across every tenant, so it lives on the admin surface.
type ReconcileRepository struct{ pool *pgxpool.Pool }

// NewReconcileRepository builds a ReconcileRepository over pool.
func NewReconcileRepository(pool *pgxpool.Pool) *ReconcileRepository {
	return &ReconcileRepository{pool: pool}
}

// LegacyBillRow is one legacy_bills row.
type LegacyBillRow struct {
	ID, CompanyID, BuildingID                    uuid.UUID
	AnalyzerID                                   *uuid.UUID
	Scope, Period                                string
	StartDate, EndDate                           *time.Time
	TotalActiveKwh, EnergyCost, DistributionCost *decimal.Decimal
	CapacityCost, PowerCost, GreenEnergyCost     *decimal.Decimal
	ReactivePenalty, VatCost, OtherTaxesCost     *decimal.Decimal
	TotalCost                                    *decimal.Decimal
	ReactivePenaltyApplied                       *bool
	Payload                                      json.RawMessage
}

// LiveBillRow is one non-superseded analyzer or building bill with what attribution needs.
type LiveBillRow struct {
	CompanyID, BuildingID                                  uuid.UUID
	AnalyzerID                                             *uuid.UUID
	Scope, Period, Status                                  string
	FlagReason                                             *string
	ActiveImport, NetConsumption, ActiveExport             decimal.Decimal
	EnergyCost, DistributionCost, GreenEnergyCost          decimal.Decimal
	PowerCost, DemandOverrunCost, ReactivePenalty          decimal.Decimal
	OtherTaxesCost, VatBase, VatCost, TotalCost            decimal.Decimal
	ReactivePenaltyApplied, TieredApplied                  bool
	InductiveThreshold                                     *decimal.Decimal
	PriceType, Term, UserGroup                             *string
	TariffVatRate, TariffDistributionPrice, InstalledPower *decimal.Decimal
}

// Anomaly is an unresolved consumption anomaly (a withheld period, 02 §3.2).
type Anomaly struct {
	AnalyzerID uuid.UUID
	From, To   time.Time
}

// Names maps company, building and analyzer ids to what the report shows.
type Names struct {
	Companies, Buildings, Analyzers map[uuid.UUID]string
	AnalyzerBuilding                map[uuid.UUID]uuid.UUID
	BuildingCompany                 map[uuid.UUID]uuid.UUID
}

// LegacyBills lists every legacy_bills row.
func (r *ReconcileRepository) LegacyBills(ctx context.Context) ([]LegacyBillRow, error) {
	rows, err := r.pool.Query(ctx, `select id, company_id, building_id, analyzer_id, scope, period, start_date, end_date, total_active_kwh,
		energy_cost, distribution_cost, capacity_cost, power_cost, green_energy_cost, reactive_penalty, vat_cost, other_taxes_cost,
		total_cost, reactive_penalty_applied, payload from legacy_bills order by company_id, scope, building_id, analyzer_id, period`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (LegacyBillRow, error) {
		var b LegacyBillRow
		err := row.Scan(&b.ID, &b.CompanyID, &b.BuildingID, &b.AnalyzerID, &b.Scope, &b.Period, &b.StartDate, &b.EndDate, &b.TotalActiveKwh,
			&b.EnergyCost, &b.DistributionCost, &b.CapacityCost, &b.PowerCost, &b.GreenEnergyCost, &b.ReactivePenalty, &b.VatCost,
			&b.OtherTaxesCost, &b.TotalCost, &b.ReactivePenaltyApplied, &b.Payload)
		return b, err
	})
}

// LiveBills lists the live analyzer and building bills with their tariff and installed power.
func (r *ReconcileRepository) LiveBills(ctx context.Context) ([]LiveBillRow, error) {
	rows, err := r.pool.Query(ctx, `select b.company_id, b.building_id, b.analyzer_id, b.scope::text, b.period_key, b.status::text, b.flag_reason,
		b.active_import, b.net_consumption, b.active_export, b.energy_cost, b.distribution_cost, b.green_energy_cost, b.power_cost,
		b.demand_overrun_cost, b.reactive_penalty, b.other_taxes_cost, b.vat_base, b.vat_cost, b.total_cost, b.reactive_penalty_applied,
		b.tiered_applied, b.inductive_threshold, t.price_type::text, t.term::text, t.user_group::text, t.vat_rate, t.distribution_cost,
		case when b.scope = 'analyzer' then (select a.installed_power_kw from analyzers a where a.id = b.analyzer_id)
		     else (select sum(a.installed_power_kw) from analyzers a where a.building_id = b.building_id and a.deleted_at is null) end
		from bills b left join tariffs t on t.id = b.tariff_id
		where b.status <> 'superseded' and b.scope in ('analyzer','building') and b.building_id is not null
		order by b.company_id, b.scope, b.building_id, b.analyzer_id, b.period_key`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (LiveBillRow, error) {
		var b LiveBillRow
		err := row.Scan(&b.CompanyID, &b.BuildingID, &b.AnalyzerID, &b.Scope, &b.Period, &b.Status, &b.FlagReason, &b.ActiveImport,
			&b.NetConsumption, &b.ActiveExport, &b.EnergyCost, &b.DistributionCost, &b.GreenEnergyCost, &b.PowerCost, &b.DemandOverrunCost,
			&b.ReactivePenalty, &b.OtherTaxesCost, &b.VatBase, &b.VatCost, &b.TotalCost, &b.ReactivePenaltyApplied, &b.TieredApplied,
			&b.InductiveThreshold, &b.PriceType, &b.Term, &b.UserGroup, &b.TariffVatRate, &b.TariffDistributionPrice, &b.InstalledPower)
		return b, err
	})
}

// UnresolvedAnomalies lists the periods billing withholds.
func (r *ReconcileRepository) UnresolvedAnomalies(ctx context.Context) ([]Anomaly, error) {
	rows, err := r.pool.Query(ctx, `select analyzer_id, period_start, period_end from consumption_anomalies where resolved_at is null`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Anomaly, error) {
		var a Anomaly
		return a, row.Scan(&a.AnalyzerID, &a.From, &a.To)
	})
}

// ReadingCounts is meter_readings rows per analyzer and Istanbul month.
func (r *ReconcileRepository) ReadingCounts(ctx context.Context) (map[uuid.UUID]map[string]int, error) {
	rows, err := r.pool.Query(ctx, `select analyzer_id, to_char(ts at time zone 'Europe/Istanbul', 'YYYY-MM'), count(*)
		from meter_readings group by 1, 2`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]map[string]int{}
	for rows.Next() {
		var id uuid.UUID
		var month string
		var n int
		if err := rows.Scan(&id, &month, &n); err != nil {
			return nil, err
		}
		if out[id] == nil {
			out[id] = map[string]int{}
		}
		out[id][month] = n
	}
	return out, rows.Err()
}

// CarbonByBuildingYear sums emissions per building and year of period_start.
func (r *ReconcileRepository) CarbonByBuildingYear(ctx context.Context) (map[uuid.UUID]map[int]decimal.Decimal, error) {
	rows, err := r.pool.Query(ctx, `select building_id, extract(year from period_start)::int, sum(emission_kgco2e)
		from carbon_activities group by 1, 2`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]map[int]decimal.Decimal{}
	for rows.Next() {
		var id uuid.UUID
		var year int
		var sum decimal.Decimal
		if err := rows.Scan(&id, &year, &sum); err != nil {
			return nil, err
		}
		if out[id] == nil {
			out[id] = map[int]decimal.Decimal{}
		}
		out[id][year] = sum
	}
	return out, rows.Err()
}

// StoredFiles counts the given stored_files ids present and their bytes.
func (r *ReconcileRepository) StoredFiles(ctx context.Context, ids []uuid.UUID) (count int, bytes int64, err error) {
	err = r.pool.QueryRow(ctx, `select count(*), coalesce(sum(size_bytes), 0) from stored_files where id = any($1)`, ids).Scan(&count, &bytes)
	return count, bytes, err
}

// Names reads the display names of every company, building and analyzer.
func (r *ReconcileRepository) Names(ctx context.Context) (Names, error) {
	n := Names{Companies: map[uuid.UUID]string{}, Buildings: map[uuid.UUID]string{}, Analyzers: map[uuid.UUID]string{},
		AnalyzerBuilding: map[uuid.UUID]uuid.UUID{}, BuildingCompany: map[uuid.UUID]uuid.UUID{}}
	for _, q := range []struct {
		sql string
		to  map[uuid.UUID]string
	}{
		{`select id, name from companies`, n.Companies},
		{`select id, name from buildings`, n.Buildings},
		{`select id, coalesce(nullif(installation_number, ''), meter_number, id::text) from analyzers`, n.Analyzers},
	} {
		rows, err := r.pool.Query(ctx, q.sql)
		if err != nil {
			return n, err
		}
		for rows.Next() {
			var id uuid.UUID
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				rows.Close()
				return n, err
			}
			q.to[id] = name
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return n, err
		}
	}
	for _, q := range []struct {
		sql string
		to  map[uuid.UUID]uuid.UUID
	}{
		{`select id, building_id from analyzers where building_id is not null`, n.AnalyzerBuilding},
		{`select id, company_id from buildings`, n.BuildingCompany},
	} {
		rows, err := r.pool.Query(ctx, q.sql)
		if err != nil {
			return n, err
		}
		for rows.Next() {
			var id, parent uuid.UUID
			if err := rows.Scan(&id, &parent); err != nil {
				rows.Close()
				return n, err
			}
			q.to[id] = parent
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return n, err
		}
	}
	return n, nil
}
