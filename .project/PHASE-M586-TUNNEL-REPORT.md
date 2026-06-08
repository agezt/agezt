# PHASE M586 — Public tunnels (`kernel/tunnel`)

**Status:** DONE — local, gated (unit + full daemon smoke green), ready for
branch/PR. **Owner picked Tunnels via AskUserQuestion.**

## What shipped

`kernel/tunnel` — supervises a tunnel binary to expose a local HTTP service (Web UI,
else REST) to the public internet, wrapping the providers' rendezvous servers rather
than building a relay (keeps the one-dependency promise; `os/exec` + stdlib only):
- **Presets:** `AGEZT_TUNNEL=cloudflared` → `cloudflared tunnel --url <target>`;
  `AGEZT_TUNNEL=ngrok` → `ngrok http <host:port> --log=stdout --log-format=logfmt`.
  `AGEZT_TUNNEL_CMD="<any command>"` overrides for any other binary.
- **URL discovery:** scans the binary's merged stdout+stderr; `extractURL` prefers a
  known tunnel domain (trycloudflare/cfargotunnel/ngrok*) so a provider's banner/docs
  link isn't mistaken for the tunnel, falling back to the first `https://` for custom
  binaries. The URL is printed to the daemon log via an `OnURL` callback.
- **Supervision:** restarts the binary with capped exponential backoff (1s→30s,
  reset after a healthy 30s run) if it exits unexpectedly; tears the whole process
  group down on ctx cancel (`proc_unix.go` Setpgid + negative-pid SIGKILL;
  `proc_windows.go` direct-child kill — mirrors the plugin host).
- **Operator authority:** nothing is exposed unless the operator sets a tunnel; the
  startup line + the per-URL log line both state the service is now public.

## Wiring

- `buildTunnel(ctx, stdout)` in `cmd/agezt/main.go`: reads `AGEZT_TUNNEL` /
  `AGEZT_TUNNEL_CMD` / `AGEZT_TUNNEL_TARGET`, derives the target from the Web UI addr
  (else REST), starts the supervisor, surfaces a status line. Registered right after
  the Web UI block (so the target addr is known). `addrToURL` turns `:port` /
  `host:port` into a loopback URL.
- `kernel/controlplane/config.go`: 3 `AGEZT_TUNNEL*` env vars (alphabetical).

## Tests + smoke (all green)

- **8 unit tests** (`tunnel_test.go`): `buildCommand` (cloudflared/ngrok presets,
  scheme-stripping, explicit-command-wins, missing-target / unknown-provider /
  no-config errors); `extractURL` (cloudflared box, ngrok logfmt, prefer-tunnel-domain
  over docs link, custom first-https + punctuation trim, http-ignored, none);
  `stripScheme`; `New` errors + `Name`; **supervisor URL capture**; **restart-on-exit**
  (backoff injected via `afterFunc` so no real sleep); **concurrent URL access**
  (race guard). staticcheck + go vet clean; race deferred to CI (no local CGO/gcc —
  the documented offline exception; tests use mutex/atomic/channel sync).
- **Full daemon smoke:** booted the daemon with a mock tunnel binary
  (`AGEZT_TUNNEL_CMD`) that prints a cloudflared-style `trycloudflare.com` URL → the
  status line showed `custom command → exposing http://127.0.0.1:18788`, the daemon
  logged `tunnel public URL: https://mock-tunnel-demo.trycloudflare.com`, and on
  `agt shutdown` the mock process was **cleanly torn down** (not running). Proves
  config → spawn → URL-parse → surface → teardown end to end.
- gofmt clean on staged LF blobs; full Go suite green; 6× cross-build (incl. both
  `proc_unix.go` and `proc_windows.go`) verified; `go.mod` unchanged.

## Scope / follow-ups (documented)

- The real public-URL path needs the actual `cloudflared`/`ngrok` binary + network —
  env-bound, like the channels' real-service smoke; the supervisor/parse/lifecycle is
  fully covered offline with a mock binary.
- Live URL in `agt status` (control-plane surface) is a follow-up; v1 surfaces it via
  the daemon log + startup line. No journal event kind added (kept the package
  decoupled from event/bus).

## Backlog after M586

Remaining DEFERRED: SDK publish (PyPI/npm/crates.io — registry secrets, owner-gated);
ambient/voice (hardware/STT). `agt migrate` = no real migration → skip.
