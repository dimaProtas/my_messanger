package chat

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"my_messanger/internal/server"

	"github.com/go-chi/chi/v5"
)

// Handler — HTTP-обработчики для чатов.
type Handler struct {
	service *Service
	log     *slog.Logger
}

// NewHandler создаёт новый обработчик.
func NewHandler(service *Service, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

// RegisterRoutes регистрирует маршруты чатов на роутере chi.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/private", h.CreatePrivateChat)
	r.Post("/group", h.CreateGroupChat)
	r.Get("/", h.ListChats)
	r.Get("/{chatId}", h.GetChat)
	r.Post("/{chatId}/members", h.AddMember)
	r.Delete("/{chatId}/members/{userId}", h.RemoveMember)
}

// CreatePrivateChat — POST /chats/private
func (h *Handler) CreatePrivateChat(w http.ResponseWriter, r *http.Request) {
	userID := server.GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "user not authenticated")
		return
	}

	var req CreatePrivateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body")
		return
	}

	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "user_id is required")
		return
	}

	resp, err := h.service.CreatePrivateChat(r.Context(), userID, req)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

// CreateGroupChat — POST /chats/group
func (h *Handler) CreateGroupChat(w http.ResponseWriter, r *http.Request) {
	userID := server.GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "user not authenticated")
		return
	}

	var req CreateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body")
		return
	}

	resp, err := h.service.CreateGroupChat(r.Context(), userID, req)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

// ListChats — GET /chats
func (h *Handler) ListChats(w http.ResponseWriter, r *http.Request) {
	userID := server.GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "user not authenticated")
		return
	}

	chats, err := h.service.ListChats(r.Context(), userID)
	if err != nil {
		h.log.ErrorContext(r.Context(), "list chats failed", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list chats")
		return
	}

	writeJSON(w, http.StatusOK, chats)
}

// GetChat — GET /chats/{chatId}
func (h *Handler) GetChat(w http.ResponseWriter, r *http.Request) {
	userID := server.GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "user not authenticated")
		return
	}

	chatID := chi.URLParam(r, "chatId")
	if chatID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "chatId is required")
		return
	}

	resp, err := h.service.GetChat(r.Context(), chatID, userID)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// AddMember — POST /chats/{chatId}/members
func (h *Handler) AddMember(w http.ResponseWriter, r *http.Request) {
	userID := server.GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "user not authenticated")
		return
	}

	chatID := chi.URLParam(r, "chatId")
	if chatID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "chatId is required")
		return
	}

	var req AddMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body")
		return
	}

	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "user_id is required")
		return
	}

	resp, err := h.service.AddMember(r.Context(), userID, chatID, req)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// RemoveMember — DELETE /chats/{chatId}/members/{userId}
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	userID := server.GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "user not authenticated")
		return
	}

	chatID := chi.URLParam(r, "chatId")
	targetUserID := chi.URLParam(r, "userId")

	if chatID == "" || targetUserID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "chatId and userId are required")
		return
	}

	if err := h.service.RemoveMember(r.Context(), userID, chatID, targetUserID); err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleServiceError маппит доменные ошибки на HTTP-статусы.
func (h *Handler) handleServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrChatNotFound):
		writeError(w, http.StatusNotFound, "CHAT_NOT_FOUND", err.Error())
	case errors.Is(err, ErrNotMember):
		writeError(w, http.StatusForbidden, "NOT_MEMBER", err.Error())
	case errors.Is(err, ErrNotGroupChat):
		writeError(w, http.StatusBadRequest, "NOT_GROUP_CHAT", err.Error())
	case errors.Is(err, ErrAlreadyMember):
		writeError(w, http.StatusConflict, "ALREADY_MEMBER", err.Error())
	case errors.Is(err, ErrMemberNotFound):
		writeError(w, http.StatusNotFound, "MEMBER_NOT_FOUND", err.Error())
	case errors.Is(err, ErrSelfChat):
		writeError(w, http.StatusBadRequest, "SELF_CHAT", err.Error())
	case errors.Is(err, ErrPermissionDenied):
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED", err.Error())
	case errors.Is(err, ErrValidation):
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		h.log.ErrorContext(r.Context(), "unexpected error", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

// writeJSON записывает JSON-ответ.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		// Логируем, но клиенту уже не ответить — заголовки отправлены
		slog.Error("failed to encode JSON response", "error", err)
	}
}

// writeError записывает стандартную ошибку API.
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error:   code,
		Message: message,
	})
}
