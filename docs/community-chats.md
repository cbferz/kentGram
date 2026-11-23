# Groups and channels

The first community-chat slice keeps KentGram's existing Go API and PostgreSQL
model. Existing direct chats remain `kind: "direct"`; communities use
`kind: "group"` or `kind: "channel"`.

## Chat creation

Authenticated endpoint:

`POST /api/v1/chats/community`

Private group:

```json
{
  "kind": "group",
  "title": "Project team",
  "description": "Internal discussion",
  "is_public": false
}
```

Public channel:

```json
{
  "kind": "channel",
  "title": "KentGram updates",
  "description": "Official announcements",
  "is_public": true,
  "username": "kentgram_updates"
}
```

Public usernames are case-insensitively unique. A community creator becomes
its `owner`. Public communities can be joined with
`POST /api/v1/chats/join` and `{"username":"kentgram_updates"}`.

## Membership and roles

All routes require a valid Bearer access token.

| Method | Route | Permission |
|---|---|---|
| GET | `/chats/{chatID}` | Member |
| GET | `/chats/{chatID}/members` | Member |
| POST | `/chats/{chatID}/members` | Owner or admin |
| PATCH | `/chats/{chatID}/members/{userID}/role` | Owner |
| DELETE | `/chats/{chatID}/members/{userID}` | Admin removes members; owner removes members/admins |
| POST | `/chats/{chatID}/leave` | Member/admin; owner must transfer ownership first |
| POST | `/chats/{chatID}/transfer-ownership` | Owner |

Roles are `owner`, `admin`, and `member`. The owner cannot be removed or
demoted through the member-role endpoint. Ownership transfer is transactional:
the previous owner becomes an admin and an existing member becomes the owner.

Channel message creation is restricted at the database query boundary to
owners and admins. Group members can send messages. Membership checks remain
scoped to the exact chat ID.

## Database migration

`0005_groups_channels.sql` extends the chat-kind constraint and adds community
metadata and member roles. Existing migrations are immutable; direct chats
adopt the default empty metadata and `member` role.


## Read receipts

- `GET /api/v1/chats/{chatID}/read` returns a member-to-message read cursor map.
- `POST /api/v1/chats/{chatID}/read` accepts
  `{"last_read_message_id": 123}` and advances the caller's cursor monotonically.
- A read cursor is clamped to the latest non-deleted message in that chat.
- Merely fetching message history does not mark messages as read.
- Advancing a cursor emits a `chat.read` WebSocket event to the chat's current members.


## Media attachments

- `POST /api/v1/chats/{chatID}/media` accepts multipart fields `file` and optional `caption`.
- Initial allowlist: JPEG, PNG, and PDF; maximum upload size is 20 MiB.
- Images are dimension-limited and validated from their actual file bytes.
- The API stores files under `MEDIA_DIR`; Compose persists this directory in the
  `kentgram_media` volume.
- Message history includes `media_id`, `media_url`, `media_mime`,
  `media_name`, and `media_size`.
- `GET /api/v1/chats/{chatID}/media/{mediaID}` requires current chat membership.
  Deleted messages no longer expose their media.
- Only channel owners and admins can publish attachments, matching channel text
  posting permissions.
