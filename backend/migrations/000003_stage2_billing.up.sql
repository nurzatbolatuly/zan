-- Stage 2 (BACKEND_PLAN.md): услуги/тарифы/баланс/оплата-заглушка.
--
-- 1) Закрывает открытый вопрос BACKEND_PLAN.md §6 п.7 ("единица измерения
--    цены") — целые тенге, не тиыны, поэтому не numeric(12,2) (который
--    допускает копейки/тиыны, которых в этой системе не бывает), а plain
--    integer. Затрагивает единственные два денежных поля схемы:
--    core.services.price и core.payments.amount. USING round(...)::integer
--    безопасен здесь: обе колонки только что созданы Stage 1 и ещё не
--    хранят прод-данные.
-- 2) core.payments.thread_id — для checkout {..., thread_id?} (v2 §3.5):
--    привязка платежа к конкретному треду, когда баланс кончился в момент
--    открытия треда (v2 §4.2, п.2). Само чтение/использование этого поля
--    для отметки thread.is_paid=true — задача Stage 3 (тред уже должен
--    существовать), здесь только колонка, чтобы Stage 3 не потребовал
--    ещё одной ALTER-миграции ради уже описанного в ТЗ поля контракта.
-- 3) Сид core.services — ровно 2 строки (qa/doc), т.к. v2 §3.7 не даёт
--    /admin/services эндпоинта на создание (только GET список + PUT
--    price/is_active) — то есть каталог услуг не заводится через API,
--    единственный способ завести первые строки — миграция. Цены — 1:1 с
--    демо-данными фронта (frontend/src/features/settings/mocks.ts).

ALTER TABLE core.services
    ALTER COLUMN price TYPE integer USING round(price)::integer,
    ADD CONSTRAINT services_price_non_negative CHECK (price >= 0);

ALTER TABLE core.payments
    ALTER COLUMN amount TYPE integer USING round(amount)::integer,
    ADD CONSTRAINT payments_amount_non_negative CHECK (amount >= 0),
    ADD COLUMN thread_id uuid REFERENCES core.threads (id);

INSERT INTO core.services (id, type_label, name, price, is_active) VALUES
    ('qa', 'Консультация', 'Вопрос-ответ', 2900, true),
    ('doc', 'Документ', 'Подготовка документа', 4900, true);
