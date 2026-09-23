-- +goose Up
-- R237: the screen that enqueued a bill computation needs to know WHY it
-- failed, and the reason lives in job_runs.detail as an R113 code. asynq's
-- task id is the only handle the screen holds, so the run records it. The id
-- is deterministic per (scope, subject, period), so it is not unique over
-- time: a later recomputation of the same period writes another row with the
-- same task id, and the newest one is the answer.
alter table job_runs add column task_id text;
create index on job_runs (task_id, started_at desc) where task_id is not null;

-- +goose Down
drop index if exists job_runs_task_id_started_at_idx;
alter table job_runs drop column if exists task_id;
