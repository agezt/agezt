# M365 — Lock in the Pulse delivery-routing matrix (SPEC-03 §4.3/§6.3)

## SPEC audit (read-vs-code)
SPEC-03 §4.3 (the Quiet/Balanced/Chatty dial) and §6.3 (anti-annoyance: quiet
hours, "only `alert` breaks them") together define how a scored delta becomes an
actual delivery. In code this is `pulse.Route(dial, disposition, quietHoursActive)
→ Delivery` — a pure function, the single point that decides **what reaches the
operator and what is allowed to break quiet hours**.

**Audited the surrounding pulse engine first — it is solid, not the gap:**
- Dispositions drop/digest/notify/alert + `act` exist; `act` is deliberately
  downgraded to `ask` (Initiative v1 inform-or-ask, §9) — a scope boundary, not
  a gap, so initiative rate-limiting (§8) is moot until autonomous `act` ships.
- Novelty suppression (§6.3 "never ping twice") is correctly wired: `MarkSeen`
  fires only on an actual delivery (DeliverNow/DeliverDigest); both drop paths
  (DispDrop, DeliverDrop) skip it, so a dropped item isn't wrongly suppressed.
- QuietHours.Active handles the midnight-wrap window (22→7) correctly;
  ParseQuietHours bounds-checks and fail-closes to disabled.

**The genuine gap:** `Route` itself — the safety-/correctness-critical decision
— had **no direct test**. It was exercised only incidentally by two engine
integration tests (`TestDialQuietSuppressesNotify`, one DialChatty run). A
regression (a digest leaking through quiet hours, the Quiet dial letting a
notify ping, alert failing to break through) would be a real
anti-annoyance/safety failure with no guard.

## What
Test-only, no production change. `kernel/pulse/route_test.go`:
- **`TestRoute_FullDispositionDialQuietHoursMatrix`** — the complete
  5 dispositions × 3 dials × {quiet on/off} = 30-row truth table.
- **`TestRoute_QuietHoursOnlyAlertAndActBreakThrough`** — asserts the headline
  §6.3 invariant directly (so it survives a table edit): during quiet hours
  `alert`/`act` MUST be DeliverNow and everything below MUST NOT be DeliverNow.

## Verification
- **Negative control (proves the matrix bites):** removing the quiet-hours hold
  clause in `Route` made the matrix FAIL on exactly the expected rows
  (`notify`/balanced+chatty and `digest`/chatty during quiet hours flip to
  `now`) and tripped the invariant test; restored → green, `salience.go`
  byte-identical to HEAD.
- `go test ./kernel/pulse -run TestRoute` — pass. `gofmt -l` clean; `go vet`
  clean; `GOOS=linux go build ./...` exit 0. Full suite **2108** passing (was
  2106; +2), `go test ./...` 0 failures. `go.mod`/`go.sum` unchanged. No
  CHANGELOG (test-only, no behaviour/output change).

## Scope notes
- SPEC-03 §8 "initiative rate-limiting / novelty gate before acting twice"
  guards autonomous `act`, which is intentionally not shipped (§9 MVP). Recorded
  as a deliberate non-feature, not stale — do not invent it.
- SPEC-03 now audited: heartbeat, observers, salience (severity rules + LLM
  refine + relevance boost), the dial+quiet-hours routing matrix (this), novelty
  suppression, digest batching, quiet hours. The full provenance promise (§8,
  tick→delta→salience→initiative→briefing via `agt why`) became walkable in M364.
