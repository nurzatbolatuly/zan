// Package apierror задаёт единый тип ошибки публичного HTTP-слоя
// (BACKEND_CODING_STANDARDS.md §8). internal/httpserver маппит известные
// бизнес-ошибки service-пакетов на Error; всё нераспознанное — общий
// internal_error/500 с нейтральным текстом, оригинал уходит только в лог.
package apierror

import "net/http"

// Error — тело ответа при ошибке ({code, message} — BACKEND_PLAN.md §6) и
// HTTP-статус, который под неё выставляется.
type Error struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
}

func (e *Error) Error() string {
	return e.Message
}

// Internal — общий internal_error/500 с нейтральным текстом для всего, что
// не распознано явно (§8): оригинальная ошибка уходит только в лог вызывающим
// кодом, сюда не передаётся.
func Internal() *Error {
	return &Error{
		Code:       "internal_error",
		Message:    "Что-то пошло не так, попробуйте ещё раз",
		HTTPStatus: http.StatusInternalServerError,
	}
}
