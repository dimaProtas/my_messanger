package user

import (
	"context"
	"fmt"
	"log/slog"

	"my_messanger/internal/pkg/apperror"
)

const defaultSearchLimit = 20

// Service содержит бизнес-логику модуля пользователей.
type Service struct {
	repo   *Repository
	logger *slog.Logger
}

// NewService создаёт новый экземпляр сервиса.
func NewService(repo *Repository, logger *slog.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// GetProfile возвращает профиль пользователя по userID.
func (s *Service) GetProfile(ctx context.Context, userID string) (*User, error) {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user.GetProfile: %w", err)
	}
	if user == nil {
		return nil, apperror.NotFound("user not found")
	}
	return user, nil
}

// UpdateProfile обновляет профиль пользователя.
// Проверяет, что username не пустой.
func (s *Service) UpdateProfile(ctx context.Context, userID string, req UpdateProfileRequest) (*User, error) {
	if req.Username == "" {
		return nil, apperror.BadRequest("username must not be empty")
	}

	if err := s.repo.Update(ctx, userID, req.Username); err != nil {
		s.logger.ErrorContext(ctx, "failed to update user",
			"user_id", userID,
			"error", err,
		)
		return nil, fmt.Errorf("user.UpdateProfile: %w", err)
	}

	// Возвращаем актуальное состояние после обновления.
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user.UpdateProfile: refetch: %w", err)
	}
	if user == nil {
		return nil, apperror.NotFound("user not found after update")
	}

	return user, nil
}

// SearchUsers ищет пользователей по номеру телефона (LIKE),
// исключая excludeID. Возвращает до 20 результатов с замаскированными номерами.
func (s *Service) SearchUsers(ctx context.Context, phone, excludeID string) ([]SearchResponse, error) {
	if phone == "" {
		return []SearchResponse{}, nil
	}

	pattern := "%" + phone + "%"

	users, err := s.repo.SearchByPhone(ctx, pattern, excludeID, defaultSearchLimit)
	if err != nil {
		return nil, fmt.Errorf("user.SearchUsers: %w", err)
	}

	result := make([]SearchResponse, 0, len(users))
	for _, u := range users {
		result = append(result, SearchResponse{
			ID:          u.ID,
			Username:    u.Username,
			PhoneNumber: MaskPhone(u.PhoneNumber),
		})
	}

	return result, nil
}
