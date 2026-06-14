package chat

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// Repository — слой доступа к данным чатов (PostgreSQL).
type Repository struct {
	db *sql.DB
}

// NewRepository создаёт новый репозиторий.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// CreatePrivateChat создаёт приватный чат между двумя пользователями.
// Выполняется в одной транзакции: INSERT в chats + два INSERT в chat_members.
func (r *Repository) CreatePrivateChat(ctx context.Context, user1, user2 string) (*Chat, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	chat := &Chat{}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO chats (type)
		VALUES ('private')
		RETURNING id, type, COALESCE(name, '') as name, COALESCE(created_by::text, '') as created_by, created_at, updated_at
	`).Scan(&chat.ID, &chat.Type, &chat.Name, &chat.CreatedBy, &chat.CreatedAt, &chat.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert chat: %w", err)
	}

	now := time.Now()
	for _, userID := range []string{user1, user2} {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO chat_members (chat_id, user_id, joined_at)
			VALUES ($1, $2, $3)
		`, chat.ID, userID, now)
		if err != nil {
			return nil, fmt.Errorf("insert chat_member %s: %w", userID, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return chat, nil
}

// FindPrivateChat ищет существующий приватный чат между двумя пользователями.
func (r *Repository) FindPrivateChat(ctx context.Context, user1, user2 string) (*Chat, error) {
	chat := &Chat{}
	var name, createdBy sql.NullString

	err := r.db.QueryRowContext(ctx, `
		SELECT c.id, c.type, c.name, c.created_by, c.created_at, c.updated_at
		FROM chats c
		INNER JOIN chat_members cm1 ON cm1.chat_id = c.id AND cm1.user_id = $1
		INNER JOIN chat_members cm2 ON cm2.chat_id = c.id AND cm2.user_id = $2
		WHERE c.type = 'private'
		LIMIT 1
	`, user1, user2).Scan(
		&chat.ID, &chat.Type, &name, &createdBy,
		&chat.CreatedAt, &chat.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // чат не найден — не ошибка
		}
		return nil, fmt.Errorf("find private chat: %w", err)
	}

	chat.Name = name.String
	if createdBy.Valid {
		chat.CreatedBy = createdBy.String
	}
	return chat, nil
}

// CreateGroupChat создаёт групповой чат и добавляет создателя + участников.
func (r *Repository) CreateGroupChat(ctx context.Context, name, createdBy string, memberIDs []string) (*Chat, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	chat := &Chat{}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO chats (type, name, created_by)
		VALUES ('group', $1, $2::uuid)
		RETURNING id, type, name, created_by::text, created_at, updated_at
	`, name, createdBy).Scan(
		&chat.ID, &chat.Type, &chat.Name, &chat.CreatedBy,
		&chat.CreatedAt, &chat.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert group chat: %w", err)
	}

	// Добавляем создателя
	now := time.Now()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO chat_members (chat_id, user_id, joined_at)
		VALUES ($1, $2, $3)
	`, chat.ID, createdBy, now)
	if err != nil {
		return nil, fmt.Errorf("insert creator as member: %w", err)
	}

	// Добавляем остальных участников (исключая создателя, если он дублируется)
	for _, memberID := range memberIDs {
		if memberID == createdBy {
			continue
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO chat_members (chat_id, user_id, joined_at)
			VALUES ($1, $2, $3)
		`, chat.ID, memberID, now)
		if err != nil {
			return nil, fmt.Errorf("insert member %s: %w", memberID, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return chat, nil
}

// AddMember добавляет участника в чат (предполагается, что проверка на group уже выполнена).
func (r *Repository) AddMember(ctx context.Context, chatID, userID string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO chat_members (chat_id, user_id)
		VALUES ($1, $2)
	`, chatID, userID)
	if err != nil {
		// Проверяем нарушение уникальности
		if isUniqueViolation(err) {
			return fmt.Errorf("user %s already in chat %s: %w", userID, chatID, ErrAlreadyMember)
		}
		return fmt.Errorf("add member: %w", err)
	}
	return nil
}

// RemoveMember удаляет участника из чата.
func (r *Repository) RemoveMember(ctx context.Context, chatID, userID string) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM chat_members
		WHERE chat_id = $1 AND user_id = $2
	`, chatID, userID)
	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("member %s not found in chat %s: %w", userID, chatID, ErrMemberNotFound)
	}
	return nil
}

// IsMember проверяет, является ли пользователь участником чата.
func (r *Repository) IsMember(ctx context.Context, chatID, userID string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM chat_members
			WHERE chat_id = $1 AND user_id = $2
		)
	`, chatID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("is member: %w", err)
	}
	return exists, nil
}

// GetMemberIDs возвращает список ID всех участников чата.
func (r *Repository) GetMemberIDs(ctx context.Context, chatID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT user_id FROM chat_members WHERE chat_id = $1
	`, chatID)
	if err != nil {
		return nil, fmt.Errorf("get member ids: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan member id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return ids, nil
}

// GetMembers возвращает список участников с username через JOIN users.
func (r *Repository) GetMembers(ctx context.Context, chatID string) ([]ChatMember, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT cm.chat_id, cm.user_id, u.username, cm.joined_at
		FROM chat_members cm
		INNER JOIN users u ON u.id = cm.user_id
		WHERE cm.chat_id = $1
		ORDER BY cm.joined_at ASC
	`, chatID)
	if err != nil {
		return nil, fmt.Errorf("get members: %w", err)
	}
	defer rows.Close()

	return scanChatMembers(rows)
}

