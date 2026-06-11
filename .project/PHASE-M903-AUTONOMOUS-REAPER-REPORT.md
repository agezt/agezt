# Phase M903 — Autonomous Reaper (#53)

## Goal
Surface dead agents and stale artifacts **autonomously**, so the operator learns the
pile is growing without having to ask. The destructive half of #53 — actually
retiring an agent to the graveyard (M846) or collecting an artifact (M845) — stays
operator-gated by design (default-allow posture governs capability, not irreversible
cleanup). M903 ships the **detection** surface in three layers: a read-only kernel
scan, an autonomous pulse observer, and an on-demand control-plane/UI query.

## What shipped

### 1. Kernel scan — `kernel/runtime/reaper.go`
`Kernel.ReaperScan(agentIdleCutoffMs, artifactStaleCutoffMs int64) ReaperReport`,
read-only:
- **Dead agents:** walks the journal for `task.received` events (M854), mapping each
  agent slug → its latest activity timestamp via `payload["agent"]`. For every roster
  profile it flags those that are **enabled, not retired, created before the cutoff
  (grace for brand-new agents), and last-active before the cutoff** (or never active).
- **Stale artifacts:** `ArtifactIndex().StaleEntries(cutoff)` → count + summed bytes.
- `ReaperReport.Empty()` is true when nothing is flagged.

### 2. Autonomous observer — `kernel/pulse/reaper.go`
`ReaperObserver` implements the pulse `Observer` interface (`Name()`/`Poll`). It is
**transition-based**: a baseline beat records counts silently, then it fires exactly
one `SevLow` delta **only when the dead-agent or stale-artifact count grows** above the
previous beat. Stable counts are silent (no repeat spam); shrinking counts (operator
cleaned up) are silent. A `nil` scan func no-ops. Wired in `cmd/agezt/main.go` inside
the existing pulse block with a fixed 30-day window:
```go
eng.AddObserver(pulse.NewReaperObserver(func() (int, int) {
    cut := time.Now().Add(-reaperWindow).UnixMilli()
    r := k.ReaperScan(cut, cut)
    return len(r.DeadAgents), r.StaleArtifacts
}))
```

### 3. On-demand query — control plane + UI route
- `protocol.go`: `CmdReaperScan = "reaper_scan"`; `server.go` dispatch case.
- `kernel/controlplane/reaper.go`: `handleReaperScan` reads `idle_days`/`stale_days`
  (default 30, floored at 1 via `intArg`), computes cutoffs, returns
  `{dead_agents[], dead_count, stale_artifacts, stale_bytes, idle_days, stale_days}`.
- `kernel/webui/webui.go`: `/api/reaper/scan` registered in `readArgsRoutes`
  (allowlisted args `idle_days`, `stale_days`) — GET, read-only, like `agent_activity`.

## Tests
- `TestReaperScan_FlagsIdleFiltersRetiredPausedAndNew` — three agents (live/retired/
  paused); a future cutoff flags only the live one; a past cutoff flags none. PASS.
- `TestReaperObserver_FiresOnGrowthOnly` — baseline silent → growth fires one SevLow
  delta naming both counts → stable silent → shrink silent. PASS.
- `TestReaperObserver_NilScanNoOp`. PASS.

## Gate
- `go build ./...` ✓ · `go vet` (runtime/pulse/controlplane/webui) ✓
- targeted tests runtime/pulse/controlplane/webui ✓
- linux/amd64 cross-build ✓
- gofmt clean on new files (webui.go/main.go flagged only by CRLF working-copy artifact;
  git stores LF; `git diff --stat` shows only the intended additions)
- go.mod unchanged · no new `AGEZT_*` env var (fixed 30-day const) → configEnvVars guard
  not implicated

## Notes
Detection-only by intent. The pulse brief is the autonomous notification surface; the
`/api/reaper/scan` route is the operator's drill-down. Retire/collect remain explicit
operator actions.
