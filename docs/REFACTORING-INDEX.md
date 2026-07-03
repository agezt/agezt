# Refactoring — Index & Dependency Graph

Entry point for the AGEZT refactoring effort. Every plan below is **evidence-grounded** (measured
against real files — sizes, method/importer counts, route registrations — not estimated) and
**read-only planning** (no source changed yet). Start with the master scan, then follow the
dependency graph to sequence implementation.

## Documents

| Doc | Finding(s) | Scope | Risk |
|---|---|---|---|
| [REFACTORING-SCAN.md](REFACTORING-SCAN.md) | all 30 | master scan + 15-step sequencing | — |
| [REFACTOR-A1-CONTROLPLANE-PLAN.md](REFACTOR-A1-CONTROLPLANE-PLAN.md) | A1 | 190-file controlplane → domain packages | med |
| [REFACTOR-A2-LOG-PAGINATION-PLAN.md](REFACTOR-A2-LOG-PAGINATION-PLAN.md) | A2 | c​ursor pagination for 11 log endpoints | low |
| [REFACTOR-A3-B5-AUTH-HTTPSERVER-PLAN.md](REFACTOR-A3-B5-AUTH-HTTPSERVER-PLAN.md) | A3+B5 | `kernel/auth` (domain) + `kernel/httpserver` (transport) | high |
| [REFACTOR-A3-HTTPSERVER-PLAN.md](REFACTOR-A3-HTTPSERVER-PLAN.md) | A3 | *superseded by A3+B5 for auth phases*; transport detail still valid | — |
| [REFACTOR-C2-LIB-CLASSIFICATION.md](REFACTOR-C2-LIB-CLASSIFICATION.md) | C2 | 68-module `lib/` keep/colocate/merge | low |
| [REFACTOR-C4-CHAT-DECOMPOSITION.md](REFACTOR-C4-CHAT-DECOMPOSITION.md) | C4 | 1,973-line `Chat.tsx` decomposition | med |

> **Note:** `REFACTOR-A3-HTTPSERVER-PLAN.md` is kept for its transport-layer detail but is
> **superseded** by `REFACTOR-A3-B5-AUTH-HTTPSERVER-PLAN.md` for anything auth-related. Use the
> reconciled doc as the authority.

## Dependency graph

```
                    ┌─────────────────────────────┐
                    │ A1 Phase 1                  │
                    │ extract runs.go fold →      │
                    │ kernel/journal/cursor.go    │  ← shared ms:seq cursor helper
                    └──────────────┬──────────────┘
                                   │ creates the cursor helper
                    ┌──────────────┴──────────────┐
                    ▼                              ▼
        ┌───────────────────┐          ┌───────────────────────┐
        │ A1 Phases 2–6     │          │ A2 (11 log endpoints) │  reuse journal.Cursor
        │ board/memory/     │          │ P1 6 cursor-only      │
        │ workboard/roster  │          │ P2 6 register+cursor  │
        └─────────┬─────────┘          │ P3 frontend pagers ───┼──┐
                  │ A1 P6 = route split│                       │  │ converges
                  │ (needs Router)     └───────────────────────┘  │
                  ▼                                                ▼
        ┌───────────────────────────────┐              ┌────────────────────┐
        │ A3+B5 auth/httpserver         │              │ C3 (scan) extend   │
        │ P0 kernel/auth (domain)       │              │ useCursorPager to  │
        │ P1 kernel/httpserver (Router) │◄─── A3 P5     │ all list views     │
        │ P2 openaiapi  P3 restapi      │     (webui)   └────────────────────┘
        │ P4 tenant+oauth  P5 webui ────┼── MUST precede A1 P6
        │ P6 agentgw  P7 token-file     │
        └───────────────────────────────┘

        Independent (no cross-deps):
        ┌───────────────────┐   ┌────────────────────────────┐
        │ C2 lib/ cleanup   │   │ C4 Chat.tsx decomposition  │
        │ P1 voice merge ───┼───┤ P5 useChatSession (useVoice)│  sequence, don't overlap
        └───────────────────┘   └────────────────────────────┘
```

## Hard ordering constraints

1. **`kernel/journal/cursor.go` lands once** — A1 Phase 1 and A2 Phase 0 both need it. Whichever
   runs first creates it; the other reuses. Never duplicate.
2. **A3+B5 Phase 5 (webui) BEFORE A1 Phase 6 (route split)** — A1's per-domain route registrars call
   `httpserver.Router.AddRoute`, so the shared Router must exist first.
3. **A2 Phase 3 = C3's log-endpoint slice** — do the frontend pagers together per view, not twice.
4. **C2 Phase 1 (voice merge) vs C4 Phase 5 (useVoice)** — both touch the voice concern; sequence
   them, don't run concurrently.
5. **A1 and A3+B5 are disjoint in controlplane** — A1 moves domain folds (runs/roster/board/memory/
   workboard); A3+B5 moves auth (tenant.go, *_oauth.go). Different file sets → parallelizable if coordinated.

## Suggested global order

```
1. A1 P1               → creates kernel/journal/cursor.go
2. A2 P0–P2 (backend)  → reuse the cursor; 11 endpoints
3. A3+B5 P0–P3         → kernel/auth + httpserver + openaiapi + restapi
4. A3+B5 P5 (webui)    → BEFORE A1 P6
5. A1 P2–P6            → domain package moves + route split (rides on Router)
6. A3+B5 P4,P6,P7      → tenant/oauth relocation, agentgw, token-file (security PRs)
7. C2 + C4             → frontend cleanup (independent; C2-voice ↔ C4-useVoice sequenced)
8. A2 P3 + C3          → frontend pagers across all list views (converged)
```

## Conventions for all plans

- Each phase ends green on the repo's existing gates: `go build ./...`, `go vet ./kernel/...`,
  `go test`, `tsc --noEmit`, `vitest run`, `npm run build`, plus `tools/{depscheck,sdkparity,deadcodecheck}`.
- Security-sensitive phases (A3+B5 P4/P6) additionally gate on the gitleaks secrets check.
- No plan requires a new gate; all use what CI already runs.
- Handlers stay as thin transport (`decode → call → encode`); domain logic moves down to its package.
