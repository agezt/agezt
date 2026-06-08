# PHASE M584 — Signal channel (11th), via signal-cli-rest-api

**Status:** DONE — local, gated (unit + full outbound & inbound daemon smoke green),
ready for branch/PR. **Owner picked Signal via AskUserQuestion** over Marketplace /
Tunnels / SDK-publish.

## What shipped

`plugins/channels/signal` — an in-process duplex Channel (SPEC-04 §1) talking to an
operator-run [signal-cli-rest-api](https://github.com/bbernhard/signal-cli-rest-api)
over `net/http` only (no dependency). Mirrors the Matrix channel template:
- **Inbound:** long-poll `GET /v1/receive/{number}?timeout=N`, drain envelopes;
  `dispatchable` = non-empty `dataMessage.message`, source set, source ≠ own number.
  Allowlist by sender (`channel.Allowlist`), `channel.Guard` → handler → reply.
  Non-allowlisted senders get one "not authorized" notice, fail-closed.
- **Outbound:** `POST /v2/send` `{message, number, recipients:[id]}`, chunked via
  `channel.SplitText` (2000 chars). Used by replies, `agt send`, and Pulse briefs.
- **Auth/SSRF:** API URL operator-pinned → no SSRF (same model as the HA tool).
  signal-cli-rest-api is unauthenticated; an optional bearer `Token` covers a
  fronting reverse proxy, and is scrubbed from error strings.
- **Poll-rate floor (`signalMinPollInterval = 1s`):** a healthy server blocks for
  `?timeout=`, but if one returns empty immediately we sleep the remainder so we
  never busy-spin/hammer the local API. No-op when it blocks or when there's data.

## Wiring

- `buildSignal` in `cmd/agezt/main.go` (`AGEZT_SIGNAL_API_URL` + `_NUMBER` required;
  `_RECIPIENTS` allowlist, `_TOKEN`, `_POLL_SECS` optional). Imported as
  `signalchan` (the bare `signal` name collides with `os/signal`).
- Registered after Teams: `go sgChan.Start(ctx)` + status line; `sgSink` added to the
  Pulse `combineSinks(...)`; `liveChannels["signal"]` for `agt send`; `collectChannels`
  entry for `agt status`.
- `kernel/controlplane/config.go`: 5 `AGEZT_SIGNAL_*` env vars (alphabetical).
- `agt send` help refreshed to list all 11 channels (also added `teams`, which the
  M574 help line had missed).

## Tests + smoke (all green)

- **6 unit tests** (`signal_test.go`, httptest mock mirroring matrix_test.go):
  allowed-sender drives agent + reply POSTed with the bot number, recipient, and
  bearer token, inbound+outbound journaled; non-allowlisted refused (no token → no
  Authorization header); `dispatchable` (self-skip, empty, no-source); `receive`
  parses + forwards `?timeout`; empty-noop + 2-chunk split; non-2xx → error.
- **Outbound daemon smoke:** real daemon + mock API; `agt send --channel signal
  --to +1555… "hello"` → mock got `POST /v2/send {message, number:+1555000…,
  recipients:[+1555111…]}`; status line `listening, allowlist=1 number(s)`.
- **Inbound daemon smoke:** mock served one message from an allowlisted sender; the
  daemon polled `/v1/receive`, drove the (DEMO_ECHO) agent, and POSTed the echo reply
  back to the sender. Verified the poll-rate floor: receive calls dropped from ~40/3s
  to ~5/3s after the fix.
- `gofmt` clean on staged LF blobs; full Go suite green (81 pkgs); `go build ./...`
  clean; `go.mod` unchanged (no new dependency).

## Notes

- `signal` vs `os/signal` import collision → aliased `signalchan`.
- gofmt flagged `signal_test.go` (struct-comment alignment) on the staged blob; fixed
  with `gofmt -w`. The working-tree `gofmt -l` also flagged unchanged files — a CRLF
  artifact; the staged-blob check (CI's gate) is authoritative and was clean.

## Backlog after M584

Remaining DEFERRED items still need an owner steer (external services / secrets):
Tunnels, Marketplace, SDK publish (PyPI/npm/crates.io). `agt migrate` = no real
migration → skip.
