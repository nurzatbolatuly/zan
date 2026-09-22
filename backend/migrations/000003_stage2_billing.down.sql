DELETE FROM core.services WHERE id IN ('qa', 'doc');

ALTER TABLE core.payments
    DROP COLUMN thread_id,
    DROP CONSTRAINT payments_amount_non_negative,
    ALTER COLUMN amount TYPE numeric(12, 2);

ALTER TABLE core.services
    DROP CONSTRAINT services_price_non_negative,
    ALTER COLUMN price TYPE numeric(12, 2);
