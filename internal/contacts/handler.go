package contacts

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"my_messanger/internal/pkg/apperror"
	"my_messanger/internal/pkg/response"
	"my_messanger/internal/server"

	"github.com/go-chi/chi/v5"
)

// Handler — HTTP-транспорт для модуля контактов.
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler создаёт новый экземпляр Handler.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// RegisterRoutes монтирует эндпоинты модуля contacts на переданный chi.Router.
// Предполагается, что middleware аутентификации уже применён к роутеру или его родителю.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/contacts", h.Add)
	r.Delete("/contacts/{contactId}", h.Remove)
	r.Get("/contacts", h.List)
}

// Add обрабатывает POST /contacts.
func (h *Handler) Add(w http.ResponseWriter, r *http.Request) {
	userID := server.GetUserIDFromContext(r.Context())
	if userID == "" {
		response.Error(w, http.StatusUnauthorized, apperror.CodeUnauthorized, "user not authenticated")
		return
	}

	var req AddContactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, apperror.CodeBadRequest, "invalid request body")
		return
	}

	if req.ContactUserID == "" {
		response.Error(w, http.StatusBadRequest, apperror.CodeBadRequest, "contact_user_id is required")
		return
	}

	contact, err := h.service.Add(r.Context(), userID, req.ContactUserID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	response.JSON(w, http.StatusCreated, contact)
}

// Remove обрабатывает DELETE /contacts/{contactId}.
func (h *Handler) Remove(w http.ResponseWriter, r *http.Request) {
	userID := server.GetUserIDFromContext(r.Context())
	if userID == "" {
		response.Error(w, http.StatusUnauthorized, apperror.CodeUnauthorized, "user not authenticated")
		return
	}

	contactID := chi.URLParam(r, "contactId")
	if contactID == "" {
		response.Error(w, http.StatusBadRequest, apperror.CodeBadRequest, "contactId is required")
		return
	}

	if err := h.service.Remove(r.Context(), contactID, userID); err != nil {
		h.handleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// List обрабатывает GET /contacts.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID := server.GetUserIDFromContext(r.Context())
	if userID == "" {
		response.Error(w, http.StatusUnauthorized, apperror.CodeUnauthorized, "user not authenticated")
		return
	}

	contacts, err := h.service.List(r.Context(), userID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	response.JSON(w, http.StatusOK, contacts)
}

// handleError определяет тип ошибки и пишет соответствующий HTTP-ответ.
func (h *Handler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	var appErr *apperror.AppError
	if errors.As(err, &appErr) {
		response.Error(w, appErr.HTTPStatus, appErr.Code, appErr.Message)
		return
	}

	h.logger.ErrorContext(r.Context(), "unhandled internal error",
		"method", r.Method,
		"path", r.URL.Path,
		"error", err,
	)
	response.Error(w, http.StatusInternalServerError, apperror.CodeInternal, "internal server error")
}
