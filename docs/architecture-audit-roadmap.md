# kentGram — architecture audit and implementation roadmap

Audit date: 2026-11-22
Audited revision: `main` at `c8227b000e7dc25fa3b309fc37fda1d68081250a`

## Executive summary

kentGram is an early, working native Android + Go + PostgreSQL messenger foundation. Authentication, profiles, contact/user discovery, direct-chat creation, message persistence, and basic history retrieval exist. The requested advanced messenger features are not yet implemented. The current architecture can support them, but it needs a deliberate messaging-domain expansion and a realtime delivery layer before attempting the full feature set.

No production database, VPS configuration, secrets, or user data were changed during this audit.

## Current architecture

### Android
- Native Android application written in Java, package `com.kentgram`, minSdk 24, target/compile SDK 35, Java 17.
- Main screens and the chat flow are currently concentrated in `MainActivity.java`; profile is in `ProfileActivity.java`.
- Networking uses `HttpURLConnection`, JSON, Bearer access tokens, and an executor. The main activity currently uses a single-thread executor.
- Chat history is fetched through REST. There is no WebSocket client, event stream, durable outbox, local message database, or background scheduling implementation in the current source.
- The current chat UI renders text bubbles only. Message long-press actions, reply/forward/delete/edit, media composition, reactions, polls, drafts, and scheduled-message UI are absent.
- Build is automated through GitHub Actions and publishes a rolling debug APK from `main`.

### Go API
- Go 1.23, chi router, JWT authentication, pgx connection pool.
- API currently exposes authentication/session, user/profile/follow, contacts, direct chats, chat list, message history, and text-message send routes.
- Authenticated chat handlers verify chat membership before history access and send.
- No WebSocket route, event broker, notification worker, scheduled-message worker, poll endpoints, reaction endpoints, draft endpoints, or media-pack endpoints are currently registered.

### PostgreSQL and migrations
- PostgreSQL 16; schema changes are embedded SQL migrations under `server/internal/store/migrations`.
- Migration runner serializes startup migrations with a PostgreSQL advisory lock and applies each new migration transactionally. Existing migration files must remain immutable; new schema belongs in the next numbered migration.
- Current schema has users, sessions, follows, chats, chat_members, and messages. Messages are text-only and have chat/sender ownership, timestamps, and soft-delete/edit timestamp fields.
- Message IDs are global BIGSERIAL IDs, while access is scoped by chat membership. This is a good base for stable message references, but every future mutation must continue checking membership and message-to-chat ownership.

### Deployment and resource constraints
- Compose starts only the API and PostgreSQL. API container memory limit is 512 MiB; PostgreSQL is 1 GiB. The server README explicitly defers Redis and separate workers to conserve a 4 GiB VPS budget.
- The API is bound to loopback and expects a TLS reverse proxy in front.
- Do not introduce Redis, a separate media-processing stack, or heavyweight infrastructure without measuring memory and operational cost first.

## Findings and risks

1. **No realtime transport.** REST requests alone cannot provide immediate synchronized reactions, edits, deletions, typing/read state, or poll updates. Add an authenticated WebSocket event channel with reconnect/resume semantics. Keep REST as the source of truth and fallback.
2. **Message UI is currently monolithic.** Avoid adding every feature to `MainActivity.java`. Extract chat models, API client, message adapter/rendering, action handling, and screen components into focused Java classes while preserving the existing UI and navigation.
3. **Request serialization can affect perceived responsiveness.** The main activity's single-thread executor serializes network operations. Move to a bounded executor and explicit per-chat send queue/idempotency so consecutive sends do not block unrelated reads. Never reorder messages within one chat.
4. **No durable client outbox.** A failed or interrupted send currently has no persisted retry state. Add client-generated idempotency IDs and a durable pending-send queue before adding rich media or offline behavior.
5. **Message schema is text-only.** Add a typed message payload model and attachment references without putting binary data in PostgreSQL. Preserve existing text rows and API compatibility.
6. **No scheduled worker.** Scheduled messages need a durable PostgreSQL queue, UTC execution timestamps plus the author's IANA timezone for display, row-claiming/locking, retry policy, and a unique delivery key. A one-second schedule resolution is achievable; exact network delivery at a wall-clock second cannot be guaranteed.
7. **Private media needs authorization at every access path.** Random prefixed file IDs are identifiers, not permissions. Downloads, pack membership, sending, saving, revocation, and deletion must enforce ownership or an explicit share grant server-side.
8. **Poll privacy must be enforced in storage and API.** Anonymous polls must not expose voter identity in result payloads or logs. Quiz correctness should be revealed according to poll state/settings, not embedded in public vote responses.
9. **Existing profile PR #3 is stale/overlapping.** The current `main` already contains `ProfileActivity.java` and profile navigation. PR #3 is based on an older main revision and should not be merged as-is; its remaining changes must be checked for anything not already integrated.

