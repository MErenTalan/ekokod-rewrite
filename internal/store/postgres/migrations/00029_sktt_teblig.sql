-- +goose Up
-- R565: the day-ahead matched quantity (MWh, EPİAŞ clearing-quantity
-- matchedBids) per hour, so a day's volume-weighted PTF — the
-- "ağırlıklı ortalama günlük PTF" of the SKTT Tebliğ (RG 30307 md. 6) —
-- can be computed. Null for hours fetched before this column existed.
alter table market_prices_hourly add column matched_quantity numeric(14,4);

-- R566: the EPDK forecast ("öngörülen") YEKDEM unit cost per month, which
-- the SKTT Tebliğ prices with; platform-wide like yekdem_monthly.
create table yekdem_forecast (
    year       smallint      not null,
    month      smallint      not null check (month between 1 and 12),
    value      numeric(14,4) not null,  -- TL/MWh
    source     text          not null,
    updated_at timestamptz   not null default now(),
    primary key (year, month)
);

-- EPDK decision 14718 (02.07.2026), revising July–December 2026.
insert into yekdem_forecast (year, month, value, source) values
    (2026,  7, 423.99, 'EPDK 14718'),
    (2026,  8, 450.45, 'EPDK 14718'),
    (2026,  9, 581.14, 'EPDK 14718'),
    (2026, 10, 526.39, 'EPDK 14718'),
    (2026, 11, 488.86, 'EPDK 14718'),
    (2026, 12, 393.75, 'EPDK 14718');

-- +goose Down
drop table if exists yekdem_forecast;
alter table market_prices_hourly drop column if exists matched_quantity;
