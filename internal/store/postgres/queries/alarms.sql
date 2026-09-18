-- Alarms, their attachments and their firing history (migration 00011).
-- alarms itself carries only company_id — no building_id — so a Scope
-- narrows it to the company and no further, exactly as PlantRepository does
-- for power_plants. Query names are prefixed Alarm…

-- name: AlarmGet :one
select * from alarms
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null;

-- Important Finding 2 (task-11a fix round 1): the alarm RULE is company-wide
-- (accepted), but its building-scoped children are not — the AnalyzerID
-- filter must only match an alarm through an ATTACHMENT whose analyzer is
-- itself visible to the Scope, or a narrow Scope could use this filter as an
-- oracle for which analyzers exist in Buildings[1].
-- name: AlarmList :many
select * from alarms
where alarms.company_id = sqlc.arg(company_id)
  and (cardinality(sqlc.arg(ids)::uuid[]) = 0 or alarms.id = any(sqlc.arg(ids)::uuid[]))
  -- Types is compared as text[], not alarm_type[]: pgx has no codec for an
  -- array of this custom enum, and fails even on an empty slice.
  and (cardinality(sqlc.arg(types)::text[]) = 0 or alarms.type::text = any(sqlc.arg(types)::text[]))
  and (sqlc.narg(is_enabled)::boolean is null or alarms.is_enabled = sqlc.narg(is_enabled))
  and (not sqlc.arg(filter_analyzer)::boolean
       or exists (
         select 1 from alarm_analyzers aa
         join analyzers an on an.id = aa.analyzer_id
         where aa.alarm_id = alarms.id and aa.analyzer_id = sqlc.arg(analyzer_id)
           and an.company_id = sqlc.arg(company_id) and an.deleted_at is null
           and (sqlc.arg(all_buildings)::boolean or an.building_id = any(sqlc.arg(building_ids)::uuid[]))
       ))
  and (sqlc.arg(include_deleted)::boolean or alarms.deleted_at is null)
order by alarms.name, alarms.id
limit sqlc.arg(limit_val) offset sqlc.arg(offset_val);

-- name: AlarmVisible :one
select exists (
  select 1 from alarms where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null
) as visible;

-- name: AlarmCreate :one
insert into alarms (
  id, company_id, name, type, is_enabled,
  inductive_ratio_threshold, inductive_period_value, inductive_period_unit,
  capacitive_ratio_threshold, capacitive_period_value, capacitive_period_unit,
  active_consumption_max, active_consumption_max_period_value, active_consumption_max_period_unit,
  active_consumption_min, active_consumption_min_period_value, active_consumption_min_period_unit,
  communication_threshold_hours, voltage_max, voltage_min, power_max, power_min,
  invoice_threshold_pct, notification_frequency_value, notification_frequency_unit,
  created_at, updated_at
) values (
  gen_random_uuid(), sqlc.arg(company_id), sqlc.arg(name), sqlc.arg(type), sqlc.arg(is_enabled),
  sqlc.narg(inductive_ratio_threshold), sqlc.narg(inductive_period_value), sqlc.narg(inductive_period_unit),
  sqlc.narg(capacitive_ratio_threshold), sqlc.narg(capacitive_period_value), sqlc.narg(capacitive_period_unit),
  sqlc.narg(active_consumption_max), sqlc.narg(active_consumption_max_period_value), sqlc.narg(active_consumption_max_period_unit),
  sqlc.narg(active_consumption_min), sqlc.narg(active_consumption_min_period_value), sqlc.narg(active_consumption_min_period_unit),
  sqlc.narg(communication_threshold_hours), sqlc.narg(voltage_max), sqlc.narg(voltage_min),
  sqlc.narg(power_max), sqlc.narg(power_min), sqlc.narg(invoice_threshold_pct),
  sqlc.narg(notification_frequency_value), sqlc.narg(notification_frequency_unit),
  sqlc.arg(created_at), sqlc.arg(created_at)
) returning *;

