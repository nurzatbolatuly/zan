ALTER TABLE core.file_attachments
    RENAME COLUMN object_key TO file_url;

ALTER TABLE core.file_attachments
    DROP COLUMN extracted_text,
    DROP COLUMN session_id,
    ALTER COLUMN message_id SET NOT NULL;
