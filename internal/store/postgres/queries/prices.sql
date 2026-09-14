-- PriceRepository's queries, over the two PLATFORM-WIDE tables
-- market_prices_hourly and yekdem_monthly. Neither carries a company_id, so
-- the Scope narrows nothing here; it is still validated by the repository
-- before either query runs (repository.go's PriceRepository doc).

-- name: PriceHourlyRange :many
select * from market_prices_hourly
where ts >= sqlc.arg(from_ts)::timestamptz
  and ts < sqlc.arg(to_ts)::timestamptz
order by ts;

-- name: PriceYekdem :one
select * from yekdem_monthly
where year = sqlc.arg(year) and month = sqlc.arg(month);
