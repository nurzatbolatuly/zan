-- Stage 6 (BACKEND_PLAN.md): агент "Документы" — POST /threads/{id}/generate-
-- document (service_id="doc", zan-backend-tz-v2.md §4.4) + GET /threads/{id}
-- /document?format=pdf|docx.
--
-- core.file_attachments.thread_id — прямая ссылка на тред для
-- generated_output-файлов (нет прод-данных, безопасно). Без неё GET
-- /threads/{id}/document?format= пришлось бы находить сгенерированные
-- файлы через JOIN core.messages (message_id -> thread_id), жёстко
-- привязывая генерацию документа к конкретному сообщению чата и усложняя
-- единственный реально нужный запрос ("последний сгенерированный файл этого
-- треда в этом формате"). Nullable — для purpose=analysis_input остаётся
-- NULL (владение уже определяется через message_id/session_id, см. 000004).
ALTER TABLE core.file_attachments
    ADD COLUMN thread_id uuid REFERENCES core.threads (id);

CREATE INDEX file_attachments_thread_id_idx
    ON core.file_attachments (thread_id, mime_type, created_at DESC)
    WHERE thread_id IS NOT NULL;
