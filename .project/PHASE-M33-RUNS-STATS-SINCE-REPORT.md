# Phase Report — Milestone M33 (Windowed run stats: `--since`)

> Status: **shipped** · Date: 2026-05-31
> SPEC-08 (operability/observability). Sixth step on the resilience/observability
> axis (M28 → … → M33). M29 gave all-time run health; M33 lets the operator ask
> the same question over a recent window.

## Why

`agt runs stats` (M29) aggregates the *entire* journal. That's the right default
for "what's the lifetime success rate", but an operator triaging a live incident
asks a different question: "how have runs done in the **last hour**?" An all-time
rate is dominated by ancient history and can hide a fresh spike of failures or
timeouts. Now that runs carry first-class `failed` (M30), `timeout` (M31), and
`canceled` (M32) terminal states, a windowed rate is genuinely informative.

## What shipped

- **`CmdRunsStats` optional `since_ms` arg + `window_ms` echo
  (`kernel/controlplane/runs.go`)** — when `since_ms > 0`, the handler computes
  `cutoff = now − since_ms` against the **server clock** (the same clock that
  stamps every event's `TSUnixMS`, so the comparison is apples-to-apples) and
  folds only runs whose `StartedUnixMS >= cutoff`. Runs with no recorded start
  (the completed-without-received edge) can't be placed on the timeline, so they
  are excluded from a windowed view. `window_ms` (0 = all-time) is echoed for
  transparency. All-time behaviour is byte-for-byte unchanged when `since_ms` is
  absent.
- **`agt runs stats --since <dur>` (`cmd/agt/runs.go`)** — parses both
  `--since 1h` and `--since=1h` forms with `time.ParseDuration`; a malformed or
  non-positive duration is a usage error (exit 2). Sends `since_ms` only when
  set. The header gains a `, last <dur>` suffix; an empty window renders `no runs
  in the last <dur>` instead of the all-time "no runs yet".

## Design decisions

- **Filter on START time, not end time.** "Runs in the last hour" means runs that
  *began* in the window — a long run that started 2h ago and finished 5m ago is
  not a "last-hour run". Filtering on `StartedUnixMS` keeps the window intuitive
  and stable (a run never changes which window it belongs to as it progresses).
- **Server-side cutoff.** The CLI sends a *duration*, not a wall-clock cutoff, and
  the daemon resolves `now` itself. This avoids client/daemon clock skew: the
  cutoff and the event timestamps come from the same clock.
- **Excluded runs are excluded silently but consistently.** A run with no start
  time is dropped from a windowed view (it can't be dated); in the all-time view
  it still appears, exactly as before. The window only ever *narrows*.
- **`window_ms` in the payload.** A jq consumer can tell a windowed result from an
  all-time one without re-deriving it from the request.

## Tests

`kernel/controlplane/runs_test.go` — `TestRunsStats_SinceWindow`: a just-published
completed run is counted all-time (`window_ms=0`) and under a 1-hour window
(`window_ms=3600000`); after a 60 ms sleep a 30 ms window excludes it
(`total=0`). The all-time M29 tests are unchanged, proving the default path is
untouched.

Test count: **1225 → 1226**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, all additions gofmt-clean (the CRLF artifacts `gofmt -l` flags in
`runs.go`/`runs_test.go`/`cmd/agt/runs.go` are pre-existing and untouched).

## Live proof (mock provider)

```
$ agt runs stats
run stats (over 3 run(s)):
  completed : 1 …

$ agt runs stats --since 1h
run stats (over 3 run(s), last 1h0m0s):

$ sleep 3 && agt runs stats --since 2s
no runs in the last 2s

$ agt runs stats --since=1h --json | grep -E 'window_ms|total'
  "total": 3,
  "window_ms": 3600000

$ agt runs stats --since blah
agt runs stats: --since: want a positive Go duration (e.g. 90s, 1h), got "blah"
  → exit 2
```

Three runs counted all-time and inside `--since 1h`, then aged out under
`--since 2s` once they were older than the window — the windowed fold, header,
`window_ms` echo, and arg validation all confirmed end-to-end.

## What's next

The resilience/observability axis still has clean follow-ons:

1. **Per-tool timeout** (MED) — finer than M31's per-run cap: wrap
   `tool.Invoke(ctx, …)` with a per-call deadline in `kernel/agent/agent.go` so
   one slow tool fails its single call (error fed back to the model) without
   killing the whole run.
2. **Cancel-on-disconnect** (MED) — tie an `agt run` client connection to its run
   so Ctrl-C cancels server-side (reuses M32's `CancelRun`).
3. **`agt runs list --since <dur>`** (LOW) — mirror M33's window on the list view
   for symmetry (share the cutoff helper).
