package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"my_messanger/internal/server"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/register", h.Register)
	r.Post("/login", h.Login)
	r.Post("/refresh", h.Refresh)
	r.Post("/logout", h.Logout)
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	if errs := validateRegisterRequest(&req); len(errs) > 0 {
		writeValidationErrors(w, errs)
		return
	}

	resp, err := h.service.Register(r.Context(), req)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	if errs := validateLoginRequest(&req); len(errs) > 0 {
		writeValidationErrors(w, errs)
		return
	}

	resp, err := h.service.Login(r.Context(), req)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	if strings.TrimSpace(req.RefreshToken) == "" {
		writeError(w, http.StatusBadRequest, "MISSING_TOKEN", "Refresh token is required")
		return
	}

	resp, err := h.service.RefreshTokens(r.Context(), req.RefreshToken)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	userID := server.GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated")
		return
	}

	var req LogoutRequest
	if err := decodeJSON(r, &req); err != nil {
		req = LogoutRequest{}
	}

	accessToken := extractBearerToken(r)
	if accessToken == "" {
		writeError(w, http.StatusBadRequest, "MISSING_TOKEN", "Access token is required")
		return
	}

	if err := h.service.Logout(r.Context(), userID, accessToken, req.RefreshToken); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

type validationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func validateRegisterRequest(req *RegisterRequest) []validationError {
	var errs []validationError

	phone := normalizePhone(req.PhoneNumber)
	if len(phone) < 7 || len(phone) > 15 {
		errs = append(errs, validationError{Field: "phone_number", Message: "Phone number must contain 7 to 15 digits"})
	}

	if len(req.Password) < 8 {
		errs = append(errs, validationError{Field: "password", Message: "Password must be at least 8 characters"})
	}

	username := strings.TrimSpace(req.Username)
	if len(username) < 3 || len(username) > 50 {
		errs = append(errs, validationError{Field: "username", Message: "Username must be between 3 and 50 characters"})
	}

	return errs
}

func validateLoginRequest(req *LoginRequest) []validationError {
	var errs []validationError

	phone := normalizePhone(req.PhoneNumber)
	if len(phone) < 7 || len(phone) > 15 {
		errs = append(errs, validationError{Field: "phone_number", Message: "Phone number must contain 7 to 15 digits"})
	}

	if len(req.Password) < 8 {
		errs = append(errs, validationError{Field: "password", Message: "Password must be at least 8 characters"})
	}

	return errs
}

func decodeJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error": code, "message": message})
}

func writeValidationErrors(w http.ResponseWriter, errs []validationError) {
	writeJSON(w, http.StatusUnprocessableEntity, map[string]interface{}{
		"error":   "VALIDATION_ERROR",
		"message": "Validation failed",
		"details": errs,
	})
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return ""
	}
	return auth[len(prefix):]
}

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Resource not found")
	case errors.Is(err, ErrAlreadyExists):
		writeError(w, http.StatusConflict, "ALREADY_EXISTS", err.Error())
	case errors.Is(err, ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid phone number or password")
	case errors.Is(err, ErrTokenRevoked):
		writeError(w, http.StatusUnauthorized, "TOKEN_REVOKED", "Token has been revoked for security reasons")
	case errors.Is(err, ErrTokenExpired):
		writeError(w, http.StatusUnauthorized, "TOKEN_EXPIRED", "Token has expired")
	default:
		slog.Error("internal auth error", "error", fmt.Sprintf("%v", err))
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
	}
}
