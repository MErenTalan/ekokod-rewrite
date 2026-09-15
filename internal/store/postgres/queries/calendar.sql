-- Calendar, weekend-day and vacation repository queries (migration 00008).
-- Owned by Task 11c.
--
-- calendar_events, company_weekend_days and company_vacations all carry
-- company_id directly and have no building_id at all (like stored_files), so
-- every query below scopes to company_id alone -- there is no building set to
-- intersect against.

-- name: CalendarGetEvent :one
select * from calendar_events where id = $1 and company_id = $2;

-- ListEvents narrows by starts_at, the same single-column range convention
-- OpsRepository.ListRuns/ListMessages use (and the column calendar_events'
-- own index is built on): an event whose window merely overlaps [from, to)
-- without STARTING inside it is not returned.
-- name: CalendarListEvents :many
select * from calendar_events
where company_id = sqlc.arg(company_id)::uuid
  and (sqlc.narg(range_from)::timestamptz is null or starts_at >= sqlc.narg(range_from)::timestamptz)
  and (sqlc.narg(range_to)::timestamptz is null or starts_at < sqlc.narg(range_to)::timestamptz)
order by starts_at, id
limit sqlc.arg(limit_val)::int offset sqlc.arg(offset_val)::int;

-- created_by, when set, must name a user of company_id: the `is null or
-- exists (...)` clause makes that atomic with the insert, rather than a
-- separate check-then-insert (R1: stored foreign keys are validated in SQL,
-- never via Scope.AllowsBuilding or a Go-side pre-check).
-- name: CalendarCreateEvent :one
insert into calendar_events (id, company_id, title, starts_at, ends_at, all_day, colour, created_by)
select
    coalesce(nullif(sqlc.arg(id)::uuid, '00000000-0000-0000-0000-000000000000'::uuid),
             gen_random_uuid()),
    sqlc.arg(company_id)::uuid, sqlc.arg(title), sqlc.arg(starts_at)::timestamptz,
    sqlc.arg(ends_at)::timestamptz, sqlc.arg(all_day)::boolean, sqlc.narg(colour)::text,
    sqlc.narg(created_by)::uuid
where (
    sqlc.narg(created_by)::uuid is null
    or exists (
        select 1 from users u
        where u.id = sqlc.narg(created_by)::uuid
          and u.company_id = sqlc.arg(company_id)::uuid
          and u.deleted_at is null
    )
)
returning *;

-- UpdateEvent replaces the event's mutable content -- title, starts_at,
-- ends_at, all_day, colour and created_by. id, company_id and created_at
-- never change. created_by is re-validated exactly as CalendarCreateEvent
-- validates it, so a caller cannot smuggle in another company's user through
-- an update that a create would have refused.
-- name: CalendarUpdateEvent :one
update calendar_events set
    title = sqlc.arg(title),
    starts_at = sqlc.arg(starts_at)::timestamptz,
    ends_at = sqlc.arg(ends_at)::timestamptz,
    all_day = sqlc.arg(all_day)::boolean,
    colour = sqlc.narg(colour)::text,
    created_by = sqlc.narg(created_by)::uuid
where id = sqlc.arg(id)::uuid and company_id = sqlc.arg(company_id)::uuid
  and (
    sqlc.narg(created_by)::uuid is null
    or exists (
        select 1 from users u
        where u.id = sqlc.narg(created_by)::uuid
          and u.company_id = sqlc.arg(company_id)::uuid
          and u.deleted_at is null
    )
  )
returning *;

-- name: CalendarDeleteEvent :execrows
delete from calendar_events where id = $1 and company_id = $2;

-- name: CalendarListWeekendDays :many
select * from company_weekend_days where company_id = $1 order by day_of_week;

-- name: CalendarDeleteWeekendDays :exec
delete from company_weekend_days where company_id = $1;

-- CalendarInsertWeekendDays is ReplaceWeekendDays' insert half, always run in
-- the same transaction right after CalendarDeleteWeekendDays clears the
-- company's current set (CalendarRepository.inTx). `on conflict do nothing`
-- absorbs a caller-supplied duplicate day rather than rejecting it: the
-- method replaces a SET of weekend days, and two copies of the same day are
-- not meaningfully different from that day named once.
-- name: CalendarInsertWeekendDays :exec
insert into company_weekend_days (company_id, day_of_week)
select sqlc.arg(company_id)::uuid, d
from unnest(sqlc.arg(days)::smallint[]) as d
on conflict (company_id, day_of_week) do nothing;

-- Vacations returns every vacation when from_date/to_date are both null.
-- When set they narrow to vacations OVERLAPPING [from_date, to_date):
-- end_date >= from_date and start_date < to_date, half-open the same way
-- store.TimeRange itself is, applied to the vacation's own inclusive
-- [start_date, end_date] span.
-- name: CalendarListVacations :many
select * from company_vacations
where company_id = sqlc.arg(company_id)::uuid
  and (sqlc.narg(from_date)::date is null or end_date >= sqlc.narg(from_date)::date)
  and (sqlc.narg(to_date)::date is null or start_date < sqlc.narg(to_date)::date)
order by start_date, id;

-- name: CalendarCreateVacation :one
insert into company_vacations (id, company_id, start_date, end_date, description)
values (
    coalesce(nullif(sqlc.arg(id)::uuid, '00000000-0000-0000-0000-000000000000'::uuid),
             gen_random_uuid()),
    sqlc.arg(company_id)::uuid, sqlc.arg(start_date)::date, sqlc.arg(end_date)::date,
    sqlc.narg(description)::text
)
returning *;

-- name: CalendarDeleteVacation :execrows
delete from company_vacations where id = $1 and company_id = $2;
