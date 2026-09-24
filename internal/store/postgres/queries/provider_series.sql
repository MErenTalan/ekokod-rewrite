-- ProviderSeriesRepository's queries. provider_hourly_values has no
-- company_id (see repository.go's "ROWS WITHOUT company_id" header): every
-- method joins through analyzers. The bulk COPY-into-staging-then-upsert
-- path UpsertHourly needs is NOT here — a per-call temporary table cannot
-- appear in sqlc's schema catalogue (it does not exist until the
-- transaction that creates it), so that part runs as plain SQL in
-- provider_series.go, mirroring readings.go's BulkInsert exactly. What IS
-- here for it is ProviderHourlyVisibleAnalyzerIDs, the `for share` row lock
-- that is THE tenant predicate for that write, evaluated once per batch.

-- name: ProviderHourlyVisibleAnalyzerIDs :many
-- Mirrors ReadingVisibleAnalyzerIDs exactly: returns the subset of
-- analyzer_ids visible to the scope, AND LOCKS them (`for share`) for the
-- rest of the caller's transaction, so a concurrent reassignment or
-- soft-delete cannot open a gap between this check and the write that
-- follows it. UpsertHourly compares the result against the DISTINCT
-- analyzer ids in the batch; a mismatch means at least one row's analyzer is
-- not visible, and the whole batch is refused.
select id from analyzers
where id = any(sqlc.arg(analyzer_ids)::uuid[])
  and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]))
  and deleted_at is null
order by id
for share;

-- name: ProviderHourlyAnalyzerVisible :one
-- Used ONLY to pick an error, never to gate a data read: HourlyRange runs
-- its own scoped query first and unconditionally, and calls this only when
-- that query came back empty, to tell "the analyzer itself is not visible"
-- (ErrNotFound) from "visible, nothing in this window" (empty slice) — the
-- same pattern ReadingAnalyzerVisible established for ReadingRepository.
select exists (
    select 1 from analyzers
    where id = sqlc.arg(analyzer_id)
      and company_id = sqlc.arg(company_id)
      and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]))
      and deleted_at is null
);

-- name: ProviderHourlyRange :many
select phv.* from provider_hourly_values phv
join analyzers a on a.id = phv.analyzer_id
where phv.analyzer_id = sqlc.arg(analyzer_id)
  and phv.ts >= sqlc.arg(from_ts)::timestamptz
  and phv.ts < sqlc.arg(to_ts)::timestamptz
  and a.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and a.deleted_at is null
order by phv.ts;
