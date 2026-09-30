-- +goose Up
-- A backfill or analyzer refresh only enqueues fetch_readings tasks; the
-- screen that started it needs their outcome, so each fetch run records the
-- task id of the job that enqueued it.
alter table job_runs add column parent_task_id text;
create index on job_runs (company_id, parent_task_id) where parent_task_id is not null;

-- +goose Down
drop index if exists job_runs_company_id_parent_task_id_idx;
alter table job_runs drop column if exists parent_task_id;
