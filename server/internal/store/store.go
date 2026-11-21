package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the PostgreSQL access layer. Each domain keeps its queries in its
// own file (users.go, sessions.go, ...) on this one type.
type Store struct {
	Pool *pgxpool.Pool
}

var (
	// ErrConflict is returned when a unique constraint is violated.
	ErrConflict = errors.New("record already exists")
	// ErrNotFound is returned when a row does not exist or is not visible to
	// the caller. Visibility failures deliberately look identical to absence.
	ErrNotFound = pgx.ErrNoRows
)

func Open(ctx context.Context, url string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
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

// norm maps driver errors onto the package's sentinel errors.
func norm(err error) error {
	if err == nil || errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return err
}
