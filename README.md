# kentGram

Native Android messenger and social client with a Go/PostgreSQL backend.

## Stack

- Android client: Java, Android SDK 35, Gradle Kotlin DSL
- Backend: Go 1.23, chi HTTP router, PostgreSQL 16
- Authentication: Bearer access tokens, refresh tokens, revocable device sessions
- Realtime: authenticated WebSocket transport with bounded per-connection queues
- Media: persistent local volume for avatars and chat attachments

## Implemented messaging features

- Account registration, login, refresh, logout and session management
- Direct chats
- Groups and broadcast channels
- Public community usernames and joining public communities
- Owner/admin/member roles, invitations, moderation and ownership transfer
- Text messages, replies, editing, deletion and reactions
- Realtime message events and typing indicators
- Online/offline presence scoped to shared chats
- Per-member read cursors and realtime read receipts
- JPEG/PNG images and PDF attachments (20 MiB maximum)
- Membership-checked media downloads; deleted messages no longer expose their attachments

## Backend development

The API listens on port 8080 by default. Required environment variables:

- `DATABASE_URL`
- `JWT_SECRET` (at least 32 characters)

Optional storage variables:

- `AVATAR_DIR`
- `MEDIA_DIR`
- `PUBLIC_BASE_URL`

Database migrations are embedded and applied automatically at startup. Never edit
an already-applied migration; add the next numbered SQL file under
`server/internal/store/migrations/`.

Backend checks:

```sh
cd server
go mod tidy
go vet ./...
TEST_DATABASE_URL='postgres://kentgram:password@localhost:5432/kentgram_test?sslmode=disable' go test -race ./...
CGO_ENABLED=0 go build ./cmd/api
```

## Android build

Open the repository in Android Studio with Android SDK 35 and a compatible
Android Gradle Plugin. CI builds the debug APK and verifies its signature.
Keep the existing application ID and signing key for install-over updates.

## API and protocol notes

- Community API: [docs/community-chats.md](docs/community-chats.md)
- Realtime protocol: [docs/realtime.md](docs/realtime.md)

## Roadmap

- Video, voice and larger file attachments
- Stories, feed, posts and comments
- Privacy settings and two-factor authentication
- Scheduled messages and polls
- Voice/video calls and group calls
- Bot API and automation integrations
- Multi-instance realtime broker and object storage
