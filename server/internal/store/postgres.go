package store
import("context";"errors";"strings";"time";"github.com/jackc/pgx/v5";"github.com/jackc/pgx/v5/pgxpool")
type Store struct{Pool *pgxpool.Pool}
func Open(ctx context.Context,url string)(*Store,error){c,e:=pgxpool.ParseConfig(url);if e!=nil{return nil,e};c.MaxConns=8;c.MinConns=1;c.MaxConnLifetime=time.Hour;p,e:=pgxpool.NewWithConfig(ctx,c);if e!=nil{return nil,e};if e=p.Ping(ctx);e!=nil{p.Close();return nil,e};return &Store{p},nil}
func(s *Store)Close(){s.Pool.Close()}
func(s *Store)Migrate(ctx context.Context)error{_,e:=s.Pool.Exec(ctx,`
CREATE TABLE IF NOT EXISTS users(id BIGSERIAL PRIMARY KEY,username VARCHAR(32) NOT NULL UNIQUE,email VARCHAR(254) NOT NULL UNIQUE,password_hash TEXT NOT NULL,display_name VARCHAR(64) NOT NULL DEFAULT '',created_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
CREATE INDEX IF NOT EXISTS users_created_at_idx ON users(created_at);
ALTER TABLE users ADD COLUMN IF NOT EXISTS bio VARCHAR(160) NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_url TEXT NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS follows(follower_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,following_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),PRIMARY KEY(follower_id,following_id),CHECK(follower_id<>following_id));
CREATE INDEX IF NOT EXISTS follows_following_idx ON follows(following_id,created_at DESC);
CREATE INDEX IF NOT EXISTS follows_follower_idx ON follows(follower_id,created_at DESC);`);return e}
type User struct{ID int64 `json:"id"`;Username string `json:"username"`;Email string `json:"email"`;DisplayName string `json:"display_name"`;Bio string `json:"bio"`;AvatarURL string `json:"avatar_url"`;CreatedAt time.Time `json:"created_at"`;PasswordHash string `json:"-"`}
type PublicProfile struct{ID int64 `json:"id"`;Username string `json:"username"`;DisplayName string `json:"display_name"`;Bio string `json:"bio"`;AvatarURL string `json:"avatar_url"`;CreatedAt time.Time `json:"created_at"`;Followers int64 `json:"followers"`;Following int64 `json:"following"`;IsFollowing bool `json:"is_following"`}
func(s *Store)CreateUser(ctx context.Context,n,e,h string)(User,error){var u User;err:=s.Pool.QueryRow(ctx,`INSERT INTO users(username,email,password_hash,display_name) VALUES($1,$2,$3,$1) RETURNING id,username,email,display_name,bio,avatar_url,created_at`,n,e,h).Scan(&u.ID,&u.Username,&u.Email,&u.DisplayName,&u.Bio,&u.AvatarURL,&u.CreatedAt);return u,norm(err)}
func(s *Store)UserByEmail(ctx context.Context,email string)(User,error){var u User;err:=s.Pool.QueryRow(ctx,`SELECT id,username,email,display_name,password_hash,bio,avatar_url,created_at FROM users WHERE email=$1`,email).Scan(&u.ID,&u.Username,&u.Email,&u.DisplayName,&u.PasswordHash,&u.Bio,&u.AvatarURL,&u.CreatedAt);return u,norm(err)}
func(s *Store)UserByID(ctx context.Context,id int64)(User,error){var u User;err:=s.Pool.QueryRow(ctx,`SELECT id,username,email,display_name,bio,avatar_url,created_at FROM users WHERE id=$1`,id).Scan(&u.ID,&u.Username,&u.Email,&u.DisplayName,&u.Bio,&u.AvatarURL,&u.CreatedAt);return u,norm(err)}
func(s *Store)UpdateProfile(ctx context.Context,id int64,n,b,a string)(User,error){var u User;err:=s.Pool.QueryRow(ctx,`UPDATE users SET display_name=$2,bio=$3,avatar_url=$4 WHERE id=$1 RETURNING id,username,email,display_name,bio,avatar_url,created_at`,id,n,b,a).Scan(&u.ID,&u.Username,&u.Email,&u.DisplayName,&u.Bio,&u.AvatarURL,&u.CreatedAt);return u,norm(err)}
const profileSelect=`SELECT u.id,u.username,u.display_name,u.bio,u.avatar_url,u.created_at,(SELECT count(*) FROM follows WHERE following_id=u.id),(SELECT count(*) FROM follows WHERE follower_id=u.id),EXISTS(SELECT 1 FROM follows WHERE follower_id=$2 AND following_id=u.id) FROM users u `
func(s *Store)PublicProfile(ctx context.Context,n string,v int64)(PublicProfile,error){var p PublicProfile;e:=s.Pool.QueryRow(ctx,profileSelect+`WHERE lower(u.username)=lower($1)`,n,v).Scan(&p.ID,&p.Username,&p.DisplayName,&p.Bio,&p.AvatarURL,&p.CreatedAt,&p.Followers,&p.Following,&p.IsFollowing);return p,norm(e)}
func(s *Store)SearchUsers(ctx context.Context,q string,v int64)([]PublicProfile,error){rows,e:=s.Pool.Query(ctx,profileSelect+`WHERE u.id<>$2 AND (u.username ILIKE $1 OR u.display_name ILIKE $1) ORDER BY u.username LIMIT 20`,"%"+q+"%",v);if e!=nil{return nil,e};defer rows.Close();out:=[]PublicProfile{};for rows.Next(){var p PublicProfile;if e=rows.Scan(&p.ID,&p.Username,&p.DisplayName,&p.Bio,&p.AvatarURL,&p.CreatedAt,&p.Followers,&p.Following,&p.IsFollowing);e!=nil{return nil,e};out=append(out,p)};return out,rows.Err()}
func(s *Store)Follow(ctx context.Context,f,t int64)error{tag,e:=s.Pool.Exec(ctx,`INSERT INTO follows(follower_id,following_id) SELECT $1,$2 WHERE EXISTS(SELECT 1 FROM users WHERE id=$2) ON CONFLICT DO NOTHING`,f,t);if e==nil&&tag.RowsAffected()==0{return pgx.ErrNoRows};return norm(e)}
func(s *Store)Unfollow(ctx context.Context,f,t int64)error{_,e:=s.Pool.Exec(ctx,`DELETE FROM follows WHERE follower_id=$1 AND following_id=$2`,f,t);return e}
func norm(e error)error{if errors.Is(e,pgx.ErrNoRows){return e};if e!=nil&&strings.Contains(e.Error(),"duplicate key"){return ErrConflict};return e}
var ErrConflict=errors.New("record already exists")

var ErrNotFound = pgx.ErrNoRows
