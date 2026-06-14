package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"my_messanger/internal/config"
	jwtpkg "my_messanger/internal/pkg/jwt"

	"github.com/go-redis/redis/v8"
)

type contextKey string

const (
	UserIDKey    contextKey = "userID"
	ClaimsKey    contextKey = "claims"
	RequestIDKey contextKey = "requestID"
)

func AuthMiddleware(cfg *config.Config, redisClient *redis.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error":"UNAUTHORIZED","message":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				http.Error(w, `{"error":"UNAUTHORIZED","message":"invalid authorization format"}`, http.StatusUnauthorized)
				return
			}

			claims, err := jwtpkg.ValidateToken(parts[1], cfg)
			if err != nil {
				http.Error(w, `{"error":"UNAUTHORIZED","message":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			blacklisted, err := redisClient.Exists(r.Context(), "blacklist:"+claims.ID).Result()
			if err == nil && blacklisted > 0 {
				http.Error(w, `{"error":"TOKEN_REVOKED","message":"token has been revoked"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, ClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateShortID()
		}
		ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserIDFromContext извлекает ID пользователя, сохранённый AuthMiddleware.
// Возвращает пустую строку, если контекст не содержит userID.
func GetUserIDFromContext(ctx context.Context) string {
	userID, _ := ctx.Value(UserIDKey).(string)
	return userID
}

func generateShortID() string {
	b := make([]byte, 8)
	for i := range b {
		b[i] = byte(time.Now().UnixNano()>>(i*4)) & 0xFF
	}
	return strings.TrimRight(string(b), "\x00")
}
