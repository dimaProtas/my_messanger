package message

import (
	"crypto/rand"
	"fmt"
	"time"
)

// Статусы сообщений.
const (
	StatusSent      = "sent"
	StatusDelivered = "delivered"
	StatusRead      = "read"
)

// statusOrder задаёт строгий порядок переходов: sent(1) → delivered(2) → read(3).
var statusOrder = map[string]int{
	StatusSent:      1,
	StatusDelivered: 2,
	StatusRead:      3,
}

// IsValidStatusTransition проверяет, что переход from → to допустим.
func IsValidStatusTransition(from, to string) bool {
	fromOrder, okFrom := statusOrder[from]
	toOrder, okTo := statusOrder[to]
	if !okFrom || !okTo {
		return false
	}
	return toOrder > fromOrder
}

// Message — доменная модель сообщения.
type Message struct {
	ID        string    `json:"id"`
	ChatID    string    `json:"chat_id"`
	SenderID  string    `json:"sender_id"`
	Content   string    `json:"content"`
	SentAt    time.Time `json:"sent_at"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SendMessageRequest — тело запроса на отправку сообщения.
type SendMessageRequest struct {
	Content string `json:"content"`
}

// UpdateStatusRequest — тело запроса на обновление статуса.
type UpdateStatusRequest struct {
	Status string `json:"status"`
}

// KafkaMessage — структура сообщения, передаваемого через Kafka.
type KafkaMessage struct {
	ID       string `json:"id"`
	ChatID   string `json:"chat_id"`
	SenderID string `json:"sender_id"`
	Content  string `json:"content"`
	SentAt   string `json:"sent_at"`
	Status   string `json:"status"`
}

// NewUUID генерирует UUID v4 без внешних зависимостей.
func NewUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read не должна падать в Linux, но на всякий случай — fallback на time.
		panic(fmt.Sprintf("crypto/rand.Read failed: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// KafkaMessageFromDomain конвертирует доменную модель в Kafka-модель.
func KafkaMessageFromDomain(m Message) KafkaMessage {
	return KafkaMessage{
		ID:       m.ID,
		ChatID:   m.ChatID,
		SenderID: m.SenderID,
		Content:  m.Content,
		SentAt:   m.SentAt.Format(time.RFC3339Nano),
		Status:   m.Status,
	}
}

// DomainFromKafkaMessage конвертирует Kafka-модель в доменную.
func DomainFromKafkaMessage(km KafkaMessage) (Message, error) {
	sentAt, err := time.Parse(time.RFC3339Nano, km.SentAt)
	if err != nil {
		return Message{}, fmt.Errorf("invalid sent_at format: %w", err)
	}
	return Message{
		ID:        km.ID,
		ChatID:    km.ChatID,
		SenderID:  km.SenderID,
		Content:   km.Content,
		SentAt:    sentAt,
		Status:    km.Status,
		UpdatedAt: time.Now().UTC(),
	}, nil
}
