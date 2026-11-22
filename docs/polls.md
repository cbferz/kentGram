# Polls and voting

Polls are scoped to a chat. Every endpoint requires a KentGram access token and verifies current chat membership.

## Create
`POST /api/v1/chats/{chatID}/polls`

```json
{
  "question": "Which feature should ship next?",
  "options": ["Voice messages", "Stories", "Calls"],
  "multiple_choice": false,
  "anonymous": true
}
```

A poll requires 2–10 options. Questions are limited to 300 characters and options to 100 characters. Only channel owners/admins may publish polls in channels; group members and direct-chat participants may create them.

## Read
`GET /api/v1/chats/{chatID}/polls/{pollID}`

Returns option vote totals and whether the current user selected each option. Voter identities are never returned by this API; the anonymous flag is retained as poll metadata for future client presentation.

## Vote
`POST /api/v1/chats/{chatID}/polls/{pollID}/vote`

```json
{ "option_ids": [12] }
```

For single-choice polls, a new vote replaces the user's previous choice. For multiple-choice polls, submitted options are added without duplicating existing votes. Poll row locking serializes concurrent votes and prevents voting after closure.

## Close
`POST /api/v1/chats/{chatID}/polls/{pollID}/close`

The poll creator or a chat owner/admin may close a poll. Closed polls remain readable, but votes are rejected.

## Realtime events

The chat WebSocket publishes `poll.created`, `poll.updated`, and `poll.closed` events with `chat_id` and `message_id` (the poll ID).
