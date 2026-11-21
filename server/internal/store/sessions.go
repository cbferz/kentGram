package store

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrTokenReuse means a refresh token that was already rotated out was
// presented again. The session is revoked as a precaution because the token
// may have been copied by someone else.
var ErrTokenReuse = errors.New("refresh token reuse detected")

type Session struct {
	ID         int64     `json:"id"`
	DeviceName string    `json:"device_name"`
	UserAgent  string    `json:"user_agent"`
	IP         string    `json:"ip"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Current    bool      `json:"current"`
}

func (s *Store) CreateSession(ctx context.Context, userID int64, refreshHash []byte,
	device, userAgent, ip string, expires time.Time) (int64, error) {
	var id int64
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO sessions(user_id,refresh_hash,device_name,user_agent,ip,expires_at)
		 VALUES($1,$2,$3,$4,$5,$6) RETURNING id`,
		userID, refreshHash, device, userAgent, ip, expires).Scan(&id)
	return id, norm(err)
}

// SessionActive reports whether the session belongs to the user, is not
// revoked and has not expired. Access tokens are only honoured while it is.
func (s *Store) SessionActive(ctx context.Context, sessionID, userID int64) (bool, error) {
	var active bool
	err := s.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM sessions
		   WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL AND expires_at>NOW())`,
		sessionID, userID).Scan(&active)
	return active, err
}

type Rotation struct {
	SessionID int64
	UserID    int64
}

// RotateSession swaps the session's refresh token for a new one. The presented
// token must be the current one. Presenting the immediately previous token is
// treated as theft: the session is revoked and ErrTokenReuse is returned.
func (s *Store) RotateSession(ctx context.Context, presented, replacement []byte, newExpiry time.Time) (Rotation, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Rotation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		id, userID int64
		current    []byte
		previous   []byte
		expiresAt  time.Time
		revokedAt  *time.Time
	)
	err = tx.QueryRow(ctx,
		`SELECT id,user_id,refresh_hash,prev_refresh_hash,expires_at,revoked_at
		 FROM sessions WHERE refresh_hash=$1 OR prev_refresh_hash=$1 FOR UPDATE`,
		presented).Scan(&id, &userID, &current, &previous, &expiresAt, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Rotation{}, ErrNotFound
	}
	if err != nil {
		return Rotation{}, err
	}
	if revokedAt != nil || !expiresAt.After(time.Now()) {
		return Rotation{}, ErrNotFound
	}

	if !bytes.Equal(current, presented) {
		// The presented token matched prev_refresh_hash: it was already used.
		if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=NOW() WHERE id=$1`, id); err != nil {
			return Rotation{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Rotation{}, err
		}
		return Rotation{SessionID: id, UserID: userID}, ErrTokenReuse
	}

	if _, err := tx.Exec(ctx,
		`UPDATE sessions SET prev_refresh_hash=refresh_hash, refresh_hash=$2,
		   expires_at=$3, last_used_at=NOW() WHERE id=$1`,
		id, replacement, newExpiry); err != nil {
		return Rotation{}, norm(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Rotation{}, err
	}
	return Rotation{SessionID: id, UserID: userID}, nil
}

func (s *Store) ListSessions(ctx context.Context, userID, currentID int64) ([]Session, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id,device_name,user_agent,ip,created_at,last_used_at,expires_at
		 FROM sessions WHERE user_id=$1 AND revoked_at IS NULL AND expires_at>NOW()
		 ORDER BY last_used_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Session{}
	for rows.Next() {
		var x Session
		if err := rows.Scan(&x.ID, &x.DeviceName, &x.UserAgent, &x.IP,
			&x.CreatedAt, &x.LastUsedAt, &x.ExpiresAt); err != nil {
			return nil, err
		}
		x.Current = x.ID == currentID
		out = append(out, x)
	}
	return out, rows.Err()
}

// RevokeSession revokes one of the user's own sessions. A session id that
// belongs to somebody else is indistinguishable from one that does not exist.
func (s *Store) RevokeSession(ctx context.Context, sessionID, userID int64) error {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE sessions SET revoked_at=NOW()
		 WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`, sessionID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RevokeOtherSessions signs the user out everywhere except the current device.
func (s *Store) RevokeOtherSessions(ctx context.Context, userID, keepID int64) (int64, error) {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE sessions SET revoked_at=NOW()
		 WHERE user_id=$1 AND id<>$2 AND revoked_at IS NULL`, userID, keepID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
