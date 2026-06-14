package response

import (
	"encoding/json"
	"net/http"
)

// ErrorBody — стандартное тело ошибки.
type ErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// JSON пишет data в ответ с указанным HTTP-статусом.
// Content-Type выставляется автоматически.
func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(data); err != nil {
		// Пишем в лог, но клиенту уже поздно менять статус — тело повреждено.
		http.Error(w, `{"error":"INTERNAL_ERROR","message":"failed to encode response"}`, http.StatusInternalServerError)
	}
}

// Error пишет стандартный JSON-объект ошибки.
func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, ErrorBody{
		Error:   code,
		Message: message,
	})
}