-- name: AlarmUpdate :one
update alarms set
  name = sqlc.arg(name), type = sqlc.arg(type), is_enabled = sqlc.arg(is_enabled),
  inductive_ratio_threshold = sqlc.narg(inductive_ratio_threshold),
  inductive_period_value = sqlc.narg(inductive_period_value),
  inductive_period_unit = sqlc.narg(inductive_period_unit),
  capacitive_ratio_threshold = sqlc.narg(capacitive_ratio_threshold),
  capacitive_period_value = sqlc.narg(capacitive_period_value),
  capacitive_period_unit = sqlc.narg(capacitive_period_unit),
  active_consumption_max = sqlc.narg(active_consumption_max),
  active_consumption_max_period_value = sqlc.narg(active_consumption_max_period_value),
  active_consumption_max_period_unit = sqlc.narg(active_consumption_max_period_unit),
  active_consumption_min = sqlc.narg(active_consumption_min),
  active_consumption_min_period_value = sqlc.narg(active_consumption_min_period_value),
  active_consumption_min_period_unit = sqlc.narg(active_consumption_min_period_unit),
  communication_threshold_hours = sqlc.narg(communication_threshold_hours),
  voltage_max = sqlc.narg(voltage_max), voltage_min = sqlc.narg(voltage_min),
  power_max = sqlc.narg(power_max), power_min = sqlc.narg(power_min),
  invoice_threshold_pct = sqlc.narg(invoice_threshold_pct),
  notification_frequency_value = sqlc.narg(notification_frequency_value),
  notification_frequency_unit = sqlc.narg(notification_frequency_unit),
  updated_at = sqlc.arg(updated_at)
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null
returning *;

-- name: AlarmSoftDelete :execrows
update alarms set deleted_at = sqlc.arg(deleted_at)
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null;

-- Important Finding 2: Analyzers only lists ATTACHMENTS whose analyzer is
-- itself visible to the Scope — a narrow Scope must never learn (via this
-- method) that an alarm is watching an analyzer in a building it cannot see.
-- name: AlarmAnalyzerList :many
select aa.* from alarm_analyzers aa
join analyzers an on an.id = aa.analyzer_id
where aa.alarm_id = sqlc.arg(alarm_id)
  and an.company_id = sqlc.arg(company_id) and an.deleted_at is null
  and (sqlc.arg(all_buildings)::boolean or an.building_id = any(sqlc.arg(building_ids)::uuid[]))
order by aa.analyzer_id;

-- Important Finding 2: ReplaceAnalyzers must replace ONLY the attachments
-- visible to the Scope and leave attachments to analyzers outside it
-- untouched — a narrow Scope replacing "its" analyzer list must not be able
-- to silently detach an analyzer in a building it cannot see.
-- name: AlarmAnalyzerDeleteVisibleForAlarm :exec
delete from alarm_analyzers aa
using analyzers an
where aa.analyzer_id = an.id and aa.alarm_id = sqlc.arg(alarm_id)
  and an.company_id = sqlc.arg(company_id) and an.deleted_at is null
  and (sqlc.arg(all_buildings)::boolean or an.building_id = any(sqlc.arg(building_ids)::uuid[]));

-- Important Finding 1: alarm_analyzers has no company_id of its own —
-- AlarmAnalyzerInsert re-validates both the alarm and the analyzer against
-- the Scope itself, so the isolation boundary holds even if the Go-side
-- requireVisible / AlarmCountVisibleAnalyzers pre-checks were ever skipped.
--
-- :one RETURNING true, not :exec: an :exec insert whose WHERE EXISTS excludes
-- every row still reports "no error" -- a caller checking only err would read
-- a silently-guarded no-op as success. RETURNING true turns that into
-- pgx.ErrNoRows (-> store.ErrNotFound via pgerr.Translate), which is what a
-- guard-failure proof with the Go-side pre-checks bypassed actually needs:
-- not just "nothing was written" but a caller-visible refusal.
-- name: AlarmAnalyzerInsert :one
insert into alarm_analyzers (alarm_id, analyzer_id)
select sqlc.arg(alarm_id), sqlc.arg(analyzer_id)
where exists (select 1 from alarms a where a.id = sqlc.arg(alarm_id) and a.company_id = sqlc.arg(company_id) and a.deleted_at is null)
  and exists (
    select 1 from analyzers an
    where an.id = sqlc.arg(analyzer_id) and an.company_id = sqlc.arg(company_id) and an.deleted_at is null
      and (sqlc.arg(all_buildings)::boolean or an.building_id = any(sqlc.arg(building_ids)::uuid[]))
  )
