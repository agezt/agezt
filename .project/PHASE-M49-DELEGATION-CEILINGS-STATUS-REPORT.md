# Phase Report — Milestone M49 (Surface the delegation ceilings in `agt status`)

> Status: **shipped** · Date: 2026-06-01
> SPEC-12 (multi-agent orchestration). Ninth step on the multi-agent axis —
> a *legibility* increment: the M46–M48 governance triad was invisible until a
> delegation tripped a cap; M49 makes it queryable at a glance.

## Why

M46–M48 gave delegation a complete governance triad — count (fan-out), cost
(attribution), and cost cap. But every ceiling was **silent**: nothing told an
operator what governance was active. The only way to learn a cap existed was to
trip it and read the refusal in the journal. For a feature whose whole purpose is
to bound autonomous, recursive, money-spending behaviour, "you find out when it
fires" is the wrong default. M49 surfaces the active depth / fan-out / spend caps
in `agt status` — the operator's standard health round-trip.

## What shipped

- **`Kernel.SubAgentLimits()` (`kernel/runtime/runtime.go`)** — returns a small
  `SubAgentLimits` struct (Enabled, MaxDepth, MaxFanout, MaxSpendMicrocents). It
  reports the *effective* depth (defaulting to 1 when the delegate tool is on and
  the cap is unset, exactly as `runSubAgent` does), so status never disagrees with
  enforcement. Read-only.
- **`handleStatus` (`kernel/controlplane/status.go`)** — adds a `delegation`
  object to the status response: `{enabled, max_depth, max_fanout,
  max_spend_microcents}` (0 fan-out / spend = unbounded).
- **`cmdStatus` (`cmd/agt/status.go`)** — renders one line:
  `delegation: depth≤1, fan-out ≤3, spend ≤$0.5000` (or `fan-out unbounded` /
  `spend unbounded` for an unset cap, or `delegation: off` when the tool is
  disabled). Reuses the `agt budget` `fmtUSD` formatter so spend reads identically
  across surfaces.

## Design decisions

- **Report the effective cap, not the raw config.** `SubAgentMaxDepth` defaults to
  1 at enforcement time when unset; `SubAgentLimits` applies the same defaulting so
  the operator sees `depth≤1`, the value actually enforced — not a misleading
  `depth≤0`. Fan-out / spend have no such default (0 genuinely means unbounded), so
  they're reported raw and rendered as "unbounded".
- **A nested object, not four flat fields.** Grouping the delegation ceilings under
  one `delegation` key keeps the status result readable and lets the CLI render (or
  omit) the block as a unit. The four scalars stay jq-friendly.
- **Status, not a new command or a boot banner (yet).** `agt status` is the
  operator's existing "what's my daemon doing" round-trip — the natural home, and
  already wired into CI smoke tests. A boot-time banner echoing the caps is a
  reasonable follow-up but would duplicate this surface; M49 keeps to one queryable
  place.
- **`off` is a first-class state.** When the delegate tool is disabled
  (`AGEZT_SUBAGENT=off`), status says `delegation: off` rather than showing
  meaningless caps — the operator sees delegation is unavailable, not just
  unbounded.

## Tests

- `kernel/controlplane/status_test.go::TestStatus_DelegationCeilings` — delegate
  tool on, fan-out 3, spend $0.50, depth unset → status reports `enabled=true`,
  `max_depth=1` (effective default), `max_fanout=3`, `max_spend_microcents=5e8`.
- `TestStatus_ReturnsExpectedShape` augmented to assert the `delegation` block is
  present and `enabled=false` by default (the rig doesn't enable the tool).
- `startPair` refactored into `startPairWithConfig(t, cfg)` so a control-plane test
  can drive a kernel with specific config; `startPair` now delegates to it.

Test count: **1274 → 1275**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, added lines gofmt-clean.

## Live proof (offline daemon)

```
# caps set
$ AGEZT_SUBAGENT_FANOUT=3 AGEZT_SUBAGENT_SPEND_CAP=0.50 agezt &
$ agt status
  delegation: depth≤1, fan-out ≤3, spend ≤$0.5000

# defaults (delegate tool on, no caps)
$ agt status
  delegation: depth≤1, fan-out unbounded, spend unbounded
```

The governance an operator configured (or didn't) is now visible without tripping
a cap.

## What's next

The multi-agent axis is now observable (M41–M45), governed (M46–M48), AND legible
(M49). Sharpest remaining frontiers:

1. **Per-run spend in `agt runs list` / `show`** (LOW) — `runEntry.SpentMicrocents`
   (M47) is already computed; surface it per row and on the M44 delegation `↳`
   outcome line, so spend is visible per-run, not only in aggregate. Pure client
   rendering over data already on the wire.
2. **Journal the run answer** (MED) — `llm.response`/`task.completed` carry
   `text_chars`/`usage`, not the body; adding it lights up the M44 outcome and the
   "final answer:" arc section.
3. **Tenant-scoped `why`** (LOW-MED) — route `handleWhy` via `kernelFor` + a
   tenant-token allowlist; the last non-tenant-aware control surface.
4. **Boot-banner the caps** (LOW) — echo the active delegation ceilings at daemon
   startup (next to the model advisory / recovery banners), so they're visible in
   the daemon log too, not only on demand.
