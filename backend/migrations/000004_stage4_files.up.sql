-- Stage 4 (BACKEND_PLAN.md): файлы/голос/рендер документа.
--
-- core.file_attachments (Stage 1, 000002) не выдерживает реальный
-- юзкейс загрузки, обнаруженный только теперь, когда появляется первый
-- настоящий потребитель (нет прод-данных, эти правки безопасны):
--
-- 1) message_id NOT NULL был неверен с самого начала: v2 §3.2 "{...,
--    file_ids?}" требует, чтобы файл существовал (POST /files/upload)
--    ДО того, как он привязан к сообщению (POST /threads|/messages) —
--    то есть message_id обязан допускать NULL в промежутке между
--    загрузкой и привязкой.
-- 2) session_id — v2 §2.8 не заводил это поле, но без него нечем
--    проверить владение файлом до привязки к сообщению (GET /files/{id}
--    чужой сессии, попытка приложить чужой file_id к своему сообщению) —
--    тот же принцип "не подтверждать существование чужого id", что уже
--    применяется к Payment/Thread (BACKEND_CODING_STANDARDS.md).
-- 3) file_url -> object_key: колонка ни разу не читалась/не писалась с
--    момента создания (Stage 1) — переименована, пока это не сломало ни
--    одного вызывающего кода. Хранит ключ объекта в бакете, не готовую
--    ссылку: presigned-ссылка перевыпускается на каждый ответ заново
--    (internal/platform/storage.Client.PresignGet*), у неё есть срок
--    жизни, и хранить протухающее значение в БД было бы неверно.
-- 4) extracted_text — результат FilesService.Extract, которому v2-схема
--    не оставляла места (BACKEND_PLAN.md §3, domain.FileAttachment).
ALTER TABLE core.file_attachments
    ALTER COLUMN message_id DROP NOT NULL,
    ADD COLUMN session_id uuid NOT NULL REFERENCES core.sessions (id),
    ADD COLUMN extracted_text text;

ALTER TABLE core.file_attachments
    RENAME COLUMN file_url TO object_key;
