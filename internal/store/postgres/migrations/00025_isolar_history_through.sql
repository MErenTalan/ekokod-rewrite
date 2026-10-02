-- +goose Up
-- R506: the Istanbul day through which a linked plant's stored production is
-- whole. NULL means its history has not arrived yet; a sync fetches the days
-- after it. The 400-day history used to be asked for once, when the plant
-- was linked, and a failed first sync left it missing for good.
alter table power_plants add column isolar_history_through date;

-- +goose Down
alter table power_plants drop column if exists isolar_history_through;
