# Phase Report — Milestone M67 (`agt tool stats` — tool-invocation aggregate)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 observability.

## Why

M66 added `agt tool log` (per-call audit). The aggregate view was still missing:
"across all tool calls, what's the error rate, and which tools fail?" M67 adds
`agt tool stats`, the **execution-dashboard analogue of `agt edict stats`**, and
completes the tool **list / log / stats** triad — exactly mirroring edict's
**show / log / stats** (definitions → events → aggregate).

## What shipped

- **`handleToolStats` (`kernel/controlplane/tool_log.go`)** — folds the journal's
  `tool.result` events into `total`, `errored`, `error_rate`, and a per-tool
  breakdown (`by_tool: {tool → {calls, errors}}`). Optional `tool` name filter;
  `since_ms` window via the shared `sinceCutoff` helper. Tenant-scoped via
  `kernelFor`.
- **CLI `agt tool stats [--tool <name>] [--since <dur>] [--tenant <id>]
  [--json]`** — renders the totals + error rate + a deterministic (name-sorted)
  by-tool table.
- **Tenant-allowlisted** — `CmdToolStats` added to `tenantTokenAllows` so a
  tenant can read its own tool-execution health.

## Design decisions

- **Fold on `tool.result`.** Same anchor event as `tool log` — every call
  (including a policy-denied one) emits exactly one result, so the per-tool call
  counts are exact.
- **Reuse the edict-stats shape.** `{total, errored, error_rate, by_tool, …}`
  parallels `{total, denied, denial_rate, denied_by_capability}`; the CLI render
  reuses `intOfStatus` and the name-sorted breakdown idiom, so the two dashboards
  read the same way.

## Tests

- `TestToolStats_Aggregates` — total/errored/error_rate over a mixed set, the
  per-tool calls/errors breakdown, and a `--tool` filter scoping the aggregate.

Test count: **1308 → 1309**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt tool stats
  tool invocations (over 1):
    errored   : 0
    error     : 0.0%
    by tool:
      shell            1 call(s), 0 error(s)
$ agt tool stats --json
  { "total": 1, "errored": 0, "error_rate": 0,
    "by_tool": { "shell": { "calls": 1, "errors": 0 } }, "tools": 1, "window_ms": 0 }
```
