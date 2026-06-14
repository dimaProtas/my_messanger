package chat

import (
	"errors"
	"time"
)

// Chat — основная модель чата.
type Chat struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"` // "private", "group"
	Name      string    `json:"name,omitempty"`
	CreatedBy string    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ChatMember — участник чата.
type ChatMember struct {
	ChatID   string    `json:"chat_id"`
	UserID   string    `json:"user_id"`
	Username string    `json:"username"`
	JoinedAt time.Time `json:"joined_at"`
}

// ChatResponse — расширенный ответ с участниками и последним сообщением.
type ChatResponse struct {
	Chat
	Members     []ChatMember `json:"members"`
	LastMessage *LastMessage `json:"last_message,omitempty"`
}

// LastMessage — краткая информация о последнем сообщении.
type LastMessage struct {
	Content  string    `json:"content"`
	SentAt   time.Time `json:"sent_at"`
	SenderID string    `json:"sender_id"`
}

// CreatePrivateRequest — запрос на создание приватного чата.
type CreatePrivateRequest struct {
	UserID string `json:"user_id"`
}

// CreateGroupRequest — запрос на создание группового чата.
type CreateGroupRequest struct {
	Name      string   `json:"name"`
	MemberIDs []string `json:"member_ids"`
}

// AddMemberRequest — запрос на добавление участника.
type AddMemberRequest struct {
	UserID string `json:"user_id"`
}

// ErrorResponse — стандартная структура ошибки API.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// Sentinel-ошибки доменного слоя.
var (
	ErrChatNotFound   = errors.New("chat not found")
	ErrNotMember      = errors.New("user is not a member of the chat")
	ErrNotGroupChat   = errors.New("chat is not a group chat")
	ErrAlreadyMember  = errors.New("user is already a member")
	ErrMemberNotFound = errors.New("member not found in chat")
	ErrSelfChat       = errors.New("cannot create chat with yourself")
	ErrPermissionDenied = errors.New("permission denied")
	ErrValidation     = errors.New("validation error")
)