## Recommended implementation order

### Phase 0 — Stabilize the foundation
- Split Android chat networking/UI into maintainable classes without changing behavior.
- Introduce typed API responses, bounded networking, explicit loading/error states, and per-chat send ordering.
- Add idempotency keys and a durable pending-send queue.
- Add backend integration tests for ownership boundaries and duplicate-send prevention.
- Verify Android and backend CI on every PR.

### Phase 1 — Core message interactions
- Message actions: copy, reply, forward, delete-for-self/delete-for-everyone, edit supported text/media, multi-select.
- Swipe-to-reply and quote navigation.
- Message reactions with server persistence, 3 distinct reactions per user/message (FIFO eviction), 10 emoji types per message, and live updates.
- Implement the selected-message overlay and reaction user list in Android.

### Phase 2 — Realtime and reliability
- Authenticated WebSocket endpoint and Android reconnecting client.
- Event sequence IDs, replay/resume after reconnect, heartbeat, backoff, and REST resync.
- Deliver message/reaction/poll changes only to chat members.
- Add push notification integration only after notification provider credentials and user preferences are designed. Respect mute settings and suppress redundant push while a chat is active.

### Phase 3 — Drafts, polls, and scheduling
- Per-user/per-chat cloud drafts with revision/update timestamps and conflict handling.
- Polls and quizzes: anonymous/non-anonymous, multiple choice, deadlines, vote changes, close/reopen rules, and aggregate results.
- Scheduled messages: durable queue, second-level scheduling input, timezone handling, edit/cancel, idempotent delivery, and worker observability.

### Phase 4 — Media, packs, and voice tools
- Secure upload/download pipeline and typed media metadata/file IDs.
- Private GIF/sticker library and invite-only packs with revocation and subscriber synchronization.
- Photo/video/document/audio/voice/video-note message rendering and reply previews.
- Voice/video-note playback speed and optional on-device Vosk model downloads/transcription, with integrity and license checks.

### Phase 5 — Groups, channels, folders, archive, and polish
- Group/channel data model, roles and permissions, member lifecycle.
- Pins, folders, archive behavior, notification settings, and cross-device preference sync.
- Load/performance tests and device QA for large histories and concurrent uploads.

## Safety and migration rules

- Never edit an already-applied migration; add a new numbered migration.
- Never run destructive database or Docker volume commands as part of feature work.
- Never overwrite `.env`, reverse-proxy configuration, firewall settings, or VPS-local modifications without explicit inspection and a separate plan.
- Deploy API changes only after CI passes and migration behavior is reviewed. Back up before any production schema operation.
- Keep private media behind authenticated authorization checks; do not treat an opaque file ID or unlisted URL as access control.
- Keep secrets out of source, logs, PR descriptions, and reports.

## First implementation target

The first code milestone should be **message reliability + message actions**, not the full media catalog. It directly addresses the current test feedback (long-press does nothing; copy/delete/forward are unavailable; consecutive sends are unreliable) and creates the message model and event contracts needed by reactions, replies, polls, and scheduled messages.
