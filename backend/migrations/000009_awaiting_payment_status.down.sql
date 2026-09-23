-- Возвращённые up-миграцией кредиты за queued+is_paid треды не
-- отзываются — какие треды были оплачены, после up не известно.
ALTER TABLE core.threads
    DROP CONSTRAINT threads_status_check;

UPDATE core.threads
SET status       = 'queued',
    preview_text = 'Запрос в очереди…'
WHERE status = 'awaiting_payment';

ALTER TABLE core.threads
    ALTER COLUMN status SET DEFAULT 'queued',
    ADD CONSTRAINT threads_status_check
        CHECK (status IN ('queued', 'processing', 'done', 'error', 'canceled'));
