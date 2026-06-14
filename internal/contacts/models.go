package contacts

import (
	"time"

	"my_messanger/internal/user"
)

// Contact — доменная модель контакта.
type Contact struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	ContactUserID string    `json:"contact_user_id"`
	CreatedAt     time.Time `json:"created_at"`
}

// ContactResponse — ответ API с данными пользователя-контакта.
type ContactResponse struct {
	ID          string              `json:"id"`
	ContactUser user.SearchResponse `json:"contact_user"`
	CreatedAt   time.Time           `json:"created_at"`
}

// AddContactRequest — тело запроса на добавление контакта.
type AddContactRequest struct {
	ContactUserID string `json:"contact_user_id"`
}