returning true;

-- name: AlarmCountVisibleAnalyzers :one
select count(*) from analyzers
where company_id = sqlc.arg(company_id) and deleted_at is null
  and id = any(sqlc.arg(analyzer_ids)::uuid[])
  and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]));

-- Important Finding 1: alarm_channels has no company_id of its own — every
-- statement below re-validates alarm_id against the Scope itself.
-- name: AlarmChannelList :many
select ac.* from alarm_channels ac
where ac.alarm_id = sqlc.arg(alarm_id)
  and exists (select 1 from alarms a where a.id = ac.alarm_id and a.company_id = sqlc.arg(company_id) and a.deleted_at is null)
order by ac.channel, ac.target;

-- name: AlarmChannelDeleteForAlarm :exec
delete from alarm_channels
where alarm_id = sqlc.arg(alarm_id)
  and exists (select 1 from alarms a where a.id = sqlc.arg(alarm_id) and a.company_id = sqlc.arg(company_id) and a.deleted_at is null);

-- Same :one/RETURNING true reasoning as AlarmAnalyzerInsert above.
-- name: AlarmChannelInsert :one
insert into alarm_channels (alarm_id, channel, target)
select sqlc.arg(alarm_id), sqlc.arg(channel), sqlc.arg(target)
where exists (select 1 from alarms a where a.id = sqlc.arg(alarm_id) and a.company_id = sqlc.arg(company_id) and a.deleted_at is null)
returning true;

-- Important Finding 1 and 2: alarm_events has no company_id of its own, and
-- e.AnalyzerID (when set) must itself be visible to the Scope — validated
-- here, in the write, not only by the Go-side pre-checks that run before it.
-- name: AlarmEventCreate :one
insert into alarm_events (id, alarm_id, analyzer_id, triggered_at, message, detail, notified_at, notification_error)
select gen_random_uuid(), sqlc.arg(alarm_id), sqlc.narg(analyzer_id), sqlc.arg(triggered_at),
       sqlc.arg(message), sqlc.narg(detail), sqlc.narg(notified_at), sqlc.narg(notification_error)
where exists (select 1 from alarms a where a.id = sqlc.arg(alarm_id) and a.company_id = sqlc.arg(company_id) and a.deleted_at is null)
  and (sqlc.narg(analyzer_id)::uuid is null or exists (
    select 1 from analyzers an
    where an.id = sqlc.narg(analyzer_id) and an.company_id = sqlc.arg(company_id) and an.deleted_at is null
      and (sqlc.arg(all_buildings)::boolean or an.building_id = any(sqlc.arg(building_ids)::uuid[]))
  ))
returning *;

-- Important Finding 2: an event's analyzer must be visible to the Scope, and
-- an event with NO analyzer (a company-level condition) is visible only to a
-- Scope with AllBuildings — otherwise this method would hand a narrow Scope
-- another building's analyzer id and the event's own message/detail payload.
--
-- F1 final review pass A, Important Finding 3 (fix round): a.deleted_at is
-- null was missing — a soft-deleted alarm's events stayed listable, in
-- violation of ruling 4 (a child of a soft-deleted parent must not be
-- reachable) and of AlarmVisible's own treatment of a soft-deleted alarm as
-- not visible.
-- name: AlarmEventList :many
select ae.* from alarm_events ae
join alarms a on a.id = ae.alarm_id
left join analyzers an on an.id = ae.analyzer_id
where a.company_id = sqlc.arg(company_id) and a.deleted_at is null
  and (sqlc.narg(alarm_id)::uuid is null or ae.alarm_id = sqlc.narg(alarm_id))
  and (sqlc.narg(analyzer_id)::uuid is null or ae.analyzer_id = sqlc.narg(analyzer_id))
  and (not sqlc.arg(undelivered)::boolean or ae.notified_at is null)
  -- R218: the frequency check asks the opposite question of
  -- `undelivered` — was anyone actually told, and when?
  and (sqlc.narg(notified)::boolean is null
       or (sqlc.narg(notified)::boolean and ae.notified_at is not null)
       or (not sqlc.narg(notified)::boolean and ae.notified_at is null))
  and (sqlc.narg(range_from)::timestamptz is null or ae.triggered_at >= sqlc.narg(range_from))
  and (sqlc.narg(range_to)::timestamptz is null or ae.triggered_at < sqlc.narg(range_to))
  and (
    (ae.analyzer_id is null and sqlc.arg(all_buildings)::boolean)
    or (ae.analyzer_id is not null and an.company_id = sqlc.arg(company_id) and an.deleted_at is null
        and (sqlc.arg(all_buildings)::boolean or an.building_id = any(sqlc.arg(building_ids)::uuid[])))
  )
