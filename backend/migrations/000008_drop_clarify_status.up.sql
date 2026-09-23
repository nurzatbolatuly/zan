-- Статус clarify удалён (instructions.md §2 "Вызов LLM-агента"): Q&A — прямой
-- streaming-вызов LLM без структуры ответа, уточняющий вопрос модели — обычный
-- ответ done. Существующие clarify-треды переводятся в done (тот же смысл
-- для пользователя: ответ получен, тред можно продолжить в free_until),
-- иначе новый CHECK не применится.
UPDATE core.threads
SET status = 'done'
WHERE status = 'clarify';

ALTER TABLE core.threads
    DROP CONSTRAINT threads_status_check,
    ADD CONSTRAINT threads_status_check
        CHECK (status IN ('queued', 'processing', 'done', 'error', 'canceled'));
