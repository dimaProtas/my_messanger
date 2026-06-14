package message

import (
	"context"
	"encoding/json"
	"fmt"

	"my_messanger/internal/pkg/kafka"
)

// MessageProducer оборачивает низкоуровневый kafka.Producer для отправки сообщений мессенджера.
type MessageProducer struct {
	kafkaProducer *kafka.Producer
}

// NewMessageProducer создаёт новый экземпляр MessageProducer.
func NewMessageProducer(kafkaProducer *kafka.Producer) *MessageProducer {
	return &MessageProducer{kafkaProducer: kafkaProducer}
}

// ProduceMessage сериализует KafkaMessage в JSON и отправляет в топик "messages".
// Ключом сообщения выступает chatID для гарантии порядка внутри одного чата.
func (mp *MessageProducer) ProduceMessage(ctx context.Context, msg KafkaMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("message producer: marshal: %w", err)
	}

	if err := mp.kafkaProducer.ProduceMessage(ctx, "messages", []byte(msg.ChatID), data); err != nil {
		return fmt.Errorf("message producer: produce: %w", err)
	}

	return nil
}
