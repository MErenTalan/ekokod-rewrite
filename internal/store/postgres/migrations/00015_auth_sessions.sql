-- +goose Up
-- F6a auth: session rotation bookkeeping (R141), per-user UI preferences
-- (R169) and single-use password reset tokens (R148).

alter table sessions
    add column client         text    not null default 'web' check (client in ('web','mobile')),
    add column remember       boolean not null default false,
    add column last_used_at   timestamptz,
    add column revoked_reason text check (revoked_reason in
        ('rotated','logout','logout_all','password_change','device_mismatch','reuse_detected','admin_change')),
    add column rotated_from   uuid references sessions(id) on delete set null;

alter table users
    add column ui_preferences text check (char_length(ui_preferences) <= 512),
    add column locale         text not null default 'tr' check (locale in ('tr','en'));

create table password_reset_tokens (
    id         uuid primary key default gen_random_uuid(),
    user_id    uuid        not null references users(id) on delete cascade,
    token_hash text        not null unique,
    expires_at timestamptz not null,
    used_at    timestamptz,
    created_at timestamptz not null default now()
);
create index on password_reset_tokens (user_id, created_at desc);

-- +goose Down
drop table if exists password_reset_tokens;
alter table users
    drop column if exists locale,
    drop column if exists ui_preferences;
alter table sessions
    drop column if exists rotated_from,
    drop column if exists revoked_reason,
    drop column if exists last_used_at,
    drop column if exists remember,
    drop column if exists client;
