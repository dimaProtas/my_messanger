package kafka

import (
	"context"
	"fmt"
	"log"
	"time"

	"my_messanger/internal/config"

	"github.com/segmentio/kafka-go"
)

type Producer struct {
	writer *kafka.Writer
}

func NewKafkaProducer(cfg *config.Config) *Producer {
	writer := &kafka.Writer{
		Addr:     kafka.TCP(cfg.KafkaBrokers...),
		Balancer: &kafka.LeastBytes{},
		BatchTimeout: 10 * time.Millisecond,
		RequiredAcks:  kafka.RequireOne,
		MaxAttempts:   3,
		ErrorLogger:   kafka.LoggerFunc(log.Printf), // TODO: Использовать slog
		Logger:        kafka.LoggerFunc(log.Printf), // TODO: Использовать slog
	}

	log.Println("Kafka Producer initialized with brokers:", cfg.KafkaBrokers)

	return &Producer{writer: writer}
}

func (p *Producer) ProduceMessage(ctx context.Context, topic string, key, value []byte) error {
	msg := kafka.Message{
		Topic: topic,
		Key:   key,
		Value: value,
	}

	err := p.writer.WriteMessages(ctx, msg)
	if err != nil {
		return fmt.Errorf("failed to write message to Kafka: %w", err)
	}
	return nil
}

func (p *Producer) Close() error {
	return p.writer.Close()
}
