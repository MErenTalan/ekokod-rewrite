-- GenerationRepository's queries. generation_anchors has no company_id (see
-- repository.go's "ROWS WITHOUT company_id" header): every method joins
-- through analyzers, or — for the write — validates the analyzer's
-- visibility IN the write statement itself.

-- name: GenerationAnchorGet :one
-- A single query covers BOTH "analyzer not visible" and "no anchor yet":
-- both yield zero rows, and repository.go's GenerationRepository.Anchor doc
-- says both return ErrNotFound.
select ga.* from generation_anchors ga
join analyzers a on a.id = ga.analyzer_id
where ga.analyzer_id = sqlc.arg(analyzer_id)
  and a.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and a.deleted_at is null;

-- name: GenerationAnchorUpsert :execrows
-- The select-from-analyzers source IS the visibility check (F1 ba2fb97
-- pattern, repeated from CursorRecordSuccess): an invisible analyzer_id
-- makes the source empty, so nothing is inserted or updated and execrows
-- reports 0, which the repository reads as ErrNotFound. This is the tenant
-- predicate for this write, embedded in the write statement itself — never a
-- Scope.AllowsBuilding pre-check against the stored analyzer_id, which would
-- be using AllowsBuilding to validate a stored foreign key (Scope.
-- AllowsBuilding's own doc explains why that is unsafe).
--
-- One atomic statement upserts on analyzer_id: a fresh reconciliation always
-- replaces the prior anchor rather than accumulating history.
--
-- updated_at is owned by SQL, never the caller: the insert list omits it
-- entirely (the column default `now()` stamps a fresh row) and the conflict
-- branch stamps it explicitly with `now()` — never `excluded.updated_at`,
-- which would let a caller backdate or forward-date the bookkeeping
-- timestamp with its own wall clock.
insert into generation_anchors (analyzer_id, anchor_ts, active_export, source)
select a.id, sqlc.arg(anchor_ts)::timestamptz, sqlc.arg(active_export)::numeric, sqlc.arg(source)::text
from analyzers a
where a.id = sqlc.arg(analyzer_id)
  and a.company_id = sqlc.arg(company_id)
  and a.deleted_at is null
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
on conflict (analyzer_id) do update set
    anchor_ts     = excluded.anchor_ts,
    active_export = excluded.active_export,
    source        = excluded.source,
    updated_at    = now();
