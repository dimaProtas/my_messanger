package apperror

import "net/http"

// AppError — кастомный тип ошибки с HTTP-статусом.
// Используется для передачи контролируемых ошибок от слоя сервиса к хендлеру.
// В отличие от sentinel-ошибок, позволяет параметризовать сообщение.
type AppError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *AppError) Error() string {
	return e.Message
}

// New создаёт новую AppError.
func New(code, message string, httpStatus int) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
	}
}

// ---------- Предопределённые коды и конструкторы ----------

const (
	CodeBadRequest   = "BAD_REQUEST"
	CodeUnauthorized = "UNAUTHORIZED"
	CodeForbidden    = "FORBIDDEN"
	CodeNotFound     = "NOT_FOUND"
	CodeConflict     = "CONFLICT"
	CodeInternal     = "INTERNAL_ERROR"
)

func BadRequest(message string) *AppError {
	if message == "" {
		message = "invalid request"
	}
	return New(CodeBadRequest, message, http.StatusBadRequest)
}

func Unauthorized(message string) *AppError {
	if message == "" {
		message = "authentication required"
	}
	return New(CodeUnauthorized, message, http.StatusUnauthorized)
}

func Forbidden(message string) *AppError {
	if message == "" {
		message = "access denied"
	}
	return New(CodeForbidden, message, http.StatusForbidden)
}

func NotFound(message string) *AppError {
	if message == "" {
		message = "resource not found"
	}
	return New(CodeNotFound, message, http.StatusNotFound)
}

func Conflict(message string) *AppError {
	if message == "" {
		message = "resource already exists"
	}
	return New(CodeConflict, message, http.StatusConflict)
}

func Internal(message string) *AppError {
	if message == "" {
		message = "internal server error"
	}
	return New(CodeInternal, message, http.StatusInternalServerError)
}
