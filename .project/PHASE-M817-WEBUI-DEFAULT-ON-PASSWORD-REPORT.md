# Phase M817 — Web UI default-on + password second factor

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner, running a bare
`agezt.exe`: "web ui nasıl çalışacak? 8787 port veriyor ne token ne bir şey biz
nasıl gireceğiz" + "web ui için parola koruma da lazım sadece token yetmez".

## Why

Two real friction points the owner hit opening the product cold:

1. **Web UI was OFF by default.** `buildWebUI` returned early unless
   `AGEZT_WEB_ADDR` was set, so a bare `agezt.exe` printed
   `web ui : disabled (set AGEZT_WEB_ADDR, e.g. 127.0.0.1:8787)` — the operator
   saw "8787" (just an example), opened it, found nothing, no token. The
   console — the product surface — required an env var to even start. This also
   contradicts the allow-by-default posture (you turn things OFF, not ON).
2. **Token-only auth.** The single credential was the URL token. The owner
   wanted a real password gate on top: "token alone isn't enough."

## What shipped

### Default-ON web UI
`buildWebUI` now defaults an empty `AGEZT_WEB_ADDR` to `127.0.0.1:8787`. Explicit
opt-OUT keywords (`off`/`disabled`/`none`/`no`/`0`/`false`) disable it. If the
default port is busy (a second daemon, another app), it falls back to an
OS-assigned free port on loopback so a bare `agezt` ALWAYS gets a console — the
banner prints the real tokenized URL either way.

### Password second factor (opt-in)
New `AGEZT_WEB_PASSWORD` (SECRET in the schema → vault; injected into the env at
boot). When set, the **token gets you the page but every DATA route also requires
a session cookie** minted by `POST /api/login` with the password — token AND
password, layered. Unset = token-only (pre-M817 behaviour), consistent with
allow-by-default.

- `kernel/webui/session.go`: in-memory session store (random 32-byte ids,
  12h sliding expiry, dies with the daemon), constant-time password compare,
  HttpOnly + SameSite=Strict + Secure-when-TLS cookie, and an online-guess
  lockout (8 fails → 5-min cooldown). Routes: `/api/authmeta` (probe),
  `/api/login`, `/api/logout` — all TOKEN-gated (the login itself needs the
  first factor, so the two compose: you can't even attempt the password without
  the token).
- `authorized()` refactor: token is the first factor (`tokenPresented`); when a
  password is set, a valid session cookie is required on top. The SPA shell `/`
  and the auth routes use a token-only wrapper so the lock screen can render
  before a session exists.
- Frontend `views/Login.tsx`: `AuthGate` probes `/api/authmeta` and, when a
  password is required and unauthed, renders the lock screen INSTEAD of the app
  (held in `main.tsx` ABOVE the data providers, so no EventSource/data call
  fires — and 401s — before login). A failed probe falls through to the app so
  the feature can never lock a user out of an otherwise-usable console.

## Tests

- **Go** (`session_test.go`): token-alone-is-not-enough (token→401, login→cookie,
  token+cookie→200, cookie-without-token→401), no-password→token-suffices,
  lockout after repeated failures (correct password then 429), logout revokes.
- **vitest** (`Login.test.tsx`, 5): AuthGate renders children when no password /
  when the probe fails; shows the lock screen (not the app) when required; reveals
  the app after a successful login; Login surfaces a wrong-password error.

## Smoke (isolated AGEZT_HOME, real daemon)

`AGEZT_WEB_ADDR=127.0.0.1:23940 AGEZT_WEB_PASSWORD=hunter2`: banner →
`web ui : http://…/?token=…  (password-protected)`. Curl: token-only
`/api/status` → 401; `/api/authmeta` → `{password_required:true,authed:false}`;
wrong password → 401; correct → 200 + cookie; token+cookie → 200. Playwright:
the page loads to a **Console locked** screen, entering `hunter2` → Unlock →
the full console reveals, 0 console errors. Bare `agezt.exe` (no env) →
`web ui : http://127.0.0.1:8787/?token=…`.

## Gate

Full `GOMAXPROCS=3 go test -p 2 ./...` green; vet + staticcheck clean; linux
cross-build OK; vitest 503/74 files; `kernel/webui/dist` rebuilt (committed LF
via .gitattributes); go.mod unchanged. One new AGEZT_* env var
(`AGEZT_WEB_PASSWORD`) — added to `configEnvVars` (guard) + the Config Center
"Interfaces" section.

## Notes / backlog

- Sessions are in-memory (restart = re-login) — deliberate for a credentialed
  surface; persistence wasn't asked for.
- Password is a single shared secret (no per-user accounts) — matches the
  single-operator model.
- Still owner-gated: CI billing → green badge → v1.0.0 tag.
