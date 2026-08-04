# AGEZT Deep Research Harness — Implementation Plan

> Date: 2026-07-02
> Related: [`JARVIS-VISION-2026.md`](JARVIS-VISION-2026.md) §6 P2-8 (the biggest "beyond parity" opportunity)
> Location: `kernel/research/` (new) + `plugins/tools/research/` (new) + workflow template + view

## Why this, and why now

Two axes separate a Jarvis from a chat agent: **governed proactivity** and **deep
reasoning**. On the first (Pulse+Initiative) AGEZT is ahead; on the second there is a gap:
there is no multi-source, contradiction-verified **research engine** in the code. OpenClaw and
Hermes are weak here too (web search only). That makes this both a move that pushes past
parity and one competitors cannot easily answer.

**Key insight: this is not a from-scratch job — it is a composition of existing, mature primitives.**

| Research step | Primitive already in AGEZT | File |
|---|---|---|
| Question → sub-questions / plan | `planner` (LLM→DAG) | `kernel/planner/planner.go` |
| URL discovery | `websearch` (keyless DDG, SSRF-protected) | `plugins/tools/websearch/websearch.go` |
| Fetch page + convert to text | `browser.read` / `browser.action` | `plugins/tools/browser/` |
| Multi-source triangulation (consensus) | **`council`** (different models → agreement) | `plugins/tools/council/` + `kernel/runtime` |
| Contradiction/adversarial verification | **`conductor`** (Thinker/Worker/**Verifier**) | `plugins/tools/conductor/` + `kernel/runtime` |
| Orchestration | `workflow` DAG engine | `kernel/workflow/` |
| Persistence (findings/entities) | `memory` + `worldmodel` | `kernel/memory/`, `kernel/worldmodel/` |
| Audit + citation chain | hash-chained journal + `why` | `kernel/journal/`, `cmd/agt/why.go` |
| Governance (budget/policy/HITL) | governor + edict + approvals | `kernel/governor/`, `kernel/edict/` |

Everything competing harnesses (e.g. DeerFlow) rebuild already exists in AGEZT;
**all we need is a thin layer that unifies these under a research contract.**

## Architecture

`research` lives as a new kernel package + agent-facing tool + workflow template.
It makes no LLM calls of its own; it invokes existing tools through the governed `RunTool`.

```
research.Run(question, opts)
  │
  ├─ 1. PLAN     planner → sub-questions + research DAG (loop/gate nodes)
  │
  ├─ 2. GATHER   for each sub-question (fan-out, parallel workflow node):
  │                websearch(query) → candidate URLs
  │                browser.read(url) → source text (+ untrusted marker preserved)
  │                → Source{url, title, text, fetched_at, hash}
  │
  ├─ 3. EXTRACT  extract claims from each source → Claim{text, source_id, confidence}
  │
  ├─ 4. VERIFY   for each significant claim, conductor (Verifier role):
  │                "refute this claim; reject if uncertain" → CONFIRMED | REFUTED
  │                conflicting sources → council → consensus + minority-opinion note
  │
  ├─ 5. SYNTH    cited synthesis from CONFIRMED claims only (every sentence → source_id)
  │
  └─ 6. PERSIST  findings → memory; entities/relations → worldmodel; every step → journal
                  → ReportArtifact{markdown, sources[], claims[], confidence}
```

### Governance boundary (the moat — competitors lack it)
- Every `websearch`/`browser.read` call passes through Edict policy and is written to the journal.
- Source text preserves the `ObservationUntrusted` marker → the prompt-injection guard stays active.
- Budget: a ceiling similar to `budget.total`; `governor` circuit breaker; step/source/depth caps.
- A sentence that cannot be cited **must not enter** the synthesis (citation requirement, hallucination brake).
- The result artifact + full source list + `why <event>` make it traceable end to end.

## Data types (`kernel/research/research.go`)

```go
type Source struct {
    ID        string    // ulid
    URL       string
    Title     string
    Text      string    // browser.read output (untrusted)
    Hash      string    // BLAKE3(text) — change detection on re-fetch
    FetchedAt time.Time
    Rank      int        // websearch ordering
}

type Claim struct {
    ID        string
    Text      string
    SourceIDs []string   // supporting sources
    Verdict   string     // "unverified" | "confirmed" | "refuted" | "contested"
    Note      string     // council minority opinion / verifier rationale
}

type Report struct {
    Question   string
    SubQuestions []string
    Sources    []Source
    Claims     []Claim
    Markdown   string     // cited synthesis
    Confidence float64
    Budget     BudgetUse  // token/step/source counters
}
```

## Phases

### Phase 1 — Core harness (MVP, ~1 week)
- `kernel/research/` package: `Run()` = plan → gather → extract → synth (VERIFY still simple).
- `plugins/tools/research/` agent-facing tool (the `research` verb); register it in `main.go`
  (same `out[...]` pattern as websearch/browser, around line ~7615).
- Edict `CapResearch` axis (websearch-like, low-risk read + fan-out cap).
- `agt research "<question>" [--depth N] [--max-sources M] [--json]` CLI + `agt why` compatibility.
- Tests: mock provider + fixed fixture pages (no network), fan-out cap, citation requirement.

### Phase 2 — Adversarial verification (the real differentiator, ~1 week)
- Wire the VERIFY step to `conductor`: try to refute every high-impact claim with the Verifier role.
- Send conflicting sources to `council` → consensus + minority note (`Claim.Note`).
- Populate the `Verdict` field; derive `confidence` from the confirmed/total ratio.
- Synthesis uses only `confirmed`/`contested(agreed)` claims.

### Phase 3 — Orchestration + persistence (~1 week)
- Workflow template `research.v1` (`kernel/workflow/templates`): fan-out gather nodes,
  gate node = "are there enough reliable sources?", if not loop with new sub-questions (loop node).
- Write findings to `memory` (subject dedupe, per-agent private-by-default), entities/relations to
  `worldmodel`; report → `artifact` store.
- Pulse integration: periodic research via standing order / schedule ("scan topic X every morning").

### Phase 4 — Console surface (~3-4 days)
- New view `Research.tsx` (67→68): question box, live step stream (SSE), source cards
  (confidence badge + verdict chip), cited report, `why` deep link. Reuses the existing design
  system (PageHeader + glass + ModelChip) and the `events.tsx` SSE hub.
- `views/Research.test.tsx` (in line with the ~1:1 per-view test discipline).

## Acceptance criteria
- One question → a report triangulated from ≥3 independent sources, with every sentence cited.
- At least one false/conflicting claim is caught as `refuted`/`contested` in the VERIFY step
  (proof that adversarial verification works; tested with a fixture).
- All research respects the budget ceiling; on overrun it stops cleanly rather than crashing.
- Every source fetch + every claim verdict is in the journal; `agt why <report_event>` shows the full chain.
- Source text preserves the untrusted marker; if the injection guard fires, a note is added to the report.

## Non-goals (boundary)
- Turning the planner into a mid-execution recursive re-planner — preserve the existing
  "one LLM call = static DAG" contract; loop via the workflow loop node, not a meta-agent.
- Producing uncited sentences (the hallucination-brake contract).
- Treating sources as trusted — all are untrusted, and verdicts come with evidence.
- Adding a separate LLM client — all model calls go through the existing governor/provider path.

## Next up (after this plan)
The same "compose existing primitives" approach also applies to the P0 gaps:
- **Device/companion**: tray/PWA on top of node registry + tunnel + approvals API (little new code).
- **Live browser tab**: promote the `browser.action` driver to a persistent process + stale-ref handling.
- **LLM curator**: existing deterministic curator + `council` shadow-mode consolidation.
