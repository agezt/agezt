# M364 — `agt why` walks the causation provenance chain (SPEC-01 §7.1)

## SPEC audit (read-vs-code)
SPEC-01 §7 defines the canonical `Event` with both `correlation_id` (field 8)
and `causation_id` (field 9). §7.1 names the headline explainability feature:

> **`causation_id` provenance graph** → `agt why <event>` walks the chain
> backwards: which observer → which salience score → which LLM call → which
> policy decision. Explainability for free.

**Verified gap (not assumed):**
- The implemented `Event` has `CausationID` and it **is populated** — the Pulse
  engine threads the originating `tickID` into every delta/salience/initiative/
  briefing (`kernel/pulse/engine.go`), and the outbound webhook dispatcher sets
  `CausationID: ev.ID` (`kernel/webhook/webhook.go`).
- But **`Kernel.Why` groups strictly by `correlation_id`** and no read path
  anywhere walks `causation_id`. The CLI even documents it as "list every event
  sharing an event's correlation chain". So `causation_id` was **recorded but
  never traversed** — the §7.1 promise was unfulfilled.
- The gap is observable: Pulse gives each delta/salience/initiative its **own**
  correlation and links back to the tick **only** via `causation_id`. So
  `agt why` on a Pulse initiative could **never reach the tick that caused it** —
  the one edge that connects them is the one `Why` ignored.

## What
Additive — `Why` (correlation grouping) is untouched; existing tests unchanged.
- **`kernel/runtime/runtime.go`** — new `(*Kernel).Causes(eventID)` walks
  `causation_id` from the target back to the root, returns the chain oldest-first
  (root → … → target). Cycle-guarded (seen-set; a forged journal loop must
  terminate), stops at the root (`causation_id == ""`) or a dangling parent.
- **`kernel/controlplane/server.go`** — `handleWhy` now also returns
  `causation_chain` (best-effort; only when the chain has > 1 event). Existing
  `events`/`correlation`/`parent_correlation` fields unchanged.
- **`cmd/agt/why.go`** — renders a "caused by (provenance, root first)" section
  when the chain reaches beyond the event itself; included in `--json`
  (`causation_chain`); help text updated.

## Verification
- **Kernel white-box** (`kernel/runtime/causation_internal_test.go`, 5 tests):
  `TestCauses_WalksCausationAcrossCorrelation` builds a Pulse-style
  tick→initiative under DIFFERENT correlations and asserts both halves — `Why`
  cannot see the tick (the gap), `Causes` reaches it root-first (the fix);
  plus deep-chain ordering, termination, dangling-parent→self, not-found error.
- **Control-plane** (`controlplane_test.go::TestWhy_CausationChainCrossesCorrelation`):
  the real `CmdWhy` RPC returns a `causation_chain` reaching the tick across the
  correlation boundary, while the correlation `events` list does not.
- **Live daemon demo** (mock provider, fresh home, dead-port `AGEZT_WEBHOOKS`):
  a run's `webhook.failed` delivery event journaled with `causation_id` =
  originating `task.received`. `agt why <failed-id>` rendered the new
  `caused by (provenance, root first): task.received → webhook.failed` section;
  `--json` carried `causation_chain` (len 2). End-to-end on the real CLI path.
- `gofmt -l` clean; `go vet` clean; `GOOS=linux go build ./...` exit 0. Full
  suite **2106** passing (was 2100; +6), `go test ./...` 0 failures.
  `go.mod`/`go.sum` unchanged. CHANGELOG (Added, user-visible `agt why` output).

## Scope notes
- The Pulse causation graph is currently a shallow star (everything → tick), so
  the live "deep observer→salience→LLM→policy" chain §7.1 illustrates is
  data-shallow today; `Causes` walks arbitrary depth, so a future deepening of
  the causation wiring surfaces automatically with no read-path change. Not
  over-claimed: the capability (traverse causation) is what was missing.
- A true causation cycle cannot be produced through `Bus` (causation_id must
  reference an already-published, older event), so the seen-set guard is purely
  defensive against a corrupt/hand-edited journal — documented honestly in the
  test rather than fabricating a cycle.
