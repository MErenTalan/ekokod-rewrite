-- Bills, bill lines, bill members and bill hourly detail (migration 00009).
-- Query names are prefixed Bill…

-- name: BillGet :one
select * from bills
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]));

-- name: BillList :many
select * from bills
where company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]))
  and (cardinality(sqlc.arg(ids)::uuid[]) = 0 or id = any(sqlc.arg(ids)::uuid[]))
  and (sqlc.narg(building_id)::uuid is null or building_id = sqlc.narg(building_id))
  and (sqlc.narg(analyzer_id)::uuid is null or analyzer_id = sqlc.narg(analyzer_id))
  and (sqlc.narg(bill_scope)::bill_scope is null or scope = sqlc.narg(bill_scope))
  and (sqlc.narg(period_key)::text is null or period_key = sqlc.narg(period_key))
  -- Statuses is compared as text[], not bill_status[]: pgx has no codec
  -- for an array of this custom enum, and fails even on an empty slice.
  and (cardinality(sqlc.arg(statuses)::text[]) = 0 or status::text = any(sqlc.arg(statuses)::text[]))
  and (sqlc.arg(include_superseded)::boolean or status <> 'superseded')
order by period_key desc, id
limit sqlc.arg(limit_val) offset sqlc.arg(offset_val);

-- name: BillCurrent :one
select * from bills
where company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]))
  and scope = sqlc.arg(bill_scope)
  and coalesce(analyzer_id, building_id, company_id) = sqlc.arg(subject_id)
  and period_key = sqlc.arg(period_key)
  and status <> 'superseded';

-- name: BillVisible :one
select exists (
  select 1 from bills
  where id = sqlc.arg(id) and company_id = sqlc.arg(company_id)
    and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]))
) as visible;

-- BillCountVisibleAnalyzers checks that every analyzer id in a batch belongs
-- to the Scope's company and one of its visible buildings, in one round
-- trip: the caller compares the count to the distinct id count and refuses
-- the whole write if they differ.
-- name: BillCountVisibleAnalyzers :one
select count(*) from analyzers
where company_id = sqlc.arg(company_id) and deleted_at is null
  and id = any(sqlc.arg(analyzer_ids)::uuid[])
  and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]));

-- name: BillCreate :one
insert into bills (
  id, company_id, building_id, analyzer_id, scope, period_key, period_start, period_end,
  days_in_period, tariff_id, tariff_effective_from, active_import, t1_kwh, t2_kwh, t3_kwh,
  inductive_kvarh, capacitive_kvarh, active_export, net_consumption, low_tier_kwh, high_tier_kwh,
  max_demand_kw, tiered_applied, index_start, index_end, effective_energy_price, low_tier_price,
  high_tier_price, energy_cost, distribution_cost, green_energy_cost, power_cost,
  demand_overrun_cost, reactive_penalty, other_taxes_cost, vat_base, vat_cost, generation_credit,
  total_cost, inductive_ratio, capacitive_ratio, inductive_threshold, capacitive_threshold,
  reactive_penalty_applied, reactive_power_price, generation_usage, generation_price_per_kwh,
  ptf_yekdem_used, ptf_hours_matched, ptf_hours_missing, ptf_average, yekdem_used, status,
  flag_reason, pdf_path, computed_at, created_at, updated_at
) values (
  gen_random_uuid(), sqlc.arg(company_id), sqlc.narg(building_id), sqlc.narg(analyzer_id),
  sqlc.arg(bill_scope), sqlc.arg(period_key), sqlc.arg(period_start), sqlc.arg(period_end),
  sqlc.arg(days_in_period), sqlc.narg(tariff_id), sqlc.narg(tariff_effective_from),
  sqlc.arg(active_import), sqlc.arg(t1_kwh), sqlc.arg(t2_kwh), sqlc.arg(t3_kwh),
  sqlc.arg(inductive_kvarh), sqlc.arg(capacitive_kvarh), sqlc.arg(active_export),
  sqlc.arg(net_consumption), sqlc.arg(low_tier_kwh), sqlc.arg(high_tier_kwh),
  sqlc.narg(max_demand_kw), sqlc.arg(tiered_applied), sqlc.arg(index_start), sqlc.arg(index_end),
  sqlc.narg(effective_energy_price), sqlc.narg(low_tier_price), sqlc.narg(high_tier_price),
  sqlc.arg(energy_cost), sqlc.arg(distribution_cost), sqlc.arg(green_energy_cost),
  sqlc.arg(power_cost), sqlc.arg(demand_overrun_cost), sqlc.arg(reactive_penalty),
  sqlc.arg(other_taxes_cost), sqlc.arg(vat_base), sqlc.arg(vat_cost), sqlc.arg(generation_credit),
  sqlc.arg(total_cost), sqlc.narg(inductive_ratio), sqlc.narg(capacitive_ratio),
  sqlc.narg(inductive_threshold), sqlc.narg(capacitive_threshold), sqlc.arg(reactive_penalty_applied),
  sqlc.narg(reactive_power_price), sqlc.arg(generation_usage), sqlc.narg(generation_price_per_kwh),
  sqlc.arg(ptf_yekdem_used), sqlc.narg(ptf_hours_matched), sqlc.narg(ptf_hours_missing),
  sqlc.narg(ptf_average), sqlc.narg(yekdem_used), sqlc.arg(status), sqlc.narg(flag_reason),
  sqlc.narg(pdf_path), sqlc.arg(computed_at), sqlc.arg(created_at), sqlc.arg(created_at)
) returning *;

-- name: BillLineInsert :one
insert into bill_lines (id, bill_id, code, label, quantity, unit, unit_price, rate_pct, amount, sort_order)
values (gen_random_uuid(), sqlc.arg(bill_id), sqlc.arg(code), sqlc.arg(label), sqlc.narg(quantity),
        sqlc.narg(unit), sqlc.narg(unit_price), sqlc.narg(rate_pct), sqlc.arg(amount), sqlc.arg(sort_order))
returning *;

-- name: BillMemberInsert :exec
insert into bill_members (bill_id, analyzer_id) values (sqlc.arg(bill_id), sqlc.arg(analyzer_id));

-- name: BillMarkSuperseded :execrows
update bills set status = 'superseded', updated_at = sqlc.arg(updated_at)
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]))
  and status <> 'superseded';

-- name: BillUpdateStatus :one
update bills set status = sqlc.arg(status), flag_reason = sqlc.narg(flag_reason), updated_at = sqlc.arg(updated_at)
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]))
returning *;

-- name: BillSetPDFPath :execrows
update bills set pdf_path = sqlc.arg(pdf_path), updated_at = now()
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]));

-- name: BillLineList :many
select * from bill_lines where bill_id = sqlc.arg(bill_id) order by sort_order, id;

-- name: BillMemberList :many
select * from bill_members where bill_id = sqlc.arg(bill_id) order by analyzer_id;

-- name: BillHourlyDetailList :many
select * from bill_hourly_detail where bill_id = sqlc.arg(bill_id) order by ts;

-- name: BillHourlyDetailDeleteForBill :exec
delete from bill_hourly_detail where bill_id = sqlc.arg(bill_id);

-- name: BillHourlyDetailInsert :exec
insert into bill_hourly_detail (bill_id, ts, consumption, ptf, yekdem, kbk, unit_price, cost)
values (sqlc.arg(bill_id), sqlc.arg(ts), sqlc.arg(consumption), sqlc.arg(ptf), sqlc.arg(yekdem),
        sqlc.arg(kbk), sqlc.arg(unit_price), sqlc.arg(cost));
