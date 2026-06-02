# M150 — Channel-health check in `agt doctor`

## Why
M141 surfaced the configured channels in `agt status`, and deliberately rendered a
**half-configured** channel — one with a listen addr but no inbound secret/public
key — as `outbound-only`, exposing the silent misconfiguration. But `agt status` is
a glance, not a nag. The classic footgun is: an operator sets `AGEZT_SLACK_TOKEN` +
`AGEZT_SLACK_ADDR`, forgets `AGEZT_SLACK_SIGNING_SECRET`, and the Slack endpoint
comes up but rejects every event (fail-closed verify) — so the bot looks alive yet
never responds, with no error anywhere. `agt doctor` is the go-to diagnostic; it
should call this out persistently. This is the same status→doctor pairing M137 did
for network exposure.

## What
- **`cmd/agt/doctor.go`** — a new `checkChannels(status)` doctor check, wired into
  `runDoctorChecks` next to `checkExposure` (reusing the already-fetched status
  snapshot — no extra round-trip). It reads the M141 `channels` array and WARNs when
  any channel has `addr != "" && inbound == false` — a listening endpoint that can't
  accept anything — naming each and pointing at the fix (set the signing
  secret / public key, or unset the addr to run outbound-only). All-healthy is an OK
  (`N configured, M can receive commands`); no channels is an OK. An addr-less
  outbound-only channel is a deliberate choice and is NOT flagged.

## Files
- `cmd/agt/doctor.go` — `checkChannels`; wired into `runDoctorChecks`.
- `cmd/agt/doctor_test.go` — `TestCheckChannels`.

## Tests (+1, all passing)
- `TestCheckChannels` — a channel with an addr but `inbound:false` → WARN naming
  only it (a healthy inbound channel in the same snapshot is not named); all-healthy
  (inbound, plus an addr-less outbound-only) → OK; no channels → OK.

## Live proof (offline mock, real booted daemon)
Booted with `AGEZT_SLACK_TOKEN` + `AGEZT_SLACK_ADDR` + `AGEZT_SLACK_CHANNELS` but
**no** `AGEZT_SLACK_SIGNING_SECRET`:

```
$ agt doctor
  …
  [WARN] channels         : 1 channel(s) listening but inbound DISABLED: slack (127.0.0.1:8840)
           ↳ set the channel's signing secret / public key (AGEZT_SLACK_SIGNING_SECRET /
             AGEZT_DISCORD_PUBLIC_KEY) so inbound messages are accepted, or unset the addr
             to run outbound-only
```

## Verification
- `go.mod` / `go.sum` unchanged.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all touched files.
- `go test ./...` — **FAIL 0**, **1475 tests** (was 1474; +1), 61 packages.

## Result
A silently-dead inbound channel (addr set, secret/key missing) is now a persistent
WARN in `agt doctor`, naming the channel and the fix — closing the loop M141 opened,
so an operator can't be left wondering why their bot never answers.
