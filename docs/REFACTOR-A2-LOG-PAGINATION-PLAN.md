# Refactor A2 — per-endpoint pagination for the log endpoints

> Companion to `docs/REFACTORING-SCAN.md` finding **A2**.
> **Generated:** 2026-07-03 (post-merge of PR #467). Grounded in a live handler + route-map scan.

## Evidence (measured — narrows the scope)

Scan said "~12 log endpoints still stream full slices and bypass readArgsRoutes." Verified reality:
**11 `handle*Log` list endpoints**, all with `seq`+`ts` fields, all currently no-cursor — but **6 are
already on `readArgsRoutes`** (from the merged pagination work) with `limit` + filters. The gap is
narrower and uniform.

| Endpoint | File | On readArgsRoutes? | cursor in allowlist? | Gap |
|---|---|---|---|---|
| /api/tool_log | tool_log.go | yes (limit,tool,errors) | no | add cursor |
| /api/provider_log | provider_log.go | yes (limit,fallbacks) | no | add cursor |
| /api/policy_log | policy_log.go (CmdEdictLog) | yes (limit,denied) | no | add cursor |
| /api/approvals_log | approvals_log.go | yes (limit,denied) | no | add cursor |
| /api/plan_history | plan_history.go | yes (limit,status) | no | add cursor |
| /api/schedule/fires | schedule_fires.go | yes (limit,id,status,since_ms,intent) | no | add cursor |
| /api/ratelimit_log | ratelimit_log.go | no | — | register + cursor |
| /api/webhook_log | webhook_log.go | no | — | register + cursor |
| /api/world_log | world_log.go | no (/api/world is separate) | — | register + cursor |
| /api/netguard_log | netguard_log.go | no | — | register + cursor |
| /api/warden_log | warden_log.go | no | — | register + cursor |
| /api/memory_log | memory_log.go | no (/api/memory is separate) | — | register + cursor |

**Out of scope (verified):** all `handle*Stats` handlers (aggregates, not lists); `edict_overlay.go`
(`handleEdictCompact`/`handleEdictOverlay` — no seq/ts, it's a compaction overlay, not a log).

**Uniform cursor:** every log endpoint walks the journal; every entry has `seq`+`ts`. So **all 11 use
the same `ms:seq` cursor** that A1 Phase 1 extracts into `kernel/journal/cursor.go` — one cursor,
applied 11 times, not per-endpoint design. Handler shape confirmed at tool_log.go:28 (reads `limit`
+ filters, walks journal tail); adding cursor is identical to the /api/runs change in commit dbff54ad.

## Dependency on A1

Depends on `kernel/journal/cursor.go` (A1 Phase 1). Land it once:
- Preferred: A1 P1 first, then all 11 reuse `journal.Cursor`.
- Standalone: if A2 runs first, create `kernel/journal/cursor.go` in A2 Phase 0; A1 P1 reuses it.
Either way the helper lives in `kernel/journal`, never duplicated per file.

## Plan

- **P0 shared cursor helper (once):** `kernel/journal/cursor.go` — `EncodeCursor(tsMS,seq)`,
  `DecodeCursor(s)`, `FilterBeforeCursor(entries,cursor)`. Table tests (round-trip, unparseable
  fallback; port the cursor-direction bug test the /api/runs work caught). Gate: `kernel/journal`.
- **P1 the 6 already-registered endpoints (cursor-only):** per endpoint = (1) handler parses `cursor`
  → `journal.DecodeCursor` → `FilterBeforeCursor` before `limit` → emit `next_cursor`; (2) add
  `"cursor"` to the endpoint's allowlist in webui.go; (3) one pagination test. Order: tool_log →
  provider_log → policy_log → approvals_log → plan_history → schedule/fires.
- **P2 the 6 not-yet-registered endpoints (register + cursor):** ratelimit_log, webhook_log,
  world_log, netguard_log, warden_log, memory_log — same handler change, plus MOVE the route from
  `apiRoutes` to `readArgsRoutes` with `["limit","cursor",<filters>]` (the /api/runs migration ×6).
- **P3 frontend pagers:** add a wrapper per endpoint in `lib/cursorPager.ts` (merged in 1cf4bb01) —
  `useToolLogPager` etc. — plus load-more footers in the consuming views. **Converges with scan C3**
  (extend useCursorPager to all list views); do together per view.

## Sequencing

```
P0 journal/cursor.go (shared — or reuse A1 P1)
P1 6 registered endpoints   ← cursor-only, mechanical, 1 commit each
P2 6 unregistered endpoints ← register + cursor (the /api/runs migration ×6)
P3 frontend pagers          ← converges with C3, per-view
```

## Interactions

- **A1:** A1 P1 and A2 P0 both want `kernel/journal/cursor.go` — land once, other reuses. A2's handler
  changes are disjoint from A1's runs.go/roster.go moves.
- **C3:** A2 P3 IS the log-endpoint slice of C3. Pull them together per view.

## Effort / risk

Lowest-risk, most-mechanical of the refactoring plans. P1 = 6 × (3-line Go + 1 allowlist word + 1
test). P2 = 6 × (same + route-map move). P3 = 11 tiny frontend wrappers + footers. No new cursor
design. Per-endpoint gate: `go build ./... && go test ./kernel/{controlplane,webui,journal}/...`;
frontend phase adds `tsc + vitest + npm run build`.
