-- Bills, bill lines, bill members and bill hourly detail (migration 00009).
-- Query names are prefixed Bill…

-- Folded minor (fix round 1): a bill of a soft-deleted building must not
-- stay readable through the bill's own row — Get/List both exclude it.
-- name: BillGet :one
select * from bills
where bills.id = sqlc.arg(id) and bills.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or bills.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and (bills.building_id is null or exists (select 1 from buildings b where b.id = bills.building_id and b.deleted_at is null));

-- name: BillList :many
select * from bills
where bills.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or bills.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and (cardinality(sqlc.arg(ids)::uuid[]) = 0 or bills.id = any(sqlc.arg(ids)::uuid[]))
  and (sqlc.narg(building_id)::uuid is null or bills.building_id = sqlc.narg(building_id))
  and (sqlc.narg(analyzer_id)::uuid is null or bills.analyzer_id = sqlc.narg(analyzer_id))
  and (sqlc.narg(bill_scope)::bill_scope is null or bills.scope = sqlc.narg(bill_scope))
  and (sqlc.narg(period_key)::text is null or bills.period_key = sqlc.narg(period_key))
  -- Statuses is compared as text[], not bill_status[]: pgx has no codec
  -- for an array of this custom enum, and fails even on an empty slice.
  and (cardinality(sqlc.arg(statuses)::text[]) = 0 or bills.status::text = any(sqlc.arg(statuses)::text[]))
  and (sqlc.arg(include_superseded)::boolean or bills.status <> 'superseded')
  and (bills.building_id is null or exists (select 1 from buildings b where b.id = bills.building_id and b.deleted_at is null))
order by bills.period_key desc, bills.id
limit sqlc.arg(limit_val) offset sqlc.arg(offset_val);

-- name: BillCurrent :one
select * from bills
where bills.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or bills.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and bills.scope = sqlc.arg(bill_scope)
  and coalesce(bills.analyzer_id, bills.building_id, bills.company_id) = sqlc.arg(subject_id)
  and bills.period_key = sqlc.arg(period_key)
  and bills.status <> 'superseded'
  and (bills.building_id is null or exists (select 1 from buildings b where b.id = bills.building_id and b.deleted_at is null));

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

-- Critical Finding 1 (task-11a fix round 1): building_id is a stored foreign
-- key and MUST be validated in SQL — Scope.AllowsBuilding cannot know which
-- company owns a building, so the Go-side billBuildingWritable pre-check is
-- not sufficient on its own. Important Finding 4: tariff_id is a stored
-- foreign key too (bills.tariff_id references tariffs), validated the same
-- way TariffVisible validates a tariff for this Scope, INCLUDING the
-- company-wide (building_id is null) case — a narrow Scope may reference the
-- company-wide tariff Effective can already resolve for it.
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
)
select
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
where
  (sqlc.narg(building_id)::uuid is null or exists (
    select 1 from buildings b
    where b.id = sqlc.narg(building_id) and b.company_id = sqlc.arg(company_id) and b.deleted_at is null
      and (sqlc.arg(all_buildings)::boolean or b.id = any(sqlc.arg(building_ids)::uuid[]))
  ))
  and (sqlc.narg(tariff_id)::uuid is null or exists (
    select 1 from tariffs t
    where t.id = sqlc.narg(tariff_id) and t.company_id = sqlc.arg(company_id) and t.deleted_at is null
      and (sqlc.arg(all_buildings)::boolean or t.building_id is null or t.building_id = any(sqlc.arg(building_ids)::uuid[]))
  ))
returning *;

-- Important Finding 1: bill_lines has no company_id of its own — join
-- through bills. BillLineInsert re-validates bill_id against the Scope
-- itself, so the isolation boundary holds even if the Go-side requireVisible
-- pre-check (in BillRepository.requireVisible, run before the transaction
-- Create/Supersede/ReplaceHourlyDetail open) were ever skipped.
-- name: BillLineInsert :one
insert into bill_lines (id, bill_id, code, label, quantity, unit, unit_price, rate_pct, amount, sort_order)
select gen_random_uuid(), sqlc.arg(bill_id), sqlc.arg(code), sqlc.arg(label), sqlc.narg(quantity),
       sqlc.narg(unit), sqlc.narg(unit_price), sqlc.narg(rate_pct), sqlc.arg(amount), sqlc.arg(sort_order)
where exists (
  select 1 from bills bl where bl.id = sqlc.arg(bill_id) and bl.company_id = sqlc.arg(company_id)
    and (sqlc.arg(all_buildings)::boolean or bl.building_id = any(sqlc.arg(building_ids)::uuid[]))
)
returning *;

