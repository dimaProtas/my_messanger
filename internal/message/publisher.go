package message

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"my_messanger/internal/server"

	"github.com/go-redis/redis/v8"
)

// Publisher отвечает за доставку сообщений подписчикам
// через Redis Pub/Sub и WebSocket-подключения.
type Publisher struct {
	redisClient *redis.Client
	connManager *server.ConnectionManager
	logger      *slog.Logger
}

// NewPublisher создаёт новый экземпляр Publisher.
func NewPublisher(redisClient *redis.Client, connManager *server.ConnectionManager, logger *slog.Logger) *Publisher {
	return &Publisher{
		redisClient: redisClient,
		connManager: connManager,
		logger:      logger,
	}
}

// PublishToMembers отправляет JSON-представление сообщения каждому участнику чата
// через Redis Pub/Sub в канал "messages:{userID}". Использует Pipeline для
// минимизации round-trip'ов к Redis.
// Пользователи, находящиеся онлайн (подписанные через WebSocket), получат сообщение
// моментально — логика подписки реализована в WsServer.writePump.
func (p *Publisher) PublishToMembers(ctx context.Context, message Message, memberIDs []string) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("publisher: marshal message: %w", err)
	}

	if len(memberIDs) == 0 {
		return nil
	}

	pipe := p.redisClient.Pipeline()
	for _, userID := range memberIDs {
		channel := fmt.Sprintf("messages:%s", userID)
		pipe.Publish(ctx, channel, data)
	}
	cmds, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("publisher: redis pipeline exec: %w", err)
	}

	// Логируем количество доставленных подписчиков для каждого канала.
	// Это опционально, но полезно для мониторинга.
	for _, cmd := range cmds {
		if pubCmd, ok := cmd.(*redis.IntCmd); ok {
			subscribers, _ := pubCmd.Result()
			p.logger.Debug("message published to channel",
				"subscribers", subscribers,
			)
		}
	}

	return nil
}

// PublishStatusUpdate публикует обновление статуса сообщения подписчикам чата.
func (p *Publisher) PublishStatusUpdate(ctx context.Context, message Message, memberIDs []string) error {
	// Используем тот же канал "messages:{userID}" — клиент сам разберёт,
	// новое это сообщение или обновление статуса существующего.
	return p.PublishToMembers(ctx, message, memberIDs)
}
