DROP INDEX IF EXISTS core.file_attachments_thread_id_idx;

ALTER TABLE core.file_attachments
    DROP COLUMN thread_id;
