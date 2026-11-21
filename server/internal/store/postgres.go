package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool" 
)

type Store struct {
	Pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 8
	cfg.MinConns = 1
	cfg.MaxConnLifetime = time.Hour
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{Pool: pool}, nil
}

func (s *Store) Close() { s.Pool.Close() }

func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(32) NOT NULL UNIQUE,
    email VARCHAR(254) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS users_created_at_idx ON users(created_at);
	return err
}

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	CreatedAt    time.Time `json:"created_at"`
	PasswordHash string    `json:"-"`
}

func (s *Store) CreateUser(ctx context.Context, username, email, passwordHash string) (User, error) {
	var user User
	err := s.Pool.QueryRow(ctx, `
INSERT INTO users(username, email, password_hash, display_name)
VALUES($1, $2, $3, $1)
RETURNING id, username, email, display_name, created_at
`, username, email, passwordHash).Scan(
		&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.CreatedAt,
	)
	return user, normalizeDBError(err)
}

func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	var user User
	err := s.Pool.QueryRow(ctx, `
SELECT id, username, email, display_name, password_hash, created_at
FROM users WHERE email = $1
`, email).Scan(
		&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.PasswordHash, &user.CreatedAt,
	)
	return user, normalizeDBError(err)
}

func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	var user User
	err := s.Pool.QueryRow(ctx, `
SELECT id, username, email, display_name, created_at
FROM users WHERE id = $1
`, id).Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.CreatedAt)
	return user, normalizeDBError(err)
}

func normalizeDBError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err != nil && strings.Contains(err.Error(), "duplicate key") {
		return ErrConflict
	}
	return err
}

var ErrConflict = errors.New("record already exists")
