# KentGram 3.0 — Architecture Proposal

## Architecture decision

Start with a modular monolith, not a fleet of microservices. Keep deployment practical for the existing VPS and split services only when measured load or independent scaling requires it.

## Proposed stack
- Backend: Go, preserving existing team expertise.
- Primary database: PostgreSQL with explicit, versioned migrations.
- Realtime: authenticated WebSocket gateway with durable event cursors and per-account/device isolation.
- Cache and ephemeral coordination: Redis for measured needs such as presence, rate limits, and fan-out.
- Media: S3-compatible object storage (MinIO or managed S3), metadata in PostgreSQL, signed or membership-checked downloads.
- Android: native Android client; preserve applicationId com.kentgram and existing signing identity. Kotlin/Compose can be evaluated for new UI modules without changing package identity.
- Web: TypeScript application with shared design tokens and API client.
- Calls: WebRTC with a separately operated TURN service; group SFU is a later phase.
- Operations: Docker Compose for isolated test/staging, health checks, structured logs, metrics, backups, and restore drills.

## Bounded contexts
1. Identity: registration, login, sessions, device trust, two-step verification.
2. Profiles: profile data, avatars, privacy, block lists.
3. Messaging: chats, membership, messages, reactions, polls, pins, forwarding.
4. Media: upload sessions, validation, transcoding, thumbnails, storage lifecycle.
5. Social: follows, feed ranking, posts, comments, likes, bookmarks.
6. Stories: audience rules, expiry, views, media lifecycle.
7. Realtime: durable event log, cursor recovery, online state, typing, receipts.
8. Calls: signaling, call sessions, TURN credentials, SFU integration.
9. Bots: scoped tokens, webhook delivery, rate limits, Bot API.
10. Administration: operators, permissions, audit events, reports, moderation.

Each context owns its application logic and database access boundaries. Shared tables must not become an excuse for cross-module queries without explicit contracts.

## API contracts
- Keep /api/v1 stable during transition.
- Design new endpoints under /api/v2.
- Use REST for commands and queries where appropriate; WebSocket for realtime delivery.
- Every realtime event includes event ID, resource ID, actor ID, timestamp, and schema version.
- Scope user-owned records by authenticated principal and resource membership.
- Never accept owner IDs from untrusted request bodies as authorization evidence.
- Uploads use bounded sizes, MIME sniffing, content decoding, quotas, and malware scanning where available.

## Security and privacy
- TLS everywhere; short-lived access tokens and rotating refresh sessions.
- Rate limits on authentication, messaging, media, search, and bot endpoints.
- Object-level authorization on every read and write.
- Secret values stay outside Git and build logs.
- Use a reviewed end-to-end encryption protocol/library if E2EE is introduced; do not design custom cryptography.
- Add account export and deletion workflows with documented retention behavior.
- Audit administrator actions and scope permissions by role.

## OpenGram research boundary
The OpenGram repository describes a C#/.NET 9 MTProto server with multiple services and infrastructure dependencies. Its README says it is based on mytelegram, but a top-level LICENSE file was not found during the initial GitHub check. Do not copy, vendor, or adapt OpenGram source until the full license chain, upstream obligations, and compatibility requirements are verified.

For KentGram 3.0, treat OpenGram as a capability and architecture reference. MTProto compatibility is out of scope for the first milestone. A later decision record may evaluate an isolated protocol gateway if licensing and product requirements justify it.

## Migration and deployment
- Existing production database and containers remain untouched during development.
- Create a separate test database, media bucket/directory, network, and test hostname.
- Write repeatable backups before any data migration.
- Make import tooling idempotent and dry-runnable with counts and integrity checks.
- Keep old and new systems available in parallel until acceptance tests pass.
- Require a documented rollback plan and explicit approval before cutover.
