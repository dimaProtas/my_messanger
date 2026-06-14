package contacts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"my_messanger/internal/pkg/apperror"
	"my_messanger/internal/user"
)

// Repository инкапсулирует SQL-запросы к таблице contacts.
type Repository struct {
	db *sql.DB
}

// NewRepository создаёт новый экземпляр репозитория.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// Add создаёт новый контакт. При нарушении UNIQUE(user_id, contact_user_id)
// возвращает apperror.Conflict. Не проверяет существование пользователей —
// это ответственность сервисного слоя.
func (r *Repository) Add(ctx context.Context, userID, contactUserID string) (Contact, error) {
	query := `
		INSERT INTO contacts (user_id, contact_user_id)
		VALUES ($1, $2)
		RETURNING id, user_id, contact_user_id, created_at`

	var c Contact
	err := r.db.QueryRowContext(ctx, query, userID, contactUserID).Scan(
		&c.ID,
		&c.UserID,
		&c.ContactUserID,
		&c.CreatedAt,
	)
	if err != nil {
		// Код ошибки PostgreSQL для unique_violation — 23505.
		if isUniqueViolation(err) {
			return Contact{}, apperror.Conflict("contact already exists")
		}
		// Код ошибки для foreign_key_violation — 23503.
		if isForeignKeyViolation(err) {
			return Contact{}, apperror.NotFound("contact user not found")
		}
		return Contact{}, fmt.Errorf("contacts.Add: %w", err)
	}

	return c, nil
}

// Remove удаляет контакт. Проверяет принадлежность контакта пользователю
// через условие WHERE id = $1 AND user_id = $2.
func (r *Repository) Remove(ctx context.Context, contactID, userID string) error {
	query := `DELETE FROM contacts WHERE id = $1 AND user_id = $2`

	result, err := r.db.ExecContext(ctx, query, contactID, userID)
	if err != nil {
		return fmt.Errorf("contacts.Remove: exec: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("contacts.Remove: rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return apperror.NotFound("contact not found or does not belong to user")
	}

	return nil
}

// List возвращает список контактов пользователя с JOIN к users
// для получения username и phone_number контакта.
func (r *Repository) List(ctx context.Context, userID string) ([]ContactResponse, error) {
	query := `
		SELECT
			c.id,
			c.created_at,
			u.id,
			u.username,
			u.phone_number
		FROM contacts c
		JOIN users u ON c.contact_user_id = u.id
		WHERE c.user_id = $1
		ORDER BY c.created_at DESC`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("contacts.List: query: %w", err)
	}
	defer rows.Close()

	var result []ContactResponse
	for rows.Next() {
		var cr ContactResponse
		var cu user.SearchResponse
		if err := rows.Scan(
			&cr.ID,
			&cr.CreatedAt,
			&cu.ID,
			&cu.Username,
			&cu.PhoneNumber,
		); err != nil {
			return nil, fmt.Errorf("contacts.List: scan row: %w", err)
		}
		cr.ContactUser = cu
		result = append(result, cr)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("contacts.List: rows iteration: %w", err)
	}

	return result, nil
}

// Exists проверяет, существует ли контакт между userID и contactUserID.
func (r *Repository) Exists(ctx context.Context, userID, contactUserID string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM contacts WHERE user_id = $1 AND contact_user_id = $2)`

	var exists bool
	err := r.db.QueryRowContext(ctx, query, userID, contactUserID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("contacts.Exists: %w", err)
	}

	return exists, nil
}

// isUniqueViolation проверяет, является ли ошибка нарушением уникальности (код 23505).
// lib/pq возвращает *pq.Error, которую можно проверить по Code.
func isUniqueViolation(err error) bool {
	return isPqErrorCode(err, "23505")
}

// isForeignKeyViolation проверяет, является ли ошибка нарушением внешнего ключа (код 23503).
func isForeignKeyViolation(err error) bool {
	return isPqErrorCode(err, "23503")
}

// isPqErrorCode проверяет код ошибки PostgreSQL (напр. "23505" — unique_violation).
func isPqErrorCode(err error, code string) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code) == code
	}
	return false
}