order by ae.triggered_at desc, ae.id
limit sqlc.arg(limit_val) offset sqlc.arg(offset_val);

-- F1 final review pass A, Important Finding 3 (fix round): MarkNotified
-- previously checked only alarms.company_id — no a.deleted_at guard (a
-- soft-deleted alarm's events stayed writable) and no building/analyzer
-- visibility disjunction at all (a narrow Scope could mark ANY event in the
-- company notified, including one on an analyzer outside its buildings, or
-- a NULL-analyzer company-level event it can't even list). This now mirrors
-- AlarmEventList's own visibility rule exactly, so an event unreachable by
-- ListEvents is unreachable by MarkNotified too.
-- name: AlarmMarkNotified :execrows
update alarm_events ae set notified_at = sqlc.arg(notified_at), notification_error = sqlc.narg(notification_error)
where ae.id = sqlc.arg(id)
  and exists (select 1 from alarms a where a.id = ae.alarm_id and a.company_id = sqlc.arg(company_id) and a.deleted_at is null)
  and (
    (ae.analyzer_id is null and sqlc.arg(all_buildings)::boolean)
    or (ae.analyzer_id is not null and exists (
          select 1 from analyzers an
          where an.id = ae.analyzer_id and an.company_id = sqlc.arg(company_id) and an.deleted_at is null
            and (sqlc.arg(all_buildings)::boolean or an.building_id = any(sqlc.arg(building_ids)::uuid[]))
        ))
  );

-- Important Finding 1: alarm_fired_bills has no company_id of its own and
-- names TWO parents (alarms and bills) — both are re-validated against the
-- Scope here, not only by the Go-side requireVisible/BillVisible pre-checks.
-- name: AlarmMarkBillFired :one
insert into alarm_fired_bills (alarm_id, bill_id)
select sqlc.arg(alarm_id), sqlc.arg(bill_id)
where exists (select 1 from alarms a where a.id = sqlc.arg(alarm_id) and a.company_id = sqlc.arg(company_id) and a.deleted_at is null)
  and exists (
    select 1 from bills b where b.id = sqlc.arg(bill_id) and b.company_id = sqlc.arg(company_id)
      and (sqlc.arg(all_buildings)::boolean or b.building_id = any(sqlc.arg(building_ids)::uuid[]))
  )
on conflict do nothing
returning true;

-- Important Finding 1: isolar_forwarded_alarms has no company_id of its own
-- — re-validated against power_plants here, not only by the Go-side
-- AlarmPlantVisible pre-check.
-- name: AlarmMarkIsolarForwarded :one
insert into isolar_forwarded_alarms (plant_id, alarm_ref, sent_at)
select sqlc.arg(plant_id), sqlc.arg(alarm_ref), sqlc.arg(sent_at)
where exists (
  select 1 from power_plants p where p.id = sqlc.arg(plant_id) and p.company_id = sqlc.arg(company_id) and p.deleted_at is null
)
on conflict do nothing
returning true;

-- name: AlarmPlantVisible :one
select exists (
  select 1 from power_plants where id = sqlc.arg(plant_id) and company_id = sqlc.arg(company_id) and deleted_at is null
) as visible;

-- name: AdminCompaniesWithEnabledAlarms :many
-- Every live company with at least one enabled rule. The dispatcher uses it so
-- a company with no alarms never gets an empty run (R219).
select distinct a.company_id from alarms a
join companies c on c.id = a.company_id and c.deleted_at is null
where a.deleted_at is null and a.is_enabled
order by a.company_id;
