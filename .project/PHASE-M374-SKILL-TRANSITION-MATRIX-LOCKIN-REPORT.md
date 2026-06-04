# M374 — Lock in the skill lifecycle transition matrix (SPEC-05 §5.2)

## SPEC audit (read-vs-code)
SPEC-05 §5.2 defines the skill lifecycle state machine — the governed pipeline a
self-authored / Forge-proposed skill walks before it can reach `active`
(production): `draft → shadow → active`, with `shadow`/`active → quarantined`
(regression), `quarantined → active` (un-quarantine), any non-archived state →
`archived` (operator cleanup), and `archived` terminal.

**Verified vs `kernel/skill`:** fully implemented and correct — `Status` (all 5
states incl. `shadow`), `legalTransitions`, `CanTransition`, `PromoteTarget`.
This is NOT a feature gap.

**The gap (test coverage, priority A):** `TestLegalTransitions` spot-checked only
**7 of the 25** from×to pairs, and nothing tested the consistency between
`PromoteTarget` and `legalTransitions`. This state machine is correctness-/
safety-critical — it is the gate that decides whether a possibly self-authored
skill reaches production. A regression in an unchecked edge (e.g. `draft→active`
skipping the shadow-test gate, or `archived` becoming non-terminal) could
silently ship a bad skill, and a divergence between the promote ladder and the
legal edges would let `promote()` drive a skill into an illegal state.

## What
Test-only, no production change. `kernel/skill/transitions_matrix_test.go`:
- **`TestCanTransition_FullMatrix`** — the complete 5×5 from×to matrix, with the
  expected legal set written from the SPEC diagram **independently** of the
  implementation's map (so a spec divergence is caught, not rubber-stamped);
  plus explicit invariants: self-transitions illegal, `archived` terminal, every
  non-archived state can be archived, `draft→active` forbidden (shadow gate),
  and an unknown status fails closed.
- **`TestPromoteTarget_ConsistentWithLegalTransitions`** — every target
  `PromoteTarget` returns must be a legal `CanTransition` edge (promote can never
  produce an illegal state); and `draft` promotes to `shadow`, never straight to
  `active`.

## Verification
- **Negative control (proves the matrix bites):** adding an illegal `draft→active`
  edge to `legalTransitions` made the matrix FAIL on exactly that edge and the
  shadow-gate assertion; restored `skill.go` byte-identical (git diff empty) →
  green.
- `gofmt -l` clean; `go vet` clean; `GOOS=linux go build ./...` exit 0. Full
  suite **2134** passing (was 2132; +2), `go test ./...` 0 failures.
  `go.mod`/`go.sum` unchanged. No CHANGELOG (test-only).

## Scope notes
- SPEC-05 audited solid: memory types (5 canonical FACT/SUMMARY/RELATION/
  PREFERENCE/OBSERVATION; SKILL_REF intentionally lives in the skill store, not
  memory), distillation (M344), world-model Relate (M345), FileStore concurrency
  (M363), skill lifecycle (this) + Forge transitions + active-only retrieval.
- Honestly deferred (large, recorded not closed): §5.1/§5.2 **auto** shadow-
  testing and auto-quarantine (v1 is the manual gate, documented in the Status
  consts); §8 hard-prune for "right to forget" (tombstone exists; a true
  journal-level content prune is a nuanced compliance feature). Embeddings/
  semantic-vector tier (§1/§9) is the plugin-backed RAG layer, out of the
  memory-lite core.
