-- AdminMarketDataRepository's queries (store.AdminMarketDataRepository,
-- implemented in internal/store/postgres/admin/marketdata.go). Neither table
-- carries a company_id: these writes are platform-wide by construction, so
-- there is no Scope to check here — see repository.go's admin section header.

-- name: AdminMarketDataUpsertHourlyPrices :execrows
insert into market_prices_hourly (ts, ptf, matched_quantity, fetched_at)
select unnest(sqlc.arg(ts)::timestamptz[]),
       unnest(sqlc.arg(ptf)::numeric[]),
       unnest(sqlc.arg(matched_quantity)::numeric[]),
       unnest(sqlc.arg(fetched_at)::timestamptz[])
on conflict (ts) do update set
    ptf = excluded.ptf,
    matched_quantity = coalesce(excluded.matched_quantity, market_prices_hourly.matched_quantity),
    fetched_at = excluded.fetched_at;

-- name: AdminMarketDataUpsertYekdem :execrows
insert into yekdem_monthly (year, month, value, fetched_at)
select unnest(sqlc.arg(year)::smallint[]),
       unnest(sqlc.arg(month)::smallint[]),
       unnest(sqlc.arg(value)::numeric[]),
       unnest(sqlc.arg(fetched_at)::timestamptz[])
on conflict (year, month) do update set
    value = excluded.value,
    fetched_at = excluded.fetched_at;
