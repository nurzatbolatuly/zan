-- Статус queued заменён на awaiting_payment (instructions.md §2 «Ключевая
-- бизнес-логика»): в БД queued всегда означал «тред сохранён, но не оплачен»,
-- теперь это названо прямо и видно в истории. Любой тред рождается в
-- awaiting_payment и сразу пытается оплатиться с баланса
-- (thread.Service.activateFromBalance).
--
-- queued + is_paid=true мог остаться только после падения процесса между
-- списанием кредита и стартом обработки (старый CreateThread). Такой тред
-- в новой модели не активируется (ActivatePaid требует is_paid=false),
-- поэтому списанная единица возвращается на баланс, а тред становится
-- обычным неоплаченным — пользователь перезапустит вопрос сам.
INSERT INTO core.user_credits (id, session_id, service_id, quantity, updated_at)
SELECT gen_random_uuid(), session_id, service_id, count(*), now()
FROM core.threads
WHERE status = 'queued' AND is_paid
GROUP BY session_id, service_id
ON CONFLICT (session_id, service_id)
DO UPDATE SET quantity = core.user_credits.quantity + EXCLUDED.quantity, updated_at = now();

-- Старый CHECK снимается ДО UPDATE: он не допускает 'awaiting_payment'.
ALTER TABLE core.threads
    DROP CONSTRAINT threads_status_check;

UPDATE core.threads
SET status       = 'awaiting_payment',
    preview_text = 'Ожидает оплаты',
    is_paid      = false,
    paid_at      = NULL,
    free_until   = NULL
WHERE status = 'queued';

ALTER TABLE core.threads
    ALTER COLUMN status SET DEFAULT 'awaiting_payment',
    ADD CONSTRAINT threads_status_check
        CHECK (status IN ('awaiting_payment', 'processing', 'done', 'error', 'canceled'));
