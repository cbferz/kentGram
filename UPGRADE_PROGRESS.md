# KentGram upgrade — progress (branch `feat/kentgram-2-messaging-core`)

Nothing here has been deployed. Production, its DB and `/root/kentGram` were not touched.

## Done
- **DB**: `server/internal/store/migrations/0005_message_interactions.sql` (additive, idempotent): `messages.client_id`, forwards, `pinned_messages`, `message_reactions`.
- **Realtime**: `server/internal/realtime` (stdlib WebSocket hub: auth frame, ping/pong, session revocation, origin check, per-user cap).
- **API** (`server/internal/httpapi`): edit, reactions (`POST`/`DELETE .../reactions`), forward, pins, read markers, typing, presence, `GET /ws`. Events: `message.new|edited|deleted`, `reaction.updated`, `message.pinned|unpinned`, `chat.read`, `typing`, `presence`, `ready`.
- **Web** (`web/`): full client on real routes, realtime, reactions, service worker v5.
- **Android**: `RealtimeClient.java` (pure-Java RFC 6455, wss with host-name verification, reconnect with backoff), reactions in the long-press menu (`MessageActionMenu`), reaction chips, edited/pinned/forwarded markers, live sync of the open chat and chat list in `MainActivity`.

## Verified
- SQL on real PostgreSQL: `PGHOST=127.0.0.1 PGPORT=54329 PGUSER=postgres python3 server/scripts/check_message_sql.py` -> 53 PASS, 0 FAIL.
- `go test -race ./internal/realtime` passes.
- Web e2e (`web/test`, against a TEST-ONLY mock API, not the real backend): 52/52.
- Android `RealtimeClient` on a JVM against the real Go hub: 18/18 (ready, delivery, reconnect, backoff, stop, ping keepalive, server restart) and TLS host-name verification (trusted `localhost` OK, `127.0.0.1` rejected).

## NOT verified
- Go HTTP handlers and `messages_interactions_integration_test.go` were only `go vet`-checked against stub modules (no network for Go modules). Never executed. Run with `TEST_DATABASE_URL` set before any deploy.
- Android `MainActivity`/`MessageActionMenu` changes: syntax parsed only. No Android SDK here, so no type-check, Gradle build, APK, signing or device test.
- Production code in `/root/kentGram` (hand-fixed reactions endpoint) may differ from this branch: diff before deploying.

## Not implemented
Groups, channels, communities, polls, media/uploads, message search, blocks/privacy, Android typing/presence UI.

## Before releasing Android
`versionCode` is still 1 in `app/build.gradle.kts`: set it above the shipped value. Do not change `applicationId` or the signing key.

## Release commands (run only after explicit approval)
Backend: back up the DB (`pg_dump`), `git pull`, `docker compose build api`, apply migrations (0005 is additive/idempotent), `docker compose up -d api`, check `/health`. Rollback: previous image + restore only if needed (0005 adds objects only).
Web: same-origin hosting, see `deploy/Caddyfile.web.example` (not applied; `deploy/Caddyfile` is the API-only rollback).
Android: bump `versionCode`, `./gradlew assembleRelease`, sign with the existing key.
