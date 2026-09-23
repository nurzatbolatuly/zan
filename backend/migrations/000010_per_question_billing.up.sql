-- Один вопрос = одна консультация (instructions.md §2 «Ключевая
-- бизнес-логика»): бесплатных уточнений внутри треда больше нет, каждый
-- вопрос списывает единицу услуги. Вместе с этим теряют смысл:
--   * free_until/closed_at — окно бесплатных уточнений и закрытие треда по
--     его истечении (тред больше не закрывается — продолжается платно);
--   * is_paid/paid_at — оплачивается не тред, а каждый вопрос; оплачен ли
--     текущий вопрос, видно по status (awaiting_payment). История оплат
--     остаётся в core.payments (paid_at, thread_id).
ALTER TABLE core.threads
    DROP COLUMN is_paid,
    DROP COLUMN paid_at,
    DROP COLUMN free_until,
    DROP COLUMN closed_at;
