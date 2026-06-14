package message

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Repository реализует слой доступа к данным для сообщений.
type Repository struct {
	db *sql.DB
}

// NewRepository создаёт новый экземпляр репозитория.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// Save сохраняет сообщение. Идемпотентен благодаря ON CONFLICT DO NOTHING.
func (r *Repository) Save(ctx context.Context, msg Message) error {
	query := `
		INSERT INTO messages (id, chat_id, sender_id, content, sent_at, status, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO NOTHING`

	_, err := r.db.ExecContext(ctx, query,
		msg.ID,
		msg.ChatID,
		msg.SenderID,
		msg.Content,
		msg.SentAt,
		msg.Status,
		msg.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("message repository: save: %w", err)
	}
	return nil
}

// GetHistory возвращает историю сообщений чата с курсорной пагинацией (sent_at < before).
func (r *Repository) GetHistory(ctx context.Context, chatID string, before time.Time, limit int) ([]Message, error) {
	query := `
		SELECT id, chat_id, sender_id, content, sent_at, status, updated_at
		FROM messages
		WHERE chat_id = $1 AND sent_at < $2
		ORDER BY sent_at DESC
		LIMIT $3`

	rows, err := r.db.QueryContext(ctx, query, chatID, before, limit)
	if err != nil {
		return nil, fmt.Errorf("message repository: get history: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ChatID, &m.SenderID, &m.Content, &m.SentAt, &m.Status, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("message repository: scan row: %w", err)
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("message repository: rows iteration: %w", err)
	}

	return messages, nil
}

// GetByID возвращает сообщение по идентификатору.
func (r *Repository) GetByID(ctx context.Context, messageID string) (*Message, error) {
	query := `
		SELECT id, chat_id, sender_id, content, sent_at, status, updated_at
		FROM messages
		WHERE id = $1`

	var m Message
	err := r.db.QueryRowContext(ctx, query, messageID).Scan(
		&m.ID, &m.ChatID, &m.SenderID, &m.Content, &m.SentAt, &m.Status, &m.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("message repository: get by id: %w", err)
	}
	return &m, nil
}

// UpdateStatus обновляет статус сообщения. Возвращает true, если была изменена хотя бы одна строка.
func (r *Repository) UpdateStatus(ctx context.Context, messageID, status string) (bool, error) {
	query := `
		UPDATE messages
		SET status = $1, updated_at = $2
		WHERE id = $3`

	result, err := r.db.ExecContext(ctx, query, status, time.Now().UTC(), messageID)
	if err != nil {
		return false, fmt.Errorf("message repository: update status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("message repository: rows affected: %w", err)
	}

	return rowsAffected > 0, nil
}
