# Web client checks

`mock-server.mjs` is a **test-only** stand-in for the Go API (same JSON
contracts, same status codes, same `DisallowUnknownFields` strictness). It
exists so the browser client can be exercised when the Go server cannot be
built. It is not the backend and says nothing about it - run
`go test ./...` with `TEST_DATABASE_URL` for that.

```
node web/test/e2e.mjs                 # needs Playwright + Chromium
SHOTS=/tmp/shots node web/test/e2e.mjs   # also writes screenshots
```

The suite drives two real browser sessions (login, search, chat, realtime
delivery, read receipts, typing, reply, reactions, edit, pin, forward, delete,
offline retry, WebSocket drop and resync, token-refresh storm), checks layout
for horizontal overflow and composer visibility from 320px to 2560px, and
checks the service worker (precache, versioned cache, offline shell).
