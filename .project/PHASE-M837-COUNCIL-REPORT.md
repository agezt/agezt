# Phase M837 — Council of Elders (engine + `council` tool)

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "bir tane ihtiyar
heyeti kur — birden çok provider ve model seçilmiş 3 agent aralarından konsensus
kurarak bir konuyu tartışabilir ve çözüme kavuşturur … herhangi bir durumda bu
heyete başvurulup buradan cevap alınabilir." Milestone 1 (backend); the Web UI
Council view is M838.

## What shipped

A multi-model deliberation primitive — a panel of advisors, each on a DIFFERENT
keyed provider/model, that debate a question and converge to a consensus.

**Engine (`kernel/runtime/council.go`)** — mirrors the M821 vision sidecar
(`DescribeImages`): each member is a one-shot Governor `Complete` routed to a
specific model via per-request model routing; the council orchestrates the
rounds. `Kernel.Council(ctx, corr, question, members, rounds)`:
1. **Round 0** — each member gives an independent opening position (run
   CONCURRENTLY; provider calls are slow).
2. **Deliberation** (default 1 round) — each member sees the peers' latest
   positions and refines or dissents.
3. **Consensus** — the chair (first member) synthesizes the final positions into
   a decisive answer; a `CONSENSUS:/DISSENT:` split records genuine disagreement.
- Resilient: a member call that errors becomes an opinion-with-error, not a
  council failure; only an empty panel / total wipe-out errors. Token-bounded per
  turn. Journals `council.convened` / `council.opinion` / `council.consensus`
  (new event kinds) — the audit trail "ne oldu, kim ne dedi".

**Default membership** — `catalog.BestModelsAcross(eligible, 3)` returns one
best-context model per **keyed** provider (distinct providers), assembled by the
daemon (`cfg.CouncilMembers`, built like `VisionModel` with the
registered+credentialed predicate — so it NEVER picks an unkeyed model).
`AGEZT_COUNCIL_MEMBERS` (comma model list) overrides; added to controlplane
`configEnvVars`.

**Tool (`plugins/tools/council`)** — `council {question, rounds?}` → returns
`{consensus, dissent, opinions[]}`; decoupled via a `Runner` interface, kernel
injected by the daemon. `kernel/edict/toolmap.go`: `case "council": return
CapDelegate` (multi-model consultation, same axis as delegate — no new grant).

## Verification

- **Unit:** engine (2 members × opening + deliberation = 4 opinions, both models
  spoke, consensus parsed, events journaled; default membership used when members
  nil; empty council / empty question error; `splitConsensusDissent` cases);
  `catalog.BestModelsAcross` (one best model per provider, sorted, capped,
  ineligible skipped); tool over a fake runner; toolmap case.
- **Live** (isolated home, 3 keyed providers via .env): an agent `council` call
  on "SQLite vs JSON files for a single-binary Go app" convened **3 distinct
  models** (deepseek-chat, gemini-2.0-flash, k2p5), 2 rounds → **6**
  `council.opinion` + 1 `convened` + 1 `consensus`; the consensus correctly
  **recorded Elder Gamma's dissent** (JSON-files position).

## Gate

runtime + catalog + council-tool + edict + event tests green; vet + staticcheck +
linux cross-build clean; gofmt swept; config guard green. go.mod unchanged.
Default-allow: council on by default; per-run cost cap + daily ceiling bound the
N model calls.

## Next

M838 — Council Web UI: `CmdCouncilAsk` + `CmdCouncilMembers` control-plane
commands, `/api/council/*` routes, and a `Council.tsx` view (seat→model editor
from the keyed catalog + ask box rendering opinions + consensus live).
