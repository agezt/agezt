# Phase M818 — http / browser.read default-ALLOW (owner law)

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "browser.read ve
http.get için sürekli `http: host not in allowlist` … her şey yasak bu ne" — the
network tools refused every host out of the box.

## Why

The `http` and `browser.read` tools shipped **default-DENY**: an empty host
allowlist meant *no host allowed*, and the daemon only flipped `AllowAll` on with
`AGEZT_HTTP_ALLOW_ALL=1` / `AGEZT_ALLOW_ALL=1`. So a fresh daemon answered every
`http.get`/`browser.read` with `host not in allowlist`. That is the exact
opposite of the owner's standing law (memory `default-allow-posture`): *every
capability is allowed by default; restriction is the opt-OUT*. The banner even
read `http(hosts=0, egress=guarded)` — zero hosts, everything blocked.

## What changed (`cmd/agezt/main.go` tool wiring only)

The tool LIBRARY defaults stay safe (`httptool.New()` is still default-deny — a
good library default); the DAEMON now opts into the open posture:

- **http:** empty `AGEZT_HTTP_ALLOWED_HOSTS` ⇒ `AllowAll = true` (any public
  host). A non-empty list is the opt-OUT that RESTRICTS to those hosts.
- **browser.read:** identical rule with `AGEZT_BROWSER_ALLOWED_HOSTS`.
- `AGEZT_ALLOW_ALL=1` / the per-tool `*_ALLOW_ALL=1` still force open and now also
  override a pinned allowlist back to open.

**The SSRF egress guard is untouched and stays the hard floor**: even with any
host allowed, netguard still refuses loopback (127.0.0.0/8, ::1), the private
network (RFC1918 / ULA), and cloud metadata (169.254.169.254) on every hop —
relaxed only by the explicit `AGEZT_HTTP_ALLOW_LOOPBACK` / `_ALLOW_PRIVATE`
flags. "Open" means the public internet, not a pivot into co-located admin
surfaces. This is the F4/SSRF carve-out the owner law preserves.

Banner now reads `http(any host, egress=guarded), browser.read(any host)`; a
pinned allowlist shows `http(hosts=N, …)`.

## Tests / verification

- Full `GOMAXPROCS=3 go test -p 2 ./...` green; vet + staticcheck clean. The
  httptool/browser/netguard unit suites (AllowAll + egress-guard behaviour) are
  unchanged and pass — no test pinned the old default-deny daemon banner.
- **Live smoke** (isolated home, real deepseek provider): banner shows
  `http(any host, egress=guarded)`. An agent run — "GET https://example.com and
  report the title" — returned **"Example Domain … No blocking occurred — 200
  OK"** (`$0.0068`). Pre-M818 this failed with `host not in allowlist`.

## Gate

Go suite + vet + staticcheck clean; linux cross-build unaffected; no schema /
env-var additions (behaviour change only — the same `AGEZT_*_ALLOWED_HOSTS`
vars now mean "restrict" instead of "the only thing allowed"); go.mod unchanged.

## Note

Existing operators who had RELIED on default-deny + an allowlist are unaffected:
a pinned `AGEZT_HTTP_ALLOWED_HOSTS` still restricts exactly as before. Only the
*empty* (unconfigured) case flipped from deny-all to allow-all, matching the
owner's allow-by-default posture.
