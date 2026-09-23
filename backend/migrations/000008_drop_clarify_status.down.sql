-- Переведённые в done треды обратно в clarify не восстанавливаются — какие из
-- них были clarify, после up не известно.
ALTER TABLE core.threads
    DROP CONSTRAINT threads_status_check,
    ADD CONSTRAINT threads_status_check
        CHECK (status IN ('queued', 'processing', 'clarify', 'done', 'error', 'canceled'));
