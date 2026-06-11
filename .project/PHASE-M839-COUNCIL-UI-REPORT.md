# Phase M839 — Council of Elders Web UI

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "bizim bunu da
webui da" — surface the Council of Elders (M837) in the console. Milestone 2 (UI)
of the council arc.

## What shipped

A **Council** view: the operator's seat at the table.

**Control plane (`kernel/controlplane/council.go` + protocol/server):**
- `council_members` — the default membership the panel will convene with (one
  seat per keyed provider), for display. Backed by a new
  `Kernel.CouncilDefaultMembers()`.
- `council_ask {question, rounds}` — convenes the panel synchronously (a fresh
  correlation) and returns `{consensus, dissent, members, rounds, opinions}`.

**Web UI routes:** `/api/council/members` (GET, read allowlist) and
`/api/council/ask` (POST JSON, within the 120 s jsonProxy budget — rounds run
concurrently so wall-clock is a few model-call latencies).

**Frontend (`frontend/src/views/Council.tsx`, nav "Council"):** shows the seated
models, a question box (⌘/Ctrl+Enter to convene) with a rounds control, a
"deliberating…" state, then the **consensus** prominently (with dissent boxed in
warn colour) above the full **transcript** — opinions grouped by round (opening
positions / deliberation rounds), each card showing seat + model, rendered
markdown.

**Parser hardening:** `splitConsensusDissent` is now markdown-aware — it matches a
`CONSENSUS:` / `## CONSENSUS` / `**Dissent**` style header by stripping leading
markdown markers, since real chair models emit headers, not bare labels.

## Verification

- **Unit:** webui read-only guard extended for `council_members`; split-parser
  cases added for markdown headers; runtime + controlplane + webui Go tests green;
  frontend `tsc` + 512 vitest green.
- **Live HTTP** (isolated home, 3 keyed providers, daemon :8799):
  `GET /api/council/members` → `Elder Alpha=deepseek-chat`,
  `Beta=gemini-2.0-flash`, `Gamma=k2p5`; `POST /api/council/ask` ("tabs or spaces
  for Go?") → 6 opinions from the 3 models + a consensus.

## Gate

runtime + controlplane + webui Go tests green; frontend tsc + vitest green; vet +
staticcheck + linux clean; dist rebuilt & committed (LF, in sync). go.mod
unchanged.

## Council arc

Both milestones done: M837 (engine + `council` tool, agent-facing) + M839 (Web UI,
operator-facing). The owner's "ihtiyar heyeti — webui da" is complete.
