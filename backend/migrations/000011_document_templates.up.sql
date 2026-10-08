-- Шаблоны документов (instructions.md §1 «Настройки» → «Шаблоны»): админ
-- загружает образцы документов (PDF/DOCX) и относит каждый к типу документа
-- из справочника. Используются будущим агентом документов (§9) как образец.
--
-- 1) core.document_types — справочник типов, редактируется в админке (не
--    enum/CHECK: новый тип не должен требовать миграции). Имя уникально без
--    учёта регистра — «Договор» и «договор» один и тот же тип.
CREATE TABLE core.document_types (
    id         uuid PRIMARY KEY,
    name       text NOT NULL CHECK (btrim(name) <> ''),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX document_types_name_key ON core.document_types (lower(name));

-- 2) core.document_templates — сам шаблон. Хранятся два объекта бакета:
--    object_key — исходный файл как загружен; preview_object_key — PDF для
--    просмотра (для PDF-шаблона совпадает с object_key, для DOCX — копия,
--    сконвертированная helper/ — FilesService.ConvertToPdf). FK на тип без
--    CASCADE: тип, у которого есть шаблоны, удалить нельзя
--    (internal/service/template.ErrTypeInUse).
CREATE TABLE core.document_templates (
    id                 uuid PRIMARY KEY,
    document_type_id   uuid NOT NULL REFERENCES core.document_types (id),
    title              text NOT NULL CHECK (btrim(title) <> ''),
    object_key         text NOT NULL,
    original_name      text NOT NULL,
    mime_type          text NOT NULL,
    size_bytes         bigint NOT NULL,
    preview_object_key text NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX document_templates_document_type_id_idx
    ON core.document_templates (document_type_id);

-- 3) Стартовый справочник — типы, с которых пользователь начал постановку
--    задачи, плюс самые частые бытовые документы. id фиксированные — тот же
--    приём, что сид core.agent_prompts (000005): детерминированность между
--    окружениями.
INSERT INTO core.document_types (id, name) VALUES
    ('00000000-0000-0000-0001-000000000001', 'Договор'),
    ('00000000-0000-0000-0001-000000000002', 'Приказ'),
    ('00000000-0000-0000-0001-000000000003', 'Исковое заявление'),
    ('00000000-0000-0000-0001-000000000004', 'Претензия'),
    ('00000000-0000-0000-0001-000000000005', 'Заявление'),
    ('00000000-0000-0000-0001-000000000006', 'Жалоба'),
    ('00000000-0000-0000-0001-000000000007', 'Доверенность')
ON CONFLICT (id) DO NOTHING;
