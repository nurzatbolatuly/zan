-- Колонки возвращаются пустыми: значения, удалённые up-миграцией, не
-- восстанавливаются (оплаты по-прежнему есть в core.payments).
ALTER TABLE core.threads
    ADD COLUMN is_paid    boolean NOT NULL DEFAULT false,
    ADD COLUMN paid_at    timestamptz,
    ADD COLUMN free_until timestamptz,
    ADD COLUMN closed_at  timestamptz;
