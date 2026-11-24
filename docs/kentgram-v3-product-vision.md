# KentGram 3.0 — Product Vision

## Goal

Build an independent, self-hosted social messenger combining real-time communication, community spaces, and creator-style publishing in Android and Web. Learn from OpenGram, Telegram, and Instagram capabilities, but use KentGram's own identity, interface, APIs, and implementation.

## Product pillars

### Messenger
- Direct conversations, groups, supergroups, channels, and forum topics.
- Replies, forwarding, editing, deletion, reactions, polls, pinned and scheduled messages.
- Typing state, presence, read receipts, multi-device synchronization, and message search.
- Photos, video, albums, documents, voice messages, and resumable uploads.
- Privacy controls, block/report flows, session management, and optional two-step verification.

### Social
- User profiles and follow relationships.
- Home feed with posts, photo/video carousels, captions, likes, comments, and bookmarks.
- Stories with expiry, audience controls, and view lists.
- Public profiles, discovery, hashtags, and moderation controls.

### Calls and live media
- One-to-one voice/video calls using WebRTC.
- Group calls and live broadcasts only after signaling, TURN, SFU capacity, and abuse controls are tested.
- Do not claim end-to-end encryption until independently reviewed.

### Platform
- Versioned public API and documented Bot API.
- Admin console with scoped roles, audit log, reports, and service health.
- Object storage with retention and quota policies.
- Android and Web clients sharing account, privacy, and synchronization models.

## Experience and visual identity
- A distinct KentGram design system using a restrained Liquid Glass visual language.
- Translucent surfaces, layered depth, subtle blur, controlled highlights, and motion with reduced-motion support.
- Strong readability and accessibility in light and dark themes.
- Native-feeling Android navigation and responsive Web layouts.
- Graceful fallbacks where backdrop blur is unavailable or expensive.

## Non-goals
- Do not copy Telegram or Instagram branding, proprietary assets, source code, or exact screen layouts.
- Do not promise Telegram network or MTProto-client compatibility in the first release.
- Do not invent cryptography. Use established, reviewed protocols and libraries.
- Do not replace production until migration and rollback are proven.

## Delivery principles
1. Preserve the current KentGram repository and production deployment.
2. Build KentGram 3.0 on an isolated branch and isolated test environment.
3. Deliver vertical slices including API, database migration, Android, Web, and tests.
4. Keep migrations forward-only and tested against a disposable database.
5. Require explicit approval before production deployment or destructive data migration.
