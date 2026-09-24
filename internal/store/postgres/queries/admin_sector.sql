-- AdminSectorRepository queries: sector peers across every tenant (R162).
-- Only figures leave this query; the service never exposes peer identities.

-- name: AdminSectorConsumption :many
-- Per analyzer, consumption over [from, to) is the last day's closing index
-- minus the first day's opening index (clamped at zero, so a meter reset
-- cannot produce a negative figure); a building sums its analyzers. days is
-- the most days any of its analyzers reported.
with peers as (
    select b.id as building_id, b.personnel_count, b.total_area_m2
    from buildings b
    join companies c on c.id = b.company_id
    where b.deleted_at is null and c.deleted_at is null and b.sector is not null
      and lower(btrim(b.sector)) = lower(btrim(sqlc.arg(sector)::text))
),
per_analyzer as (
    select a.building_id,
           greatest((array_agg(d.active_index order by d.bucket desc))[1]
                    - (array_agg(d.active_import_start order by d.bucket asc))[1], 0)::numeric as consumption,
           count(*)::integer as days
    from consumption_daily d
    join analyzers a on a.id = d.analyzer_id and a.deleted_at is null
    join peers p on p.building_id = a.building_id
    where d.bucket >= sqlc.arg(from_ts)::timestamptz and d.bucket < sqlc.arg(to_ts)::timestamptz
    group by a.building_id, d.analyzer_id
)
select p.building_id,
       p.personnel_count,
       p.total_area_m2,
       (select sum(x.consumption) from per_analyzer x where x.building_id = p.building_id)::numeric as consumption,
       coalesce((select max(x.days) from per_analyzer x where x.building_id = p.building_id), 0)::integer as days
from peers p
order by p.building_id;
