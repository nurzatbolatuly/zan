package httpserver

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"zan-backend/internal/apierror"
	"zan-backend/internal/domain"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/service/template"
)

// documentTypeResponse — openapi.yaml#DocumentType.
type documentTypeResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	UpdatedAt string `json:"updated_at"`
}

func newDocumentTypeResponse(t domain.DocumentType) documentTypeResponse {
	return documentTypeResponse{ID: t.ID, Name: t.Name, UpdatedAt: formatTime(t.UpdatedAt)}
}

// documentTemplateResponse — openapi.yaml#DocumentTemplate.
type documentTemplateResponse struct {
	ID             string `json:"id"`
	DocumentTypeID string `json:"document_type_id"`
	Title          string `json:"title"`
	OriginalName   string `json:"original_name"`
	MimeType       string `json:"mime_type"`
	SizeBytes      int64  `json:"size_bytes"`
	PreviewURL     string `json:"preview_url"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

func newDocumentTemplateResponse(v template.TemplateView) documentTemplateResponse {
	return documentTemplateResponse{
		ID:             v.Template.ID,
		DocumentTypeID: v.Template.DocumentTypeID,
		Title:          v.Template.Title,
		OriginalName:   v.Template.OriginalName,
		MimeType:       v.Template.MimeType,
		SizeBytes:      v.Template.SizeBytes,
		PreviewURL:     v.PreviewURL,
		CreatedAt:      formatTime(v.Template.CreatedAt),
		UpdatedAt:      formatTime(v.Template.UpdatedAt),
	}
}

// documentTypeWriteRequest — тело POST/PUT /admin/document-types(/{id}).
type documentTypeWriteRequest struct {
	Name string `json:"name" binding:"required"`
}

// adminListDocumentTypesHandler — GET /admin/document-types.
func adminListDocumentTypesHandler(svc *template.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		types, err := svc.ListTypes(c.Request.Context())
		if err != nil {
			writeTemplateError(c, err, 0)
			return
		}
		out := make([]documentTypeResponse, 0, len(types))
		for _, t := range types {
			out = append(out, newDocumentTypeResponse(t))
		}
		c.JSON(http.StatusOK, out)
	}
}

// adminCreateDocumentTypeHandler — POST /admin/document-types.
func adminCreateDocumentTypeHandler(svc *template.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req documentTypeWriteRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, invalidRequestError("name is required"))
			return
		}
		created, err := svc.CreateType(c.Request.Context(), req.Name)
		if err != nil {
			writeTemplateError(c, err, 0)
			return
		}
		c.JSON(http.StatusCreated, newDocumentTypeResponse(created))
	}
}

// adminRenameDocumentTypeHandler — PUT /admin/document-types/{id}.
func adminRenameDocumentTypeHandler(svc *template.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req documentTypeWriteRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, invalidRequestError("name is required"))
			return
		}
		updated, err := svc.RenameType(c.Request.Context(), c.Param("id"), req.Name)
		if err != nil {
			writeTemplateError(c, err, 0)
			return
		}
		c.JSON(http.StatusOK, newDocumentTypeResponse(updated))
	}
}

// adminDeleteDocumentTypeHandler — DELETE /admin/document-types/{id}.
func adminDeleteDocumentTypeHandler(svc *template.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.DeleteType(c.Request.Context(), c.Param("id")); err != nil {
			writeTemplateError(c, err, 0)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// adminListDocumentTemplatesHandler — GET /admin/document-templates.
func adminListDocumentTemplatesHandler(svc *template.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		views, err := svc.List(c.Request.Context())
		if err != nil {
			writeTemplateError(c, err, 0)
			return
		}
		out := make([]documentTemplateResponse, 0, len(views))
		for _, v := range views {
			out = append(out, newDocumentTemplateResponse(v))
		}
		c.JSON(http.StatusOK, out)
	}
}

// adminCreateDocumentTemplateHandler — POST /admin/document-templates:
// multipart {file, document_type_id, title}.
func adminCreateDocumentTemplateHandler(svc *template.Service, maxSizeBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		upload, apiErr := readUploadedFile(c, "file", maxSizeBytes)
		if apiErr != nil {
			writeError(c, apiErr)
			return
		}
		if upload == nil {
			writeError(c, invalidRequestError("Missing multipart field \"file\""))
			return
		}

		created, err := svc.Create(c.Request.Context(), template.CreateRequest{
			DocumentTypeID: c.PostForm("document_type_id"),
			Title:          c.PostForm("title"),
			File:           template.File{OriginalName: upload.Name, Data: upload.Data},
		})
		if err != nil {
			writeTemplateError(c, err, maxSizeBytes)
			return
		}
		c.JSON(http.StatusCreated, newDocumentTemplateResponse(created))
	}
}

// adminUpdateDocumentTemplateHandler — PUT /admin/document-templates/{id}:
// multipart {document_type_id, title, file?} — без file меняются только тип
// и название.
func adminUpdateDocumentTemplateHandler(svc *template.Service, maxSizeBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		upload, apiErr := readUploadedFile(c, "file", maxSizeBytes)
		if apiErr != nil {
			writeError(c, apiErr)
			return
		}

		req := template.UpdateRequest{
			DocumentTypeID: c.PostForm("document_type_id"),
			Title:          c.PostForm("title"),
		}
		if upload != nil {
			req.File = &template.File{OriginalName: upload.Name, Data: upload.Data}
		}

		updated, err := svc.Update(c.Request.Context(), c.Param("id"), req)
		if err != nil {
			writeTemplateError(c, err, maxSizeBytes)
			return
		}
		c.JSON(http.StatusOK, newDocumentTemplateResponse(updated))
	}
}

// adminDeleteDocumentTemplateHandler — DELETE /admin/document-templates/{id}.
func adminDeleteDocumentTemplateHandler(svc *template.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Delete(c.Request.Context(), c.Param("id")); err != nil {
			writeTemplateError(c, err, 0)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// writeTemplateError — единая таблица template.Err* -> ApiError для всех
// ручек типов и шаблонов документов.
func writeTemplateError(c *gin.Context, err error, maxSizeBytes int64) {
	switch {
	case errors.Is(err, template.ErrInvalidInput):
		writeError(c, invalidRequestError(err.Error()))
	case errors.Is(err, template.ErrTypeNotFound):
		writeError(c, notFoundError("document_type_not_found", "Тип документа не найден"))
	case errors.Is(err, template.ErrNotFound):
		writeError(c, notFoundError("document_template_not_found", "Шаблон не найден"))
	case errors.Is(err, template.ErrTypeNameTaken):
		writeError(c, &apierror.Error{
			Code:       "document_type_name_taken",
			Message:    "Тип документа с таким названием уже есть",
			HTTPStatus: http.StatusConflict,
		})
	case errors.Is(err, template.ErrTypeInUse):
		writeError(c, &apierror.Error{
			Code:       "document_type_in_use",
			Message:    "У этого типа есть шаблоны — сначала удалите их или перенесите в другой тип",
			HTTPStatus: http.StatusConflict,
		})
	case errors.Is(err, template.ErrTooLarge):
		writeError(c, fileTooLargeError(maxSizeBytes))
	case errors.Is(err, template.ErrUnsupportedFormat):
		writeError(c, &apierror.Error{
			Code:       "unsupported_file_type",
			Message:    "Шаблон должен быть в формате PDF или DOCX",
			HTTPStatus: http.StatusUnsupportedMediaType,
		})
	case errors.Is(err, template.ErrConversionFailed):
		writeError(c, &apierror.Error{
			Code:       "template_conversion_failed",
			Message:    "Не удалось подготовить PDF-копию документа",
			HTTPStatus: http.StatusBadGateway,
		})
	default:
		logger.FromContext(c.Request.Context()).Error("admin_document_template_operation_failed",
			slog.Group("context", slog.String("error", err.Error())))
		writeError(c, apierror.Internal())
	}
}
