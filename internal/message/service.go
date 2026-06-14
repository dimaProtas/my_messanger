package message

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Стандартные ошибки бизнес-логики.
var (
	ErrNotMember        = errors.New("user is not a member of this chat")
	ErrMessageNotFound  = errors.New("message not found")
	ErrInvalidStatus    = errors.New("invalid status")
	ErrStatusTransition = errors.New("invalid status transition")
	ErrContentRequired  = errors.New("content is required")
)

// ChatRepository определяет контракт, который должен реализовать chat-модуль.
// Определён в message-пакете согласно Dependency Inversion Principle.
type ChatRepository interface {
	// IsMember проверяет, является ли пользователь участником чата.
	IsMember(ctx context.Context, chatID, userID string) (bool, error)

	// GetMemberIDs возвращает список всех участников чата.
	GetMemberIDs(ctx context.Context, chatID string) ([]string, error)
}

// Service содержит бизнес-логику модуля сообщений.
type Service struct {
	repo        *Repository
	msgProducer *MessageProducer
	publisher   *Publisher
	chatRepo    ChatRepository
	logger      *slog.Logger
}

// NewService создаёт новый экземпляр сервиса сообщений.
func NewService(
	repo *Repository,
	msgProducer *MessageProducer,
	publisher *Publisher,
	chatRepo ChatRepository,
	logger *slog.Logger,
) *Service {
	return &Service{
		repo:        repo,
		msgProducer: msgProducer,
		publisher:   publisher,
		chatRepo:    chatRepo,
		logger:      logger,
	}
}

// Send отправляет сообщение в чат.
//  1. Проверяет членство отправителя.
//  2. Создаёт объект Message.
//  3. Отправляет в Kafka (асинхронная доставка).
//  4. Возвращает сообщение со статусом "sent".
//
// Компромисс: клиент получает ответ до сохранения в БД.
// Сохранение происходит асинхронно через Consumer → ProcessIncoming.
func (s *Service) Send(ctx context.Context, chatID, senderID, content string) (*Message, error) {
	if content == "" {
		return nil, ErrContentRequired
	}

	isMember, err := s.chatRepo.IsMember(ctx, chatID, senderID)
	if err != nil {
		return nil, fmt.Errorf("service: check membership: %w", err)
	}
	if !isMember {
		return nil, ErrNotMember
	}

	now := time.Now().UTC()
	msg := Message{
		ID:        NewUUID(),
		ChatID:    chatID,
		SenderID:  senderID,
		Content:   content,
		SentAt:    now,
		Status:    StatusSent,
		UpdatedAt: now,
	}

	kafkaMsg := KafkaMessageFromDomain(msg)

	if err := s.msgProducer.ProduceMessage(ctx, kafkaMsg); err != nil {
		return nil, fmt.Errorf("service: produce message: %w", err)
	}

	s.logger.Info("message sent",
		"message_id", msg.ID,
		"chat_id", chatID,
		"sender_id", senderID,
	)

	return &msg, nil
}

// ProcessIncoming обрабатывает сообщение, полученное из Kafka:
//  1. Сохраняет в БД (идемпотентно).
//  2. Публикует всем участникам чата через Redis Pub/Sub.
func (s *Service) ProcessIncoming(ctx context.Context, kafkaMsg KafkaMessage) error {
	msg, err := DomainFromKafkaMessage(kafkaMsg)
	if err != nil {
		return fmt.Errorf("service: convert kafka message: %w", err)
	}

	if err := s.repo.Save(ctx, msg); err != nil {
		return fmt.Errorf("service: save message: %w", err)
	}

	memberIDs, err := s.chatRepo.GetMemberIDs(ctx, msg.ChatID)
	if err != nil {
		return fmt.Errorf("service: get member ids: %w", err)
	}

	if err := s.publisher.PublishToMembers(ctx, msg, memberIDs); err != nil {
		return fmt.Errorf("service: publish to members: %w", err)
	}

	s.logger.Info("message processed and delivered",
		"message_id", msg.ID,
		"chat_id", msg.ChatID,
		"members_count", len(memberIDs),
	)

	return nil
}

// GetHistory возвращает историю сообщений чата.
// Проверяет, что запрашивающий пользователь является участником чата.
// Если before — нулевое время, используется текущее время.
func (s *Service) GetHistory(ctx context.Context, chatID, userID string, limit int, before time.Time) ([]Message, error) {
	isMember, err := s.chatRepo.IsMember(ctx, chatID, userID)
	if err != nil {
		return nil, fmt.Errorf("service: check membership: %w", err)
	}
	if !isMember {
		return nil, ErrNotMember
	}

	if before.IsZero() {
		before = time.Now().UTC()
	}

	messages, err := s.repo.GetHistory(ctx, chatID, before, limit)
	if err != nil {
		return nil, fmt.Errorf("service: get history: %w", err)
	}

	return messages, nil
}

// UpdateStatus обновляет статус сообщения (sent → delivered → read).
//  1. Находит сообщение.
//  2. Проверяет членство пользователя в чате.
//  3. Валидирует допустимость перехода статуса.
//  4. Обновляет статус в БД.
//  5. Публикует обновление всем участникам чата.
func (s *Service) UpdateStatus(ctx context.Context, messageID, userID, status string) (*Message, error) {
	if !isValidStatus(status) {
		return nil, ErrInvalidStatus
	}

	msg, err := s.repo.GetByID(ctx, messageID)
	if err != nil {
		return nil, fmt.Errorf("service: get message: %w", err)
	}
	if msg == nil {
		return nil, ErrMessageNotFound
	}

	if !IsValidStatusTransition(msg.Status, status) {
		return nil, fmt.Errorf("%w: cannot transition from %q to %q", ErrStatusTransition, msg.Status, status)
	}

	// Проверяем, что пользователь — участник чата.
	isMember, err := s.chatRepo.IsMember(ctx, msg.ChatID, userID)
	if err != nil {
		return nil, fmt.Errorf("service: check membership: %w", err)
	}
	if !isMember {
		return nil, ErrNotMember
	}

	updated, err := s.repo.UpdateStatus(ctx, messageID, status)
	if err != nil {
		return nil, fmt.Errorf("service: update status: %w", err)
	}
	if !updated {
		return nil, ErrMessageNotFound
	}

	// Обновляем локальную копию.
	msg.Status = status
	msg.UpdatedAt = time.Now().UTC()

	// Публикуем обновление участникам чата.
	memberIDs, err := s.chatRepo.GetMemberIDs(ctx, msg.ChatID)
	if err != nil {
		s.logger.Warn("failed to get member ids for status update broadcast",
			"message_id", messageID,
			"error", err,
		)
	} else {
		if err := s.publisher.PublishStatusUpdate(ctx, *msg, memberIDs); err != nil {
			s.logger.Warn("failed to broadcast status update",
				"message_id", messageID,
				"error", err,
			)
		}
	}

	s.logger.Info("message status updated",
		"message_id", messageID,
		"new_status", status,
	)

	return msg, nil
}

// isValidStatus проверяет, что статус является одним из допустимых значений.
func isValidStatus(status string) bool {
	_, ok := statusOrder[status]
	return ok
}
