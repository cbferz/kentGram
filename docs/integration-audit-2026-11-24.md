# KentGram 3.0 integration audit

Date: 2026-11-24

## Integration baseline

This branch is based on `main`, not on `feat/kentgram-complete-redesign`.

Reason: the GitHub branch `feat/kentgram-complete-redesign` currently resolves to commit `92d57fbfb3664feaae11776f14d4b555401586fd`, an older starter foundation. Its README describes the app as a starter shell. The current `main` README documents the implemented Android/Web/Go/PostgreSQL messenger.

At audit time, comparison against `feat/kentgram-complete-redesign` reported:
- `main`: 225 commits ahead and 24 commits behind.
- `feat/kentgram-2-messaging-core`: 5 commits ahead and 248 commits behind `main`.

## Existing functionality on main

Verified in repository files:
- Authenticated WebSocket route and bounded realtime hub.
- Durable realtime event replay endpoint `GET /api/v1/realtime/events`.
- PostgreSQL-backed reactions, message editing, forwarding, pins, polls and communities.
- Chat media upload/download handlers.
- Android client includes realtime, typing/presence, reactions, forwarding, polls and pin-related UI code.
- Migrations `0009_realtime_events.sql` and `0010_message_edit_reactions.sql` are present.

## Decision on PR #25

Do not merge PR #25 as-is. It is based on a substantially older code line and introduces overlapping realtime, API, store and migration implementations. Its custom WebSocket auth/frame protocol must not replace the current ticket/authenticated realtime design without a demonstrated missing requirement and compatibility tests.

The existing PR's test notes explicitly state that HTTP handlers and Android MainActivity were not fully built/type-checked in that branch. Its reported test results are not a substitute for validating the current main branch.

## Production safety

No production host, database, secrets, deployment configuration or running service was changed by this integration audit. No production deployment or migration was performed.

## Validation still required

- Run current backend Go tests and PostgreSQL integration tests from this branch.
- Run Android Gradle build and verify applicationId/signing continuity/versionCode.
- Run Web syntax/container/Nginx checks.
- Compare the actual production VPS working tree and applied migrations with current main before any release. The GitHub branch name alone is not proof that it matches the live VPS.

## Release rule

This branch is an integration/audit branch only. Merge to main and production deployment remain separate decisions after CI and production-drift review.
