# Realtime events

KentGram uses an authenticated WebSocket endpoint for low-latency chat events:

`wss://api.kentgram.tendercheck.pro/api/v1/realtime`

Clients authenticate the handshake with the same Bearer access token used by
the REST API. The endpoint is deliberately outside the ordinary 20-second
request timeout. Server-side ping/pong frames keep idle connections alive;
the server periodically checks that the backing session has not been revoked.

## Server-to-client events

```json
{"type":"message.created","chat_id":42,"message_id":901}
{"type":"message.deleted","chat_id":42,"message_id":901}
{"type":"typing","chat_id":42,"user_id":17}
{"type":"chat.read","chat_id":42,"message_id":901,"user_id":17}
```

Message events contain identifiers only. Clients fetch canonical message
content through the existing authenticated REST endpoint, so event delivery
does not bypass chat membership checks or duplicate message serialization.

## Client-to-server events

```json
{"type":"typing","chat_id":42}
```

Typing events are accepted only after the server verifies that the connected
user is a member of the referenced chat. They are broadcast to other current
members, never to arbitrary users.

Read acknowledgements use the authenticated REST endpoint
`POST /api/v1/chats/{chatID}/read` with `last_read_message_id`. The server
advances the member's cursor monotonically, clamps it to the latest visible
message in that chat, and emits `chat.read` to the chat's current members.
Fetching message history alone does not advance the cursor. Clients can obtain
the canonical member cursors from `GET /api/v1/chats/{chatID}/read`.

## Delivery model

The current hub is process-local and intended for the single API instance in
the current Compose deployment. Each connection has a bounded event queue;
when a client falls behind, events may be dropped and the client recovers by
fetching the latest chat state over REST. A multi-instance deployment will
need a shared broker (for example Redis Pub/Sub or NATS) between API instances.
