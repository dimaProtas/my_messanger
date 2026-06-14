package message

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"my_messanger/internal/config"

	"github.com/segmentio/kafka-go"
)

// Consumer читает сообщения из Kafka и передаёт их в Service для обработки.
type Consumer struct {
	reader  *kafka.Reader
	service *Service
	logger  *slog.Logger
}

// NewConsumer создаёт новый экземпляр Kafka Consumer.
// Использует группу "messenger-group" и читает топик "messages".
// StartOffset = LastOffset — при первом запуске группы читаем только новые сообщения.
func NewConsumer(cfg *config.Config, service *Service, logger *slog.Logger) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     cfg.KafkaBrokers,
		GroupID:     "messenger-group",
		Topic:       "messages",
		StartOffset: kafka.LastOffset,
		MinBytes:    10e3, // 10 KB
		MaxBytes:    10e6, // 10 MB
		// Ручное управление смещениями через FetchMessage/CommitMessages
	})

	return &Consumer{
		reader:  reader,
		service: service,
		logger:  logger,
	}
}

// Start запускает бесконечный цикл чтения сообщений из Kafka.
// Блокирует выполнение до отмены контекста.
func (c *Consumer) Start(ctx context.Context) error {
	c.logger.Info("kafka consumer started",
		"topic", "messages",
		"group", "messenger-group",
	)

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				c.logger.Info("kafka consumer shutting down")
				return nil
			}
			c.logger.Error("failed to fetch message from kafka", "error", err)
			continue
		}

		var kafkaMsg KafkaMessage
		if err := json.Unmarshal(msg.Value, &kafkaMsg); err != nil {
			c.logger.Error("failed to unmarshal kafka message",
				"error", err,
				"offset", msg.Offset,
				"partition", msg.Partition,
			)
			// Сообщение повреждено — коммитим, чтобы не застревать.
			if commitErr := c.reader.CommitMessages(ctx, msg); commitErr != nil {
				c.logger.Error("failed to commit poisoned message", "error", commitErr)
			}
			continue
		}

		if err := c.service.ProcessIncoming(ctx, kafkaMsg); err != nil {
			c.logger.Error("failed to process incoming message",
				"error", err,
				"message_id", kafkaMsg.ID,
				"chat_id", kafkaMsg.ChatID,
			)
			// НЕ коммитим — сообщение будет перечитано при рестарте consumer'а.
			// TODO: добавить dead-letter очередь после N повторных попыток.
			continue
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			c.logger.Error("failed to commit message offset",
				"error", err,
				"message_id", kafkaMsg.ID,
				"offset", msg.Offset,
			)
		}
	}
}

// Stop корректно закрывает Kafka Reader.
func (c *Consumer) Stop() error {
	c.logger.Info("stopping kafka consumer")
	return c.reader.Close()
}