// batchGetMembers возвращает участников для нескольких чатов одним запросом.
// Возвращает map[chatID][]ChatMember.
func (r *Repository) batchGetMembers(ctx context.Context, chatIDs []string) (map[string][]ChatMember, error) {
	if len(chatIDs) == 0 {
		return nil, nil
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT cm.chat_id, cm.user_id, u.username, cm.joined_at
		FROM chat_members cm
		INNER JOIN users u ON u.id = cm.user_id
		WHERE cm.chat_id = ANY($1::uuid[])
		ORDER BY cm.chat_id, cm.joined_at ASC
	`, pq.Array(chatIDs))
	if err != nil {
		return nil, fmt.Errorf("batch get members: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]ChatMember)
	for rows.Next() {
		var m ChatMember
		if err := rows.Scan(&m.ChatID, &m.UserID, &m.Username, &m.JoinedAt); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		result[m.ChatID] = append(result[m.ChatID], m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return result, nil
}

// ListUserChats возвращает все чаты пользователя с последним сообщением и участниками.
func (r *Repository) ListUserChats(ctx context.Context, userID string) ([]ChatResponse, error) {
	// Шаг 1: получаем чаты с последним сообщением через LEFT JOIN LATERAL
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			c.id, c.type, COALESCE(c.name, '') as name, COALESCE(c.created_by::text, '') as created_by,
			c.created_at, c.updated_at,
			lm.content AS last_msg_content,
			lm.sent_at AS last_msg_sent_at,
			lm.sender_id AS last_msg_sender_id
		FROM chats c
		INNER JOIN chat_members cm ON cm.chat_id = c.id
		LEFT JOIN LATERAL (
			SELECT m.content, m.sent_at, m.sender_id
			FROM messages m
			WHERE m.chat_id = c.id
			ORDER BY m.sent_at DESC
			LIMIT 1
		) lm ON true
		WHERE cm.user_id = $1
		ORDER BY COALESCE(lm.sent_at, c.created_at) DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user chats: %w", err)
	}
	defer rows.Close()

	type chatRow struct {
		Chat
		LastMsgContent  *string
		LastMsgSentAt   *time.Time
		LastMsgSenderID *string
	}

	var chatRows []chatRow
	var chatIDs []string

	for rows.Next() {
		var cr chatRow
		var name, createdBy string
		var lastContent, lastSenderID sql.NullString
		var lastSentAt sql.NullTime

		err := rows.Scan(
			&cr.ID, &cr.Type, &name, &createdBy,
			&cr.CreatedAt, &cr.UpdatedAt,
			&lastContent, &lastSentAt, &lastSenderID,
		)
		if err != nil {
			return nil, fmt.Errorf("scan chat row: %w", err)
		}

		cr.Name = name
		cr.CreatedBy = createdBy

		if lastContent.Valid {
			cr.LastMsgContent = &lastContent.String
		}
		if lastSentAt.Valid {
			cr.LastMsgSentAt = &lastSentAt.Time
		}
		if lastSenderID.Valid {
			cr.LastMsgSenderID = &lastSenderID.String
		}

		chatRows = append(chatRows, cr)
		chatIDs = append(chatIDs, cr.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	if len(chatRows) == 0 {
		return []ChatResponse{}, nil
	}

	// Шаг 2: batch-запрос участников для всех найденных чатов
	membersMap, err := r.batchGetMembers(ctx, chatIDs)
	if err != nil {
		return nil, fmt.Errorf("batch get members: %w", err)
	}

	// Шаг 3: сборка ChatResponse
	responses := make([]ChatResponse, 0, len(chatRows))
	for _, cr := range chatRows {
		resp := ChatResponse{
			Chat:    cr.Chat,
			Members: membersMap[cr.ID],
		}
		if resp.Members == nil {
			resp.Members = []ChatMember{}
		}
		if cr.LastMsgContent != nil {
			resp.LastMessage = &LastMessage{
				Content:  *cr.LastMsgContent,
				SentAt:   *cr.LastMsgSentAt,
				SenderID: *cr.LastMsgSenderID,
			}
		}
		responses = append(responses, resp)
	}

	return responses, nil
}

// GetByID возвращает чат по ID.
func (r *Repository) GetByID(ctx context.Context, chatID string) (*Chat, error) {
	chat := &Chat{}
	var name, createdBy sql.NullString

	err := r.db.QueryRowContext(ctx, `
		SELECT id, type, name, created_by, created_at, updated_at
		FROM chats
		WHERE id = $1
	`, chatID).Scan(
		&chat.ID, &chat.Type, &name, &createdBy,
		&chat.CreatedAt, &chat.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("chat %s: %w", chatID, ErrChatNotFound)
		}
		return nil, fmt.Errorf("get by id: %w", err)
	}

	chat.Name = name.String
	if createdBy.Valid {
		chat.CreatedBy = createdBy.String
	}
	return chat, nil
}

// IsGroupChat проверяет, является ли чат групповым.
func (r *Repository) IsGroupChat(ctx context.Context, chatID string) (bool, error) {
	var chatType string
	err := r.db.QueryRowContext(ctx, `
		SELECT type FROM chats WHERE id = $1
	`, chatID).Scan(&chatType)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, fmt.Errorf("chat %s: %w", chatID, ErrChatNotFound)
		}
		return false, fmt.Errorf("is group chat: %w", err)
	}
	return chatType == "group", nil
}

// scanChatMembers — вспомогательная функция для сканирования []ChatMember из rows.
func scanChatMembers(rows *sql.Rows) ([]ChatMember, error) {
	var members []ChatMember
	for rows.Next() {
		var m ChatMember
		if err := rows.Scan(&m.ChatID, &m.UserID, &m.Username, &m.JoinedAt); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return members, nil
}

// isUniqueViolation проверяет, является ли ошибка нарушением уникальности (код 23505).
func isUniqueViolation(err error) bool {
	if pqErr, ok := err.(*pq.Error); ok {
		return pqErr.Code == "23505"
	}
	return false
}
