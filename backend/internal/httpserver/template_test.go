package httpserver_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type documentTypeBody struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type documentTemplateBody struct {
	ID             string `json:"id"`
	DocumentTypeID string `json:"document_type_id"`
	Title          string `json:"title"`
	OriginalName   string `json:"original_name"`
	MimeType       string `json:"mime_type"`
	PreviewURL     string `json:"preview_url"`
}

func adminJSONRequest(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("X-Admin-Token", testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// adminTemplateRequest — multipart {document_type_id, title, file?}; пустое
// fileName — без поля file (PUT без замены файла).
func adminTemplateRequest(t *testing.T, method, path, typeID, title, fileName string, data []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	require.NoError(t, w.WriteField("document_type_id", typeID))
	require.NoError(t, w.WriteField("title", title))
	if fileName != "" {
		part, err := w.CreateFormFile("file", fileName)
		require.NoError(t, err)
		_, err = part.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())

	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("X-Admin-Token", testAdminToken)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func serve(router *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestAdminDocumentTypes_RequireAdminToken(t *testing.T) {
	router, _ := newTestRouter(t)

	rec := serve(router, httptest.NewRequest(http.MethodGet, "/admin/document-types", nil))

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAdminDocumentTypes_CreateRenameAndRejectDuplicate(t *testing.T) {
	router, _ := newTestRouter(t)

	rec := serve(router, adminJSONRequest(t, http.MethodPost, "/admin/document-types", map[string]string{"name": " Приказ "}))
	require.Equal(t, http.StatusCreated, rec.Code)
	var created documentTypeBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.Equal(t, "Приказ", created.Name)

	rec = serve(router, adminJSONRequest(t, http.MethodPut, "/admin/document-types/"+created.ID, map[string]string{"name": "Приказ по личному составу"}))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "Приказ по личному составу")

	rec = serve(router, adminJSONRequest(t, http.MethodPost, "/admin/document-types", map[string]string{"name": "ДОГОВОР"}))
	require.Equal(t, http.StatusConflict, rec.Code)
	require.Contains(t, rec.Body.String(), "document_type_name_taken")

	rec = serve(router, adminJSONRequest(t, http.MethodPost, "/admin/document-types", map[string]string{"name": "   "}))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminDocumentTemplates_UploadListUpdateDelete(t *testing.T) {
	router, _ := newTestRouter(t)

	rec := serve(router, adminTemplateRequest(t, http.MethodPost, "/admin/document-templates",
		seededDocumentTypeID, "Договор аренды", "lease.docx", []byte("PK\x03\x04 docx")))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created documentTemplateBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.Equal(t, "Договор аренды", created.Title)
	require.Equal(t, "lease.docx", created.OriginalName)
	require.Equal(t, "https://storage.public/converted/preview.pdf", created.PreviewURL)

	rec = serve(router, adminJSONRequest(t, http.MethodGet, "/admin/document-templates", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var listed []documentTemplateBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listed))
	require.Len(t, listed, 1)

	// Тип с шаблонами удалить нельзя.
	rec = serve(router, adminJSONRequest(t, http.MethodDelete, "/admin/document-types/"+seededDocumentTypeID, nil))
	require.Equal(t, http.StatusConflict, rec.Code)
	require.Contains(t, rec.Body.String(), "document_type_in_use")

	// PUT без файла — меняется только название, файл прежний.
	rec = serve(router, adminTemplateRequest(t, http.MethodPut, "/admin/document-templates/"+created.ID,
		seededDocumentTypeID, "Договор аренды квартиры", "", nil))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var updated documentTemplateBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	require.Equal(t, "Договор аренды квартиры", updated.Title)
	require.Equal(t, "lease.docx", updated.OriginalName)

	// PUT с файлом — файл заменён.
	rec = serve(router, adminTemplateRequest(t, http.MethodPut, "/admin/document-templates/"+created.ID,
		seededDocumentTypeID, "Договор аренды квартиры", "lease.pdf", []byte("%PDF-1.7")))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	require.Equal(t, "application/pdf", updated.MimeType)

	rec = serve(router, adminJSONRequest(t, http.MethodDelete, "/admin/document-templates/"+created.ID, nil))
	require.Equal(t, http.StatusNoContent, rec.Code)

	rec = serve(router, adminJSONRequest(t, http.MethodDelete, "/admin/document-types/"+seededDocumentTypeID, nil))
	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestAdminDocumentTemplates_UploadErrors(t *testing.T) {
	tests := []struct {
		name     string
		typeID   string
		fileName string
		data     []byte
		wantCode int
		wantBody string
	}{
		{name: "unsupported format", typeID: seededDocumentTypeID, fileName: "notes.txt", data: []byte("text"), wantCode: http.StatusUnsupportedMediaType, wantBody: "unsupported_file_type"},
		{name: "unknown type", typeID: "00000000-0000-0000-0000-00000000dead", fileName: "a.pdf", data: []byte("%PDF-1.7"), wantCode: http.StatusNotFound, wantBody: "document_type_not_found"},
		{name: "missing file", typeID: seededDocumentTypeID, wantCode: http.StatusBadRequest, wantBody: "invalid_request"},
		{name: "too large", typeID: seededDocumentTypeID, fileName: "a.pdf", data: make([]byte, testFileMaxSizeBytes+1), wantCode: http.StatusRequestEntityTooLarge, wantBody: "file_too_large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, _ := newTestRouter(t)

			rec := serve(router, adminTemplateRequest(t, http.MethodPost, "/admin/document-templates", tt.typeID, "T", tt.fileName, tt.data))

			require.Equal(t, tt.wantCode, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), tt.wantBody)
		})
	}
}

func TestAdminDocumentTemplates_UpdateUnknownReturns404(t *testing.T) {
	router, _ := newTestRouter(t)

	rec := serve(router, adminTemplateRequest(t, http.MethodPut, "/admin/document-templates/00000000-0000-0000-0000-00000000dead",
		seededDocumentTypeID, "T", "", nil))

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), "document_template_not_found")
}
