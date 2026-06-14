package user

import (
	"context"
	"database/sql"
	"fmt"

	"my_messanger/internal/pkg/apperror"
)

// Repository инкапсулирует SQL-запросы к таблице users.
type Repository struct {
	db *sql.DB
}

// NewRepository создаёт новый экземпляр репозитория.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// FindByID возвращает пользователя по ID. Если не найден — (nil, nil).
func (r *Repository) FindByID(ctx context.Context, id string) (*User, error) {
	query := `
		SELECT id, phone_number, username, created_at, updated_at
		FROM users
		WHERE id = $1`

	user := &User{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.PhoneNumber,
		&user.Username,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("user.FindByID: scan row: %w", err)
	}

	return user, nil
}

// Update обновляет username и updated_at для заданного пользователя.
func (r *Repository) Update(ctx context.Context, id, username string) error {
	query := `
		UPDATE users
		SET username = $1, updated_at = NOW()
		WHERE id = $2`

	result, err := r.db.ExecContext(ctx, query, username, id)
	if err != nil {
		return fmt.Errorf("user.Update: exec: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("user.Update: rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return apperror.NotFound("user not found")
	}

	return nil
}

// SearchByPhone ищет пользователей по LIKE-паттерну номера телефона,
// исключая пользователя с excludeID. Результат ограничен limit.
// Параметр pattern должен уже содержать символы % (например, "%916%").
func (r *Repository) SearchByPhone(ctx context.Context, pattern, excludeID string, limit int) ([]User, error) {
	query := `
		SELECT id, phone_number, username, created_at, updated_at
		FROM users
		WHERE phone_number LIKE $1 AND id <> $2
		ORDER BY username
		LIMIT $3`

	rows, err := r.db.QueryContext(ctx, query, pattern, excludeID, limit)
	if err != nil {
		return nil, fmt.Errorf("user.SearchByPhone: query: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(
			&u.ID,
			&u.PhoneNumber,
			&u.Username,
			&u.CreatedAt,
			&u.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("user.SearchByPhone: scan row: %w", err)
		}
		users = append(users, u)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("user.SearchByPhone: rows iteration: %w", err)
	}

	return users, nil
}

// Exists проверяет существование пользователя по ID.
func (r *Repository) Exists(ctx context.Context, id string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`

	var exists bool
	err := r.db.QueryRowContext(ctx, query, id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("user.Exists: %w", err)
	}

	return exists, nil
}
