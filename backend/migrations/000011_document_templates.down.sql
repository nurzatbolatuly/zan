-- Объекты шаблонов в бакете миграция не удаляет (у golang-migrate нет
-- доступа к S3) — после отката они остаются осиротевшими под templates/ и
-- converted/.
DROP TABLE core.document_templates;
DROP TABLE core.document_types;
