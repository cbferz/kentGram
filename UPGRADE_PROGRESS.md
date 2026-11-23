# KentGram 2.0 — upgrade progress

Updated: 2026-11-23

## Merged to main

- PR #20 merged: 8ff5cd7000f75f1ea9f442d172623809703b7fc7.
- PR #21 merged: fd67b79bdcec0120951b48a72277d2256160f024.
- PR #22 merged: 0f7d0c172bf2ba1b01b15bfdd62b1bcdf40131bf (Web community creation/join/member management, profile editing and avatar upload).
- PR #19 and PR #18 closed as superseded by the consolidated implementation.
- Includes communities, chat media, polls, forwarding, pins, read receipts, message replies, edit, emoji reactions and Web/Android UI.
- Message edit supports Android PUT and Web PATCH.
- Reactions are PostgreSQL-backed, chat-scoped and limited to three distinct reactions per user per message.
- Durable event log records direct chat creation, message send/forward/delete/edit, reactions, pin/unpin, read receipts and poll create/vote/close.
- Authenticated GET /api/v1/realtime/events?since=<cursor> provides cursor-based recovery.
- Web stores per-account cursors, performs periodic catch-up and supports poll creation/voting/closing.
- Web community management and profile/avatar editing are now merged; community member lists, roles and ownership are driven by authenticated Backend routes.
- Android stores a cursor, drains 100-event pages after reconnect and refreshes expired access tokens before abandoning realtime.
- Signed PR #21 preview APK: https://github.com/cbferz/kentGram/releases/download/kentgram-redesign-pr21/kentgram-debug.apk

## Verification

- PR #20 CI passed: Backend, Android signed APK and Web checks.
- PR #21 CI passed: Go vet/tests/build, Android build/signature/publication, Web syntax/container/Nginx checks.
- PR #22 CI passed: Backend tests/vet/build, Web syntax/container/Nginx checks, Android build/signature/publication. API paths and request/response shapes were reviewed before merge.
- PR #23 CI is currently blocked before execution: repeated Backend, Android and Web jobs report `runner_id=0`, an empty runner name and no steps. The jobs did not reach source checkout, tests or builds; do not interpret these as code-level test failures or merge until CI can execute successfully.
- Post-merge main Web and Android workflows passed. Main Backend workflow is still completing its post-merge integration test run; re-check before deployment.
- Main Android workflow produced a signed APK. Package/applicationId and signing key were preserved.
- Migration 0009 (durable realtime events) and migration 0010 (message edit/reactions) are additive; CI applies them on a clean PostgreSQL database.

## Production and data safety

- Production has not been deployed or modified.
- No production database migration has been run.
- No production backup has been created because no deployment/migration was attempted.
- No production rollback test has been performed.
- PR #20 post-merge Android workflow initially encountered Maven Central HTTP 429; rerun succeeded.

## Remaining work

1. Re-check the post-merge main Backend workflow; fix and rerun if it fails.
2. Finish and validate PR #23: durable realtime events for community create/join/add/remove/leave/role/ownership changes, including delivery to users removed from a community.
3. Improve Web community-management UX by replacing prompt/confirm flows with accessible dialogs and proper validation.
4. Add end-to-end tests for Web/Android cross-client scenarios, including reconnect and poll voting.
5. Verify migrations 0009/0010 against a backup copy of an existing database before any staging or production migration.
6. Prepare staging deployment, health/WebSocket/auth/message smoke tests and a verified rollback procedure.
7. Do not deploy to production until staging checks and backup/rollback verification pass.
