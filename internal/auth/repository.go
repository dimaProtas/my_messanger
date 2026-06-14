package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateUser(ctx context.Context, phoneNumber, hashedPassword, username string) (User, error) {
	const query = `INSERT INTO users (id, phone_number, hashed_password, username, created_at, updated_at) VALUES (uuid_generate_v4(), $1, $2, $3, NOW(), NOW()) RETURNING id, phone_number, username, created_at, updated_at`

	var user User
	err := r.db.QueryRowContext(ctx, query, phoneNumber, hashedPassword, username).
		Scan(&user.ID, &user.PhoneNumber, &user.Username, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, fmt.Errorf("%w: phone %s already exists", ErrAlreadyExists, phoneNumber)
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

func (r *Repository) FindUserByPhone(ctx context.Context, phoneNumber string) (*User, error) {
	const query = `SELECT id, phone_number, username, created_at, updated_at FROM users WHERE phone_number = $1`

	var user User
	err := r.db.QueryRowContext(ctx, query, phoneNumber).
		Scan(&user.ID, &user.PhoneNumber, &user.Username, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find user by phone: %w", err)
	}
	return &user, nil
}

func (r *Repository) FindUserByPhoneWithPassword(ctx context.Context, phoneNumber string) (*User, []byte, error) {
	const query = `SELECT id, phone_number, hashed_password, username, created_at, updated_at FROM users WHERE phone_number = $1`

	var user User
	var hashedPassword []byte
	err := r.db.QueryRowContext(ctx, query, phoneNumber).
		Scan(&user.ID, &user.PhoneNumber, &hashedPassword, &user.Username, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("find user by phone with password: %w", err)
	}
	return &user, hashedPassword, nil
}

func (r *Repository) FindUserByID(ctx context.Context, userID string) (*User, error) {
	const query = `SELECT id, phone_number, username, created_at, updated_at FROM users WHERE id = $1`

	var user User
	err := r.db.QueryRowContext(ctx, query, userID).
		Scan(&user.ID, &user.PhoneNumber, &user.Username, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find user by id: %w", err)
	}
	return &user, nil
}

func (r *Repository) SaveRefreshToken(ctx context.Context, userID, jti string, expiresAt time.Time) error {
	const query = `INSERT INTO refresh_tokens (id, user_id, jti, expires_at, revoked, created_at) VALUES (uuid_generate_v4(), $1, $2, $3, FALSE, NOW())`

	_, err := r.db.ExecContext(ctx, query, userID, jti, expiresAt)
	if err != nil {
		return fmt.Errorf("save refresh token: %w", err)
	}
	return nil
}

func (r *Repository) FindRefreshToken(ctx context.Context, jti string) (*RefreshToken, error) {
	const query = `SELECT user_id, jti, expires_at, revoked FROM refresh_tokens WHERE jti = $1`

	var rt RefreshToken
	err := r.db.QueryRowContext(ctx, query, jti).Scan(&rt.UserID, &rt.JTI, &rt.ExpiresAt, &rt.Revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find refresh token: %w", err)
	}
	return &rt, nil
}

func (r *Repository) RevokeRefreshToken(ctx context.Context, jti string) error {
	const query = `UPDATE refresh_tokens SET revoked = TRUE WHERE jti = $1`

	res, err := r.db.ExecContext(ctx, query, jti)
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) RevokeAllUserTokens(ctx context.Context, userID string) error {
	const query = `UPDATE refresh_tokens SET revoked = TRUE WHERE user_id = $1 AND revoked = FALSE`

	_, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("revoke all user tokens: %w", err)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505"
	}
	return strings.Contains(err.Error(), "duplicate key")
}
