package message

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"my_messanger/internal/server"

	"github.com/go-chi/chi/v5"
)

// Handler обрабатывает HTTP-запросы, связанные с сообщениями.
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler создаёт новый экземпляр обработчика.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// RegisterRoutes регистрирует маршруты модуля сообщений на chi.Router.
// Предполагается, что router уже защищён AuthMiddleware.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/chats/{chatId}/messages", h.SendMessage)
	r.Get("/chats/{chatId}/messages", h.GetHistory)
	r.Put("/messages/{messageId}/status", h.UpdateStatus)
}

// SendMessage обрабатывает POST /chats/{chatId}/messages.
func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) {
	chatID := chi.URLParam(r, "chatId")
	if chatID == "" {
		h.writeError(w, http.StatusBadRequest, "INVALID_CHAT_ID", "chat id is required")
		return
	}

	userID := getUserID(r)
	if userID == "" {
		h.writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "user not authenticated")
		return
	}

	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}

	msg, err := h.service.Send(r.Context(), chatID, userID, req.Content)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, msg)
}

// GetHistory обрабатывает GET /chats/{chatId}/messages?limit=50&before=2024-01-01T00:00:00Z.
func (h *Handler) GetHistory(w http.ResponseWriter, r *http.Request) {
	chatID := chi.URLParam(r, "chatId")
	if chatID == "" {
		h.writeError(w, http.StatusBadRequest, "INVALID_CHAT_ID", "chat id is required")
		return
	}

	userID := getUserID(r)
	if userID == "" {
		h.writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "user not authenticated")
		return
	}

	limit := h.parseIntQuery(r, "limit", 50)
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	before := time.Now().UTC()
	if beforeStr := r.URL.Query().Get("before"); beforeStr != "" {
		parsed, err := time.Parse(time.RFC3339, beforeStr)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "INVALID_BEFORE", "before must be a valid RFC3339 timestamp")
			return
		}
		before = parsed
	}

	messages, err := h.service.GetHistory(r.Context(), chatID, userID, limit, before)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	if messages == nil {
		messages = []Message{}
	}

	h.writeJSON(w, http.StatusOK, messages)
}

// UpdateStatus обрабатывает PUT /messages/{messageId}/status.
func (h *Handler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	messageID := chi.URLParam(r, "messageId")
	if messageID == "" {
		h.writeError(w, http.StatusBadRequest, "INVALID_MESSAGE_ID", "message id is required")
		return
	}

	userID := getUserID(r)
	if userID == "" {
		h.writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "user not authenticated")
		return
	}

	var req UpdateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}

	msg, err := h.service.UpdateStatus(r.Context(), messageID, userID, req.Status)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, msg)
}

// handleServiceError маппит ошибки бизнес-логики на HTTP-статусы.
func (h *Handler) handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case isOneOf(err, ErrNotMember):
		h.writeError(w, http.StatusForbidden, "NOT_MEMBER", err.Error())
	case isOneOf(err, ErrMessageNotFound):
		h.writeError(w, http.StatusNotFound, "MESSAGE_NOT_FOUND", err.Error())
	case isOneOf(err, ErrInvalidStatus, ErrStatusTransition):
		h.writeError(w, http.StatusBadRequest, "INVALID_STATUS", err.Error())
	case isOneOf(err, ErrContentRequired):
		h.writeError(w, http.StatusBadRequest, "CONTENT_REQUIRED", err.Error())
	default:
		h.logger.Error("unexpected error in message handler", "error", err)
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

// --- Вспомогательные функции ---

// getUserID извлекает ID пользователя из контекста запроса.
// Использует экспортируемый ключ server.UserIDKey, установленный AuthMiddleware.
func getUserID(r *http.Request) string {
	userID, _ := r.Context().Value(server.UserIDKey).(string)
	return userID
}

// writeJSON сериализует payload в JSON и пишет в ответ.
func (h *Handler) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		h.logger.Error("failed to write json response", "error", err)
	}
}

// writeError пишет стандартизированный JSON-ответ с ошибкой.
func (h *Handler) writeError(w http.ResponseWriter, status int, code, message string) {
	h.writeJSON(w, status, map[string]string{
		"error":   code,
		"message": message,
	})
}

// parseIntQuery парсит целочисленный query-параметр с fallback-значением.
func (h *Handler) parseIntQuery(r *http.Request, key string, defaultVal int) int {
	val := r.URL.Query().Get(key)
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return n
}

// isOneOf проверяет, соответствует ли ошибка одной из целевых sentinel-ошибок.
// Использует errors.Is для поддержки wrapped errors.
func isOneOf(err error, targets ...error) bool {
	for _, t := range targets {
		if errors.Is(err, t) {
			return true
		}
	}
	return false
}
