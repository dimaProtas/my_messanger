package contacts

import (
	"context"
	"fmt"
	"log/slog"

	"my_messanger/internal/pkg/apperror"
	"my_messanger/internal/user"
)

// UserRepository — интерфейс к user.Repository, необходимый сервису контактов.
// Определён здесь (DIP), чтобы избежать жёсткой зависимости от пакета user.
type UserRepository interface {
	FindByID(ctx context.Context, id string) (*user.User, error)
}

// Service содержит бизнес-логику модуля контактов.
type Service struct {
	repo     *Repository
	userRepo UserRepository
	logger   *slog.Logger
}

// NewService создаёт новый экземпляр сервиса.
func NewService(repo *Repository, userRepo UserRepository, logger *slog.Logger) *Service {
	return &Service{
		repo:     repo,
		userRepo: userRepo,
		logger:   logger,
	}
}

// Add добавляет контакт.
// Бизнес-правила:
//   - Нельзя добавить самого себя (400).
//   - Пользователь-контакт должен существовать (404).
//   - Нельзя дублировать контакт (409).
func (s *Service) Add(ctx context.Context, userID, contactUserID string) (*ContactResponse, error) {
	// 1. Запрет на добавление самого себя.
	if userID == contactUserID {
		return nil, apperror.BadRequest("cannot add yourself as a contact")
	}

	// 2. Проверка существования контактного пользователя.
	contactUser, err := s.userRepo.FindByID(ctx, contactUserID)
	if err != nil {
		return nil, fmt.Errorf("contacts.Add: find contact user: %w", err)
	}
	if contactUser == nil {
		return nil, apperror.NotFound("contact user not found")
	}

	// 3. Вставка в БД. Add сама проверяет UNIQUE и возвращает Conflict.
	c, err := s.repo.Add(ctx, userID, contactUserID)
	if err != nil {
		return nil, fmt.Errorf("contacts.Add: %w", err)
	}

	return &ContactResponse{
		ID: c.ID,
		ContactUser: user.SearchResponse{
			ID:          contactUser.ID,
			Username:    contactUser.Username,
			PhoneNumber: user.MaskPhone(contactUser.PhoneNumber),
		},
		CreatedAt: c.CreatedAt,
	}, nil
}

// Remove удаляет контакт. Проверяет, что контакт принадлежит пользователю.
func (s *Service) Remove(ctx context.Context, contactID, userID string) error {
	if err := s.repo.Remove(ctx, contactID, userID); err != nil {
		return fmt.Errorf("contacts.Remove: %w", err)
	}
	return nil
}

// List возвращает список контактов пользователя с замаскированными телефонами.
func (s *Service) List(ctx context.Context, userID string) ([]ContactResponse, error) {
	contacts, err := s.repo.List(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("contacts.List: %w", err)
	}

	// Маскируем телефоны в результатах.
	for i := range contacts {
		contacts[i].ContactUser.PhoneNumber = user.MaskPhone(contacts[i].ContactUser.PhoneNumber)
	}

	return contacts, nil
}
