package auth

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"my_messanger/internal/config"
	jwtpkg "my_messanger/internal/pkg/jwt"

	"github.com/go-redis/redis/v8"
	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

type Service struct {
	repo        *Repository
	cfg         *config.Config
	redisClient *redis.Client
}

func NewService(repo *Repository, cfg *config.Config, redisClient *redis.Client) *Service {
	return &Service{repo: repo, cfg: cfg, redisClient: redisClient}
}

func (s *Service) Register(ctx context.Context, req RegisterRequest) (*AuthResponse, error) {
	phone := normalizePhone(req.PhoneNumber)

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.repo.CreateUser(ctx, phone, string(hashedPassword), req.Username)
	if err != nil {
		return nil, err
	}

	resp, err := s.generateAuthResponse(ctx, user)
	if err != nil {
		return nil, err
	}

	slog.Info("user registered", "user_id", user.ID)
	return resp, nil
}

func (s *Service) Login(ctx context.Context, req LoginRequest) (*AuthResponse, error) {
	phone := normalizePhone(req.PhoneNumber)

	user, hashedPassword, err := s.repo.FindUserByPhoneWithPassword(ctx, phone)
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword(hashedPassword, []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	resp, err := s.generateAuthResponse(ctx, *user)
	if err != nil {
		return nil, err
	}

	slog.Info("user logged in", "user_id", user.ID)
	return resp, nil
}

func (s *Service) RefreshTokens(ctx context.Context, refreshTokenStr string) (*AuthResponse, error) {
	claims, err := jwtpkg.ValidateToken(refreshTokenStr, s.cfg)
	if err != nil {
		return nil, fmt.Errorf("validate refresh token: %w", err)
	}

	jti := claims.ID
	if jti == "" {
		return nil, fmt.Errorf("refresh token missing jti claim")
	}

	rt, err := s.repo.FindRefreshToken(ctx, jti)
	if err != nil {
		return nil, fmt.Errorf("find refresh token: %w", err)
	}

	if rt.Revoked {
		s.repo.RevokeAllUserTokens(ctx, rt.UserID)
		slog.Warn("refresh token reuse detected — all tokens revoked", "user_id", rt.UserID, "jti", jti)
		return nil, ErrTokenRevoked
	}

	if time.Now().After(rt.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	if err := s.repo.RevokeRefreshToken(ctx, jti); err != nil {
		return nil, fmt.Errorf("revoke old refresh token: %w", err)
	}

	user, err := s.repo.FindUserByID(ctx, rt.UserID)
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}

	resp, err := s.generateAuthResponse(ctx, *user)
	if err != nil {
		return nil, err
	}

	slog.Info("tokens refreshed", "user_id", user.ID)
	return resp, nil
}

func (s *Service) Logout(ctx context.Context, userID, accessTokenStr, refreshTokenStr string) error {
	accessClaims, err := jwtpkg.ValidateToken(accessTokenStr, s.cfg)
	if err == nil && accessClaims != nil {
		remainingTTL := time.Until(accessClaims.ExpiresAt.Time)
		if remainingTTL > 0 {
			s.redisClient.Set(ctx, "access_token_blacklist:"+accessClaims.ID, userID, remainingTTL)
		}
	}

	if refreshTokenStr != "" {
		refreshClaims, err := jwtpkg.ValidateToken(refreshTokenStr, s.cfg)
		if err == nil && refreshClaims.ID != "" {
			s.repo.RevokeRefreshToken(ctx, refreshClaims.ID)
		}
	}

	slog.Info("user logged out", "user_id", userID)
	return nil
}

func (s *Service) generateAuthResponse(ctx context.Context, user User) (*AuthResponse, error) {
	accessToken, _, err := jwtpkg.GenerateAccessToken(user.ID, s.cfg)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, refreshJTI, err := jwtpkg.GenerateRefreshToken(user.ID, s.cfg)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	expiresAt := time.Now().Add(s.cfg.RefreshTokenTTL)
	if err := s.repo.SaveRefreshToken(ctx, user.ID, refreshJTI, expiresAt); err != nil {
		return nil, fmt.Errorf("save refresh token: %w", err)
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         user,
	}, nil
}
