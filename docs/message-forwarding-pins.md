# Message forwarding and pinned messages

## Forward a text message

`POST /api/v1/chats/{sourceChatID}/messages/{messageID}/forward`

Requires membership in both the source and destination chats. The destination
must allow the current user to post (owners/admins for channels). The API
creates a new message with the original body and immutable source identifiers.

Request:

```json
{ "destination_chat_id": 42 }
```

Response includes `forwarded_from_chat_id`,
`forwarded_from_message_id`, and `forwarded_from_username`. The username is
a snapshot so attribution remains stable if the source user later changes it.
The initial implementation forwards text messages only; media forwarding is
rejected rather than silently dropping attachments.

## Pin a message

| Method | Route | Description |
|---|---|---|
| POST | `/chats/{chatID}/pin` | Pin a message; body: `{"message_id":123}` |
| GET | `/chats/{chatID}/pin` | Read the pinned message or `null` |
| DELETE | `/chats/{chatID}/pin` | Remove the current pin |

Direct-chat participants can pin and unpin. In groups and channels only
owners/admins can pin. A chat has one active pinned message. Soft-deleting that
message automatically clears the pin. Every read and mutation is scoped to
the caller's chat membership.
