package chat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// Service — слой бизнес-логики чатов.
type Service struct {
	repo *Repository
	log  *slog.Logger
}

// NewService создаёт новый сервис.
func NewService(repo *Repository, log *slog.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// CreatePrivateChat создаёт приватный чат между текущим пользователем и указанным.
// Если чат уже существует — возвращает существующий.
func (s *Service) CreatePrivateChat(ctx context.Context, userID string, req CreatePrivateRequest) (*ChatResponse, error) {
	if userID == req.UserID {
		return nil, fmt.Errorf("user %s with %s: %w", userID, req.UserID, ErrSelfChat)
	}

	// Проверяем существующий чат
	existing, err := s.repo.FindPrivateChat(ctx, userID, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("find private chat: %w", err)
	}
	if existing != nil {
		members, err := s.repo.GetMembers(ctx, existing.ID)
		if err != nil {
			return nil, fmt.Errorf("get members: %w", err)
		}
		return &ChatResponse{
			Chat:    *existing,
			Members: members,
		}, nil
	}

	// Создаём новый
	chat, err := s.repo.CreatePrivateChat(ctx, userID, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("create private chat: %w", err)
	}

	members, err := s.repo.GetMembers(ctx, chat.ID)
	if err != nil {
		return nil, fmt.Errorf("get members: %w", err)
	}

	s.log.InfoContext(ctx, "private chat created",
		"chat_id", chat.ID,
		"user1", userID,
		"user2", req.UserID,
	)

	return &ChatResponse{
		Chat:    *chat,
		Members: members,
	}, nil
}

// CreateGroupChat создаёт групповой чат. Текущий пользователь становится создателем и участником.
func (s *Service) CreateGroupChat(ctx context.Context, userID string, req CreateGroupRequest) (*ChatResponse, error) {
	// Валидация
	if req.Name == "" {
		return nil, fmt.Errorf("name is required: %w", ErrValidation)
	}

	// Собираем всех участников: создатель + memberIDs
	allMembers := append([]string{userID}, req.MemberIDs...)
	uniqueMembers := uniqueStrings(allMembers)

	if len(uniqueMembers) < 2 {
		return nil, fmt.Errorf("group must have at least 2 members: %w", ErrValidation)
	}

	chat, err := s.repo.CreateGroupChat(ctx, req.Name, userID, req.MemberIDs)
	if err != nil {
		return nil, fmt.Errorf("create group chat: %w", err)
	}

	members, err := s.repo.GetMembers(ctx, chat.ID)
	if err != nil {
		return nil, fmt.Errorf("get members: %w", err)
	}

	s.log.InfoContext(ctx, "group chat created",
		"chat_id", chat.ID,
		"name", req.Name,
		"created_by", userID,
		"member_count", len(members),
	)

	return &ChatResponse{
		Chat:    *chat,
		Members: members,
	}, nil
}

// AddMember добавляет участника в групповой чат.
// Только участник чата может добавлять других.
func (s *Service) AddMember(ctx context.Context, userID, chatID string, req AddMemberRequest) (*ChatResponse, error) {
	// Проверка, что чат существует и является групповым
	isGroup, err := s.repo.IsGroupChat(ctx, chatID)
	if err != nil {
		return nil, err
	}
	if !isGroup {
		return nil, fmt.Errorf("chat %s: %w", chatID, ErrNotGroupChat)
	}

	// Проверка, что вызывающий пользователь — участник
	isMember, err := s.repo.IsMember(ctx, chatID, userID)
	if err != nil {
		return nil, fmt.Errorf("check membership: %w", err)
	}
	if !isMember {
		return nil, fmt.Errorf("user %s in chat %s: %w", userID, chatID, ErrNotMember)
	}

	// Добавляем
	if err := s.repo.AddMember(ctx, chatID, req.UserID); err != nil {
		if errors.Is(err, ErrAlreadyMember) {
			// Уже участник — не ошибка, возвращаем текущее состояние
			return s.buildChatResponse(ctx, chatID)
		}
		return nil, err
	}

	s.log.InfoContext(ctx, "member added",
		"chat_id", chatID,
		"added_by", userID,
		"added_user", req.UserID,
	)

	return s.buildChatResponse(ctx, chatID)
}

// RemoveMember удаляет участника из группового чата.
// Только создатель чата может удалять участников.
func (s *Service) RemoveMember(ctx context.Context, userID, chatID, targetUserID string) error {
	// Получаем чат для проверки created_by
	chat, err := s.repo.GetByID(ctx, chatID)
	if err != nil {
		return err
	}

	if chat.Type != "group" {
		return fmt.Errorf("chat %s: %w", chatID, ErrNotGroupChat)
	}
	if chat.CreatedBy != userID {
		return fmt.Errorf("only creator can remove members: %w", ErrPermissionDenied)
	}

	if err := s.repo.RemoveMember(ctx, chatID, targetUserID); err != nil {
		return err
	}

	s.log.InfoContext(ctx, "member removed",
		"chat_id", chatID,
		"removed_by", userID,
		"removed_user", targetUserID,
	)

	return nil
}

// ListChats возвращает все чаты пользователя.
func (s *Service) ListChats(ctx context.Context, userID string) ([]ChatResponse, error) {
	chats, err := s.repo.ListUserChats(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list chats: %w", err)
	}
	return chats, nil
}

// GetChat возвращает полную информацию о чате с проверкой членства.
func (s *Service) GetChat(ctx context.Context, chatID, userID string) (*ChatResponse, error) {
	isMember, err := s.repo.IsMember(ctx, chatID, userID)
	if err != nil {
		return nil, fmt.Errorf("check membership: %w", err)
	}
	if !isMember {
		return nil, fmt.Errorf("user %s in chat %s: %w", userID, chatID, ErrNotMember)
	}

	return s.buildChatResponse(ctx, chatID)
}

// buildChatResponse собирает ChatResponse из чата и его участников.
func (s *Service) buildChatResponse(ctx context.Context, chatID string) (*ChatResponse, error) {
	chat, err := s.repo.GetByID(ctx, chatID)
	if err != nil {
		return nil, err
	}

	members, err := s.repo.GetMembers(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("get members: %w", err)
	}

	return &ChatResponse{
		Chat:    *chat,
		Members: members,
	}, nil
}

// uniqueStrings возвращает слайс без дубликатов, сохраняя порядок.
func uniqueStrings(slice []string) []string {
	seen := make(map[string]struct{}, len(slice))
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			result = append(result, s)
		}
	}
	return result
}
