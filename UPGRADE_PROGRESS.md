# KentGram 2.0 — upgrade progress

Updated: 2026-11-23

## Merged

- PR #20 merged to main: 8ff5cd7000f75f1ea9f442d172623809703b7fc7.
- Includes the KentGram 2.0 feature branch: communities, chat media, polls, forwarding, pins, read receipts, Web/Android messaging UI, message edit and emoji reactions.
- Message edit supports both Android PUT and Web PATCH.
- Reaction storage is PostgreSQL-backed, chat-scoped, and enforces a maximum of three distinct reactions per user per message.
- PR #19 was closed as superseded by PR #20.
- PR #20 CI passed: Backend checks, Android signed APK build, Web syntax/container/Nginx checks.
- Signed preview APK: https://github.com/cbferz/kentGram/releases/download/kentgram-redesign-pr20/kentgram-debug.apk

## Current work — PR #21

- Branch: feat/durable-realtime-recovery
- PR: https://github.com/cbferz/kentGram/pull/21
- Latest known commit: 549ce27c34e93e5429524f1f9a81ef56fa4fa62e
- Adds migration 0009_realtime_events.sql, durable event counter/log, and authenticated GET /api/v1/realtime/events?since=<cursor>.
- Event log is written transactionally for direct chat creation, send/forward/delete/edit, reactions, pin/unpin, read receipts, and poll create/vote/close.
- Web persists a per-account cursor and catches up periodically; Web also has poll create/vote/close controls.
- Android persists its cursor, drains 100-event pages after reconnect, and refreshes access credentials when realtime/API catch-up receives 401.
- Integration test covers event ordering, cursor advancement, message/poll replay and membership isolation.
- Latest Web checks passed. Backend and Android checks are still running/queued for the latest commit; re-check before merge.

## Other branch state

- PR #18 (feat/realtime-sync-foundation) remains open but is no longer directly mergeable after PR #20. PR #21 ports its durable event-log core using a REST cursor-recovery path; the older ticket/WebSocket implementation has not been merged wholesale.
- Do not merge PR #18 blindly; compare any remaining auth/ticket/listener changes against PR #21 first.

## Production and data safety

- Production has not been deployed or modified.
- No production database migration has been run.
- Migration 0009 is additive; still take and verify a database backup before any production migration.
- No production rollback test has been performed.
- PR #20 post-merge Android workflow initially hit Maven Central HTTP 429; rerun succeeded. No source-code failure remained.

## Next steps

1. Re-check latest PR #21 Backend, Android and Web workflow results; fix any failures and rerun.
2. Verify PR #21 migration and integration tests on clean CI PostgreSQL.
3. Merge PR #21 only after all required checks pass.
4. Review/close PR #18 only after confirming no remaining useful auth/ticket functionality is lost.
5. Continue missing Web parity: communities/groups management, profile editing/avatar UX, settings and broader media workflows.
6. Add durable realtime events for membership/community changes and complete end-to-end scenarios.
7. Only then prepare a staging migration/deploy with a verified backup and rollback plan; production remains untouched until explicitly safe.
