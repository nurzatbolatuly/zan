package domain

import "time"

// DocumentType — тип документа из справочника, редактируемого в админке
// (договор, приказ, исковое заявление…). Не enum: новый тип заводится через
// /admin/document-types, без миграции.
type DocumentType struct {
	ID        string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// DocumentTemplate — образец документа определённого типа, загруженный
// админом (/admin/document-templates). Нескольких шаблонов одного типа
// допустимо.
//
// ObjectKey — исходный файл (PDF/DOCX) как загружен. PreviewObjectKey — PDF
// для просмотра в админке: у PDF-шаблона совпадает с ObjectKey, у DOCX —
// отдельный объект, сконвертированный helper/ (FilesService.ConvertToPdf).
// Ссылки на скачивание не хранятся — presigned-ссылка перевыпускается на
// каждый ответ (тот же принцип, что FileAttachment.ObjectKey).
type DocumentTemplate struct {
	ID               string
	DocumentTypeID   string
	Title            string
	ObjectKey        string
	OriginalName     string
	MimeType         string
	SizeBytes        int64
	PreviewObjectKey string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ObjectKeys — все объекты бакета, которыми владеет шаблон (без дублей, если
// превью — сам исходный PDF): удаляются вместе с шаблоном или при замене файла.
func (t DocumentTemplate) ObjectKeys() []string {
	if t.PreviewObjectKey == t.ObjectKey {
		return []string{t.ObjectKey}
	}
	return []string{t.ObjectKey, t.PreviewObjectKey}
}
