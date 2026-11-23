package store

import (
	"context"
	"strings"
	"time"
)

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	Bio          string    `json:"bio"`
	AvatarURL    string    `json:"avatar_url"`
	CreatedAt    time.Time `json:"created_at"`
	PasswordHash string    `json:"-"`
}

type PublicProfile struct {
	ID          int64     `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Bio         string    `json:"bio"`
	AvatarURL   string    `json:"avatar_url"`
	CreatedAt   time.Time `json:"created_at"`
	Followers   int64     `json:"followers"`
	Following   int64     `json:"following"`
	IsFollowing bool      `json:"is_following"`
}

const userColumns = `id,username,email,display_name,bio,avatar_url,created_at`

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &u.Bio, &u.AvatarURL, &u.CreatedAt)
	return u, norm(err)
}

func (s *Store) CreateUser(ctx context.Context, username, email, passwordHash string) (User, error) {
	return scanUser(s.Pool.QueryRow(ctx,
		`INSERT INTO users(username,email,password_hash,display_name) VALUES($1,$2,$3,$1)
		 RETURNING `+userColumns, username, email, passwordHash))
}

// UserByEmail also returns the password hash; callers must never serialise it.
func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	var u User
	err := s.Pool.QueryRow(ctx,
		`SELECT id,username,email,display_name,password_hash,bio,avatar_url,created_at
		 FROM users WHERE email=$1`, email,
	).Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &u.PasswordHash, &u.Bio, &u.AvatarURL, &u.CreatedAt)
	return u, norm(err)
}

func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	return scanUser(s.Pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id=$1`, id))
}

func (s *Store) UpdateProfile(ctx context.Context, id int64, displayName, bio, avatarURL string) (User, error) {
	return scanUser(s.Pool.QueryRow(ctx,
		`UPDATE users SET display_name=$2,bio=$3,avatar_url=$4 WHERE id=$1 RETURNING `+userColumns,
		id, displayName, bio, avatarURL))
}

const profileSelect = `SELECT u.id,u.username,u.display_name,u.bio,u.avatar_url,u.created_at,
	(SELECT count(*) FROM follows WHERE following_id=u.id),
	(SELECT count(*) FROM follows WHERE follower_id=u.id),
	EXISTS(SELECT 1 FROM follows WHERE follower_id=$2 AND following_id=u.id)
	FROM users u `

func scanProfile(row interface{ Scan(...any) error }) (PublicProfile, error) {
	var p PublicProfile
	err := row.Scan(&p.ID, &p.Username, &p.DisplayName, &p.Bio, &p.AvatarURL, &p.CreatedAt,
		&p.Followers, &p.Following, &p.IsFollowing)
	return p, norm(err)
}

func (s *Store) PublicProfile(ctx context.Context, username string, viewerID int64) (PublicProfile, error) {
	return scanProfile(s.Pool.QueryRow(ctx,
		profileSelect+`WHERE lower(u.username)=lower($1)`, username, viewerID))
}

// escapeLike neutralises LIKE wildcards in user input so a query such as "%"
// cannot be used to enumerate every account.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

func (s *Store) SearchUsers(ctx context.Context, query string, viewerID int64) ([]PublicProfile, error) {
	rows, err := s.Pool.Query(ctx,
		profileSelect+`WHERE u.id<>$2 AND (u.username ILIKE $1 OR u.display_name ILIKE $1)
		 ORDER BY u.username LIMIT 20`,
		"%"+escapeLike(query)+"%", viewerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PublicProfile{}
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) Follow(ctx context.Context, followerID, targetID int64) error {
	tag, err := s.Pool.Exec(ctx,
		`INSERT INTO follows(follower_id,following_id)
		 SELECT $1,$2 WHERE EXISTS(SELECT 1 FROM users WHERE id=$2)
		 ON CONFLICT DO NOTHING`, followerID, targetID)
	if err != nil {
		return norm(err)
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := s.Pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)`, targetID,
		).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

func (s *Store) Unfollow(ctx context.Context, followerID, targetID int64) error {
	_, err := s.Pool.Exec(ctx,
		`DELETE FROM follows WHERE follower_id=$1 AND following_id=$2`, followerID, targetID)
	return err
}

// FollowingUsers returns the accounts the viewer follows, which form the
// initial contact list. The viewer ID is also used for relationship metadata.
func (s *Store) FollowingUsers(ctx context.Context, viewerID int64) ([]PublicProfile, error) {
	rows, err := s.Pool.Query(ctx,
		profileSelect+`WHERE EXISTS (
			SELECT 1 FROM follows f
			WHERE f.follower_id=$1 AND f.following_id=u.id
		) ORDER BY lower(u.username)`,
		viewerID, viewerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PublicProfile{}
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
