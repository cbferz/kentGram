# KentGram 3.0 — Delivery Roadmap

## Phase 0 — Foundation
- [x] Create isolated kentgram-v3 branch from current main.
- [x] Record product vision and initial architecture.
- [ ] Inventory current backend, Android, Web, migrations, CI, and deployment.
- [ ] Review OpenGram and upstream mytelegram licensing and source provenance.
- [ ] Define acceptance criteria, threat model, and test/staging isolation.

## Phase 1 — Core platform
- [ ] Establish module boundaries and API v2 contracts.
- [ ] Add identity/session/device management and privacy primitives.
- [ ] Add durable realtime event delivery and cursor recovery.
- [ ] Add media upload sessions, image validation, thumbnails, and object storage.
- [ ] Add observability, backups, restore tests, and migration checks.

## Phase 2 — Messenger
- [ ] Direct chats, groups, channels, and forum topics.
- [ ] Replies, edits, deletes, forwarding, reactions, polls, pins, search.
- [ ] Multi-device synchronization, read receipts, typing, and presence.
- [ ] Android and Web feature parity for core chat workflows.

## Phase 3 — Social
- [ ] Profiles and follow graph.
- [ ] Posts, feed, likes, comments, bookmarks, and discovery.
- [ ] Stories, audience controls, expiry, and view lists.
- [ ] Reporting, moderation, and rate limits.

## Phase 4 — Advanced platform
- [ ] Bot API and developer documentation.
- [ ] One-to-one WebRTC calls.
- [ ] Group calls/live broadcasts after TURN/SFU capacity and security tests.
- [ ] Optional reviewed end-to-end encryption design and implementation.

## Phase 5 — Release
- [ ] Accessibility, performance, security, and abuse testing.
- [ ] Android signing compatibility and monotonic versionCode.
- [ ] Web browser/device compatibility and offline/update behavior.
- [ ] Data migration rehearsal and rollback rehearsal.
- [ ] Explicit production cutover approval.

## Release gates
No phase is complete from a UI mock alone. Each feature must include server authorization, persistence, tests, realtime behavior where applicable, Android and Web integration, and operational monitoring.
