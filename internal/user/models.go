package user

import "time"

// User — доменная модель пользователя.
// Поле hashed_password никогда не покидает репозиторий.
type User struct {
	ID          string    `json:"id"`
	PhoneNumber string    `json:"phone_number"`
	Username    string    `json:"username"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UpdateProfileRequest — тело запроса на обновление профиля.
type UpdateProfileRequest struct {
	Username string `json:"username"`
}

// SearchResponse — урезанная модель для результатов поиска.
type SearchResponse struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	PhoneNumber string `json:"phone_number"` // маскированный
}
