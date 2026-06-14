package user

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

// Handler — HTTP-транспорт для модуля пользователей.
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

// RegisterRoutes монтирует эндпоинты модуля users на переданный chi.Router.
// Предполагается, что middleware аутентификации уже применён к роутеру или его родителю.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/users/me", h.GetProfile)
	r.Put("/users/me", h.UpdateProfile)
	r.Get("/users/search", h.SearchUsers)
}

// GetProfile обрабатывает GET /users/me.
func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userID := server.GetUserIDFromContext(r.Context())
	if userID == "" {
		response.Error(w, http.StatusUnauthorized, apperror.CodeUnauthorized, "user not authenticated")
		return
	}

	user, err := h.service.GetProfile(r.Context(), userID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	response.JSON(w, http.StatusOK, user)
}

// UpdateProfile обрабатывает PUT /users/me.
func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID := server.GetUserIDFromContext(r.Context())
	if userID == "" {
		response.Error(w, http.StatusUnauthorized, apperror.CodeUnauthorized, "user not authenticated")
		return
	}

	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, apperror.CodeBadRequest, "invalid request body")
		return
	}

	user, err := h.service.UpdateProfile(r.Context(), userID, req)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	response.JSON(w, http.StatusOK, user)
}

// SearchUsers обрабатывает GET /users/search?phone=... .
func (h *Handler) SearchUsers(w http.ResponseWriter, r *http.Request) {
	userID := server.GetUserIDFromContext(r.Context())
	if userID == "" {
		response.Error(w, http.StatusUnauthorized, apperror.CodeUnauthorized, "user not authenticated")
		return
	}

	phone := r.URL.Query().Get("phone")

	results, err := h.service.SearchUsers(r.Context(), phone, userID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	response.JSON(w, http.StatusOK, results)
}

// handleError определяет тип ошибки и пишет соответствующий HTTP-ответ.
// AppError-ошибки логируются на уровне Info (ожидаемые ошибки валидации),
// всё остальное — на Error (неожиданные сбои).
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