-- :one RETURNING true, not :exec: see AlarmAnalyzerInsert's comment in
-- queries/alarms.sql -- an :exec insert whose WHERE EXISTS excludes every
-- row still reports "no error", which a caller checking only err would read
-- as success.
-- name: BillMemberInsert :one
insert into bill_members (bill_id, analyzer_id)
select sqlc.arg(bill_id), sqlc.arg(analyzer_id)
where exists (
  select 1 from bills bl where bl.id = sqlc.arg(bill_id) and bl.company_id = sqlc.arg(company_id)
    and (sqlc.arg(all_buildings)::boolean or bl.building_id = any(sqlc.arg(building_ids)::uuid[]))
)
returning true;

-- Supersede's replacement-vs-original check (folded minor, fix round 1):
-- returning the marked bill's own scope, subject and period lets the Go
-- caller refuse a Supersede whose replacement names a different one, instead
-- of silently letting one bill's history point at another's.
-- name: BillMarkSuperseded :one
update bills set status = 'superseded', updated_at = sqlc.arg(updated_at)
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]))
  and status <> 'superseded'
returning scope, coalesce(analyzer_id, building_id, company_id) as subject_id, period_key;

-- Folded minor: a superseded bill is a closed record — UpdateStatus must not
-- be able to move it to draft/issued/flagged after the fact, only Supersede
-- may act on it again (by inserting a new replacement).
-- name: BillUpdateStatus :one
update bills set status = sqlc.arg(status), flag_reason = sqlc.narg(flag_reason), updated_at = sqlc.arg(updated_at)
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]))
  and status <> 'superseded'
returning *;

-- name: BillSetPDFPath :execrows
update bills set pdf_path = sqlc.arg(pdf_path), updated_at = now()
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]));

-- name: BillLineList :many
select bl.* from bill_lines bl
where bl.bill_id = sqlc.arg(bill_id)
  and exists (
    select 1 from bills b where b.id = bl.bill_id and b.company_id = sqlc.arg(company_id)
      and (sqlc.arg(all_buildings)::boolean or b.building_id = any(sqlc.arg(building_ids)::uuid[]))
  )
order by bl.sort_order, bl.id;

-- name: BillMemberList :many
select bm.* from bill_members bm
where bm.bill_id = sqlc.arg(bill_id)
  and exists (
    select 1 from bills b where b.id = bm.bill_id and b.company_id = sqlc.arg(company_id)
      and (sqlc.arg(all_buildings)::boolean or b.building_id = any(sqlc.arg(building_ids)::uuid[]))
  )
order by bm.analyzer_id;

-- name: BillHourlyDetailList :many
select bhd.* from bill_hourly_detail bhd
where bhd.bill_id = sqlc.arg(bill_id)
  and exists (
    select 1 from bills b where b.id = bhd.bill_id and b.company_id = sqlc.arg(company_id)
      and (sqlc.arg(all_buildings)::boolean or b.building_id = any(sqlc.arg(building_ids)::uuid[]))
  )
order by bhd.ts;

-- name: BillHourlyDetailDeleteForBill :exec
delete from bill_hourly_detail
where bill_id = sqlc.arg(bill_id)
  and exists (
    select 1 from bills b where b.id = sqlc.arg(bill_id) and b.company_id = sqlc.arg(company_id)
      and (sqlc.arg(all_buildings)::boolean or b.building_id = any(sqlc.arg(building_ids)::uuid[]))
  );

-- :one RETURNING true, not :exec: see AlarmAnalyzerInsert's comment in
-- queries/alarms.sql -- an :exec insert whose WHERE EXISTS excludes every
-- row still reports "no error", which a caller checking only err would read
-- as success. Task 11a fix round 2: this was the one sibling
-- (AlarmAnalyzerInsert/AlarmChannelInsert/BillMemberInsert were already
-- converted in fix round 1) left as :exec, so a bill belonging to another
-- tenant's company still looked like a successful insert while storing zero
-- rows.
-- name: BillHourlyDetailInsert :one
insert into bill_hourly_detail (bill_id, ts, consumption, ptf, yekdem, kbk, unit_price, cost)
select sqlc.arg(bill_id), sqlc.arg(ts), sqlc.arg(consumption), sqlc.arg(ptf), sqlc.arg(yekdem),
       sqlc.arg(kbk), sqlc.arg(unit_price), sqlc.arg(cost)
where exists (
  select 1 from bills b where b.id = sqlc.arg(bill_id) and b.company_id = sqlc.arg(company_id)
    and (sqlc.arg(all_buildings)::boolean or b.building_id = any(sqlc.arg(building_ids)::uuid[]))
)
returning true;
