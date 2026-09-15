-- +goose Up
-- PM5340's interval energy (06 §5, R1) and the generation anchor table it is
-- reconciled against. Transcribed from
-- .superpowers/sdd/2026-09-15-f2-integration-layer/task-5-brief.md, which is
-- authoritative for these two objects' shape.
--
-- interval_generation_kwh is added to meter_readings — the one column every
-- other provider leaves NULL — rather than to a side table, because it is a
-- per-reading value at the same (analyzer_id, ts, kind) grain as every other
-- register on that row, and 04-data-model.md's registers are all columns of
-- meter_readings for the same reason. A side table would duplicate the
-- primary key and let the two halves of one reading drift out of sync.
alter table meter_readings add column interval_generation_kwh numeric(18,4);

-- generation_anchors holds the last known cumulative export register value
-- for one analyzer, at one instant: the reference point PM5340's interval
-- energy is reconciled against when a gap in interval reporting would
-- otherwise be silent. One anchor per analyzer — a fresh reconciliation
-- upserts on analyzer_id rather than accumulating history, which is why the
-- primary key is analyzer_id alone rather than (analyzer_id, anchor_ts).
create table generation_anchors (
    analyzer_id   uuid primary key references analyzers(id),
    anchor_ts     timestamptz   not null,
    active_export numeric(18,4) not null check (active_export >= 0),
    source        text          not null check (source in ('initial','operator','migration')),
    updated_at    timestamptz   not null default now()
);

-- +goose Down
-- Reverse dependency order: generation_anchors carries no dependents, and
-- meter_readings' new column is dropped last so a partial rollback (this
-- migration failing part-way, then retried) never leaves the anchor table
-- referencing a meter_readings shape that no longer exists.
drop table if exists generation_anchors;
alter table meter_readings drop column if exists interval_generation_kwh;
