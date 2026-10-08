package httpserver

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"zan-backend/internal/apierror"
)

// uploadedFile — содержимое multipart-поля с файлом, прочитанное целиком.
type uploadedFile struct {
	Name        string
	ContentType string
	Data        []byte
}

// readUploadedFile — общий разбор multipart-поля field для всех загрузок
// (вложения, голос, шаблоны документов). Ограничивает тело запроса
// maxSizeBytes ещё до разбора (http.MaxBytesReader), чтобы заведомо большой
// upload не читался в память целиком; сервисный слой всё равно перепроверяет
// размер сам. Вызывается до любого другого чтения формы — c.PostForm после
// него читает уже разобранную форму. Отсутствующее поле — (nil, nil):
// обязательность файла решает вызывающий хендлер.
func readUploadedFile(c *gin.Context, field string, maxSizeBytes int64) (*uploadedFile, *apierror.Error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSizeBytes+1)

	fh, err := c.FormFile(field)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		switch {
		case errors.As(err, &maxBytesErr):
			return nil, fileTooLargeError(maxSizeBytes)
		case errors.Is(err, http.ErrMissingFile):
			return nil, nil
		default:
			return nil, invalidRequestError("Invalid multipart body")
		}
	}

	opened, err := fh.Open()
	if err != nil {
		return nil, apierror.Internal()
	}
	data, err := io.ReadAll(opened)
	_ = opened.Close() // только чтение — ошибка закрытия не влияет на уже прочитанные данные
	if err != nil {
		return nil, apierror.Internal()
	}
	return &uploadedFile{Name: fh.Filename, ContentType: fh.Header.Get("Content-Type"), Data: data}, nil
}
