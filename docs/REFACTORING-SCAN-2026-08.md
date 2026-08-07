# Refactoring Scan 2026-08 — Deep System Scan + Master Action Plan

> **EXECUTION STATUS (2026-08-06): Phases 0–2 COMPLETE and merged.**
>
> | Phase | PR | Highlights |
> |---|---|---|
> | 0 — correctness (LD-1..8) | #553 | all 8 live defects fixed with regression tests |
> | 1 — shared primitives | #554 | projectJournal engine · fail/ok helpers · typed-args migration (295→19 annotated residuals + ratchet) · `kernel/jsonstore` behind 13 stores (fixed board.json silent-corrupt-overwrite) |
> | 2.1 — channel factory | #555 | `kernel/channelwire`; all 34 kinds; main.go channel section = one manifest loop; overlayEnv/buildAccountsLegacy/allInsts deleted; 34-vs-27 wiring drift closed |
> | 2.2 — tool registry | #556 | `kernel/toolreg`; all 32 boot tools as specs; 15 downcasts + 13 late Binds gone; **fixed live bug: fetch's netguard OnBlock was never wired** |
> | 2.3 — command registry | #557 | 319 commandSpecs; handleConn 705→~100 lines; tenantTokenAllows deleted; TenantAllowed⟹TenantRouted now a tested invariant; exhaustive tenant-boundary test |
> | 2.4 — provider bootstrap | #558 | `plugins/providerboot`; one Boot/Reload seam; **fixed 3 live drifts: middleware dropped on reload, frozen down-route eligibility, inverted first-run nudge**; TestBootReloadParity |
> | 2.5 — daemonconfig | #559 | typed `Load()` for 74 env names/10 clusters; live reads deliberately inline (Config Center os.Setenv semantics) |
> | 2.6 — boot decomposition | #560 | `kernel/selfrepair` (3.2k lines out) · `kernel/cadence/systemtasks` · HTTP-surface file split · PulseObservers + two-phase Deps/Bind · bootStep table; **fixed: tenant tokens refused during the srv.Start→SetTenants boot window** |
>
> | 3.1 — runtime.go split | #562 | runtime.go −40%; `{voice,market,mcp,image,rerank,script}tool.go` moved to their own packages |
> | 3.2 — `agent.Run` decomposition | b9787f01 | `run_setup.go` (pure prologue) · `run_provider.go` (`callProvider` collapses the streaming/non-streaming duplication) · `run_tools.go` (gate/execute/finalize on a `runState` that owns the prompt-injection causal window). Run: 833 → 309 lines; agent.go 1,712 → 1,203; coverage 79.5% → 80.5%. `runState` deliberately does NOT hold the conversation (dual-source-of-truth risk — finalize takes and returns it, pinned by a test) |
> | 3.4 — tool capability guard (commit A) | (this commit) | **fixed 6 tools that policy refused on EVERY call** — `conductor`, `market`, `voice`, `image_generate`, `rerank`, `file` `op=glob` — each resolved to a capability Edict does not govern, and unknown ⇒ default-DENY. New `market.install` axis; two mutation-verified guards (boot tools × every schema-allowed input, plus the runtime-registered set). Commit B (declaration onto `ToolDef`) still open |
> | 3.3 — per-run config resolution | 658b53ff | one `agentOverrides` table (key + doctor message + apply) replaces three copies of the same knowledge · `k.effectiveConfig(ctx)` = daemon config + live edits + agent overrides · **fixed 6 live boot-vs-live drifts: delegated sub-agents, workflow LLM nodes, workflow drafting, and memory/profile distillation all used the BOOT model after a provider reload hot-swapped it, and delegations appended the BOOT persona** · 4 single-key ctx wrappers deleted. Field-grouping half NOT done — see the correction below |
>
> main.go: 7,455 → 3,932 lines (−47%). Two plan corrections decided during execution
> (recorded in plans/phase2.6-boot-decomposition.md): `kernelAPIEngine` stays out of
> `kernel/restapi` (that package's interface-only posture is deliberate), and `NewServer`
> does NOT hard-validate deps (11 of 14 nils are legitimate operator configs).
> Per-phase working plans live in `plans/phase2.*.md`.
>
> **Remaining:** Phase 3.4–3.6, Phase 4 (frontend/hygiene), controlplane subpackage
> split (2.3 commit C+), webui route tables from registry metadata, and the
> `apperrors` delete-or-complete owner decision.

> **Generated:** 2026-08-05 · **Baseline:** `main` @ `069fe955` (clean tree)
> **Method:** 5 parallel deep scans (boot/wiring, controlplane, agentic core, extension
> surfaces, debt/tests) + import-graph analysis, cross-referenced against
> `docs/REFACTORING-SCAN.md` (2026-07-03) and the REFACTOR-A*/B*/C* plan corpus.
> **Supersedes** the sequencing section of `REFACTORING-SCAN.md`; individual findings
> there remain valid where not restated here.

## Status of the July scan (verified against source, not the docs)

| July item | Status |
|---|---|
| A1 Phase 1 (journal cursor helper) | ✅ shipped — `kernel/journal/cursor.go` |
| A3+B5 (`kernel/auth` + `kernel/httpserver`) | ✅ shipped — both packages exist with router/limits/auth |
| A2 (log pagination) | ✅ largely shipped (cursorPager + LoadMoreFooter pattern live) |
| B1 (runtime decomposition) | ⚠️ **partial** — `kernel/runtime/{voicetool,markettool,mcptool,imagetool,reranktool,scripttool}.go` still in runtime |
| B2 (`streamlimit` → governor) | ❌ not done — `kernel/streamlimit` still top-level |
| A4 (main.go decomposition) | ❌ not done — `main.go` now **7,455 lines** |
| A1 Phases 2–6 (controlplane split) | ❌ not done — 197 files, 30k LOC, 319-case switch |

---

# Part 1 — What the scan found

## 1.0 Calibration: what is genuinely good (do not churn)

- **Leaf-package hygiene is excellent.** `kernel/agent` imports only `bus/event/artifact/apperrors`; `kernel/pulse` has zero knowledge of `kernel/runtime` (bus-mediated by design); `kernel/governor` is layered (`Complete → completeChained → preflightAndRoute → runChain → callWithRetry`) with model/provider fallback correctly separated.
- **The interface seams that exist are right:** `agent.Provider/StreamingProvider/Tool/Steerer/Policy`, `warden.Engine`, `memory.Store/Embedder`, `pulse.Observer/BriefSink`, the 162-kind typed event bus instead of logging.
- **Three extension surfaces are already excellent:** skills (pure data), MCP servers (hot-attachable), market packs. Providers-via-catalog is good. Out-of-process tool plugins are the most defensively-written code in the repo (hash pinning, frame caps, callback caps, process-group kill).
- **Test discipline:** ~0.34 overall test/code ratio, fuzz + mutation tests in journal, 6 custom in-repo CI analyzers, zero TODO/FIXME debt markers.

The debt is **structural concentration**, not carelessness: three God-surfaces (`main.go`, `controlplane`, `runtime.Kernel`), and a shared-helper layer that exists but was silently unadopted.

## 1.1 Live defects found during the scan (fix before/independent of any refactor)

| ID | Defect | Evidence |
|---|---|---|
| **LD-1** | **Per-run cost ceiling is a no-op for delegated runs.** `executeSubAgent` sets `MaxRunCostMicrocents` but never `CostFn`; per `agent.go:390-394` nil CostFn disables cost accounting entirely. Sub-agents also get no context compaction, no artifact offload, no `ToolMemo`. | `kernel/runtime/subagent.go:696` vs `kernel/runtime/runtime.go:3515-3659` |
| **LD-2** | **Browser tool netguard audit never wired.** `main.go` asserts `tools["browser"]` but the tool is registered as `"browser.read"` — the assertion silently never matches, so the SSRF `OnBlock` callback is never installed. | `cmd/agezt/main.go:7202` vs `cmd/agezt/boot_tools.go:173` |
| **LD-3** | **Atomic-write divergence = data-loss risk on Windows.** 18 hand-rolled `<path>.tmp + os.Rename` sites with no fsync and a racing fixed temp name; only 4 (okr, workboard, seat, taste) grew the Windows rename-retry — roster/board/standing/workflow/cadence/mcp/toolforge saves can silently fail where OKR recovers. `internal/atomicfile`'s doc claims all sites wrap it — false (11 importers). | `internal/atomicfile/atomicfile.go:10`; `kernel/roster/roster.go:948` et al. |
| **LD-4** | **Provider retry asymmetry.** Only 3/11 adapters (openai, anthropic, ollama) wrap `Complete` in `retry.Do`; google/cohere/bedrock/vertex/openairesponses/embed/image/rerank/voice do not. **Zero** of the 7 `CompleteStream` implementations retry. A Gemini 429 fails the run; the identical OpenAI 429 recovers. | `plugins/providers/google/google.go:133` vs `openai/openai.go:127` |
| **LD-5** | **Aux LLM calls bypass governor accounting.** 12 direct `k.cfg.Provider.Complete(...)` sites (council, research, conductor, workboard, workflowdraft, workflowrun, verifyCompletion) hand-roll `CompletionRequest` with inconsistent `TaskType`/`CorrelationID` — task routing, task budgets and per-run spend attribution silently don't apply. | `kernel/runtime/council.go:185,227`, `research.go:155,222,297`, `conductor.go:245`, `workboard.go:312`, `workflowdraft.go:109`, `workflowrun.go:479`, `runtime.go:3094,3146,3907` |
| **LD-6** | **`runtime.Open` unwind leaks stores.** The 12 copy-pasted close-cascades have diverged; the configcenter failure path closes only 3 of 9+ opened stores. | `kernel/runtime/runtime.go:979-984` |
| **LD-7** | **`agt status` blind to 15 of 26 channel kinds.** `collectChannels()` is a third hand-maintained copy of channel-config knowledge covering only 11 kinds. | `cmd/agezt/main.go:3862-3957` |
| **LD-8** | **SSE concurrency gate applied to 1 of 9 SSE endpoints.** Only the firehose applies `sseGate` (V-009); 8 other hand-rolled SSE loops are ungated. | `kernel/webui/webui.go:1587` vs `:1078/:1143/:1192`, `openaiapi`, `restapi`, `agentgw` |

## 1.2 The three God-surfaces

**`cmd/agezt` (7,455-line main.go, 2,043-line `runDaemon`).** 121 imports, ~18 distinct concerns, 274 `os.Getenv` sites reading 312 env vars, 26 channel `buildX` functions (~1,320 contiguous lines, near-total duplication, two signature dialects, `overlayEnv` mutating process-global env + a `reflect.IsNil` probe to patch over it), provider selection implemented **twice** (boot + `cfg.OnReload` inline copy — the drift already caused a production bug per the comment at `main.go:6913`), 14 string-keyed type-assertion wiring sites that silently no-op on miss (LD-2 is the proof), and `main` acting as the designated cycle-breaker for the whole kernel (`SetBus`, `SetMarket`, `SetDiskFree`, shims, anonymous-interface assertions). `auto_repair.go` (1,894 lines) is a full subsystem stranded in `cmd/`.

**`kernel/controlplane` (197 files, 30k LOC, 434 methods on one `Server`).** One hand-written **319-case switch** in a 706-line `handleConn`; adding a command = 3 manual edits with no compiler check. `Server` has 38 fields, 9 of them nilable injected func-pointers as cycle-breakers. `roster.go` (4,924 lines) fuses six responsibilities: CRUD, a journal-projection engine (321-line accumulator function), 425 lines of event→English UI copy, a self-repair state machine, an 18-function mechanically-duplicated cascade-delete analyzer, and cross-subsystem mutation. 15 `*_log.go` files are character-identical journal-projection copies. `writeResp(conn, Response{...})` ×1,157 with no `ok/fail` helpers; typed arg accessors exist but raw `req.Args[...]` casts outnumber them 295:35 (the exact silent-failure class `args.go`'s own doc warns about). Tenant selection (`kernelFor`) is opt-in per handler — forgetting it silently uses the primary kernel.

**`kernel/runtime` (Kernel: 57 fields, 219 methods, 7 mutexes with documented lock order; Config: 72 flat fields).** `runtime.go` (4,401 lines) mixes store lifecycle, 24 context keys, policy/approval gating (10 gates in 150 straight-line lines), prompt assembly, run orchestration (540-line `RunWith`), and journal-causality walking. `agent.Run` is a single 816-line function (though its 3-phase tool dispatch design is sound). `RunWith` and `executeSubAgent` independently build `LoopConfig` and have already diverged (LD-1).

## 1.3 Extension-surface scorecard

| Surface | Registry? | Central edit to add one? | Runtime-addable? |
|---|---|---|---|
| Provider (existing dialect) | catalog JSON | **No** | Yes (`custom.json`) |
| Provider (new dialect) | family switch | 3 files | No |
| Skill | Forge, content-addressed | **No** | Yes |
| MCP server | `kernel/mcp.Store` | **No** | **Yes, hot** |
| Market pack | `kernel/market` | **No** | Yes |
| Tool (out-of-process) | `AGEZT_PLUGINS` | **No** | restart |
| Tool (in-process) | none — literal map | `boot_tools.go`/`main.go` + downcast sites | No |
| **Channel** | manifest only, **no factory** | **main.go ×3 sites + manifest + settings** | No |

The channel gap is the sharpest: `kernel/channel/registry.go:55` claims "no central edit" but `Manifest` carries no constructor, so 34 registered manifests vs 27 wired kinds already drifted, and a third-party channel can appear in the wizard UI yet never start.

**Dead architecture:** `contract/gen/types.gen.go` defines a full 7-kind capability plugin protocol (`channel|provider|tool|coding_agent|memory|storage|tunnel`, `RegisterParams`, `Contribution`) — **zero importers**. The shipped `kernel/plugin` implements tools-only with a one-string capability hint. Decide: implement `register` or delete the schema halves.

**Duplication census:** 11 JSON stores with hand-rolled `Open/save` (§LD-3); ~9 SSE implementations (§LD-8); 15 journal-projection files; 26 channel builders; `internal/apperrors` at 7/247 adoption with a `ctx` param it discards; 13 `context.TODO()` in `cmd/agt/overseer.go`.

**Frontend:** structurally healthy (data-driven NAV registry, clean 3-way split, code-split lazy views). Debt is 17 views >800 lines — worst `Roster.tsx` (one ~1,300-line component, 17 useState) and `Schedules.tsx` (35 useState) — while `views/Chat/` (12 files, 7 hooks) already demonstrates the house fix. `kernel/webui/dist` drift is unchecked in CI.

---

# Part 2 — Master action plan

**Principles** (all learned in-repo, see memory/git history):
- One extraction per PR, behind existing gates (`go build/vet/test`, `gofmt`, `vitest`, `tsc`, `make check`). No new gates needed.
- After any extraction, run the **source** package's tests, not just the new package's (the B1 lesson — extractions shipped with RED kernel tests once).
- Work in a private worktree from `origin/main`; never local main.
- Behavior-preserving by default; every phase-0 item is a bug fix and gets a regression test.

## Phase 0 — Correctness (small PRs, immediate, order-independent)

| # | Fix | Size |
|---|---|---|
| 0.1 | LD-1: shared `buildLoopConfig` used by `RunWith` + `executeSubAgent`; sub-agent-specific fields become explicit overrides. Restores CostFn/compaction/artifacts for delegated runs. Regression test: delegated run halts at cost ceiling. | M |
| 0.2 | LD-2: fix `tools["browser"]` → `"browser.read"` (or land 2.2's registry early). Test asserting OnBlock wired for every netguard-capable tool. | S |
| 0.3 | LD-3: swap all 18 hand-rolled writers to `internal/atomicfile.WriteFile` (fsync + unique temp + Windows retry in ONE place). Fix the false doc claim. | M |
| 0.4 | LD-4: add `plugins/providers/internal/wire` — `Do()`/`Stream()` owning retry/`httpread`/status→APIError/SSE framing; adapters keep encode/decode only. Uniform retry by construction. | M-L |
| 0.5 | LD-5: `k.completeAux(ctx, corr, taskType, req)` helper; route all 12 direct provider calls through it. | S-M |
| 0.6 | LD-6: `closers []func()` slice + defer-on-error in `runtime.Open`; delete 12 cascades. | S |
| 0.7 | LD-7: derive `collectChannels` from the manifest registry (or delete it once 2.1 lands). | S |
| 0.8 | LD-8: `kernel/httpserver/sse.go` shared writer with `sseGate` applied; migrate 9 call sites. | M |

## Phase 1 — Shared primitives (unblocks everything later)

- **1.1 `kernel/jsonstore.Store[T]`** — generic mutex-guarded JSON store (`Open`, `Get/Put/Delete`, atomicfile-backed). Migrate the 11 store packages one PR each. ~600 lines deleted; durability uniform.
- **1.2 Controlplane response/arg hygiene** — add `s.ok(conn, req, result)` / `s.fail(conn, req, err)` (single place for redaction + error codes, ~800 lines of noise removed); finish the abandoned `argString/argBool/...` migration (295 raw casts → typed), delete the competing `strArg` in `market.go`.
- **1.3 Journal-projection framework** — `s.projectJournal(req, kinds, decode)` replacing the 15 identical `*_log.go` scanners (~1,500 lines removed; pagination semantics fixed once).
- **1.4 `internal/apperrors` decision** — delete it (keep the prose convention) or give it `errors.Is/As` teeth. Current 7/247 adoption with an ignored `ctx` param is the worst of both.

## Phase 2 — Registry-fication (kills the God-surfaces' growth)

- **2.1 Channel factory** *(biggest single win, ~1,400 lines out of main.go)* — add `Manifest.New func(Deps) (Channel, pulse.BriefSink, string)` to `kernel/channel`; `Deps` bundles `{Bus, Handler, Get func(string) string}`. Migrate the 21 legacy builders to the `(label, get)` signature 6 channels already use (one PR per batch of ~5). Delete `overlayEnv` (process-global `os.Setenv` hack), `buildAccountsLegacy`, the reflect typed-nil probe, and the 27-element `allInsts` literal. Channel section of main.go becomes one loop over `channel.Manifests()`. Closes the 34-manifests-vs-27-wirings drift.
- **2.2 Tool registry** — `kernel/toolreg` with `Register(name, build func(Deps) (agent.Tool, error))` + per-tool `Configure(Deps)` hook so post-construction wiring (OnBlock, bus, kernel backrefs) is declared by the tool, not reached for by string key from main.go. Retires the 14 downcast sites (LD-2's bug class becomes a compile error).
- **2.3 Controlplane command registry** — `map[string]commandSpec{handler, tenantAllowed, streaming}` populated by per-subsystem `Register()`; `handleConn` shrinks to ~60 lines; `tenantTokenAllows` becomes a field (kills the second parallel switch); handlers no longer must be `*Server` methods → **enables splitting controlplane into subpackages** (finish July A1). Make `kernelFor` resolution part of the dispatch path, not per-handler discipline.
- **2.4 Provider bootstrap** — extract `buildGovernor/selectPrimary/buildFromCatalog/reconcileAlternateProviders` (~500 lines) into a `ProviderBootstrapper`; `cfg.OnReload` calls the same path instead of its inline copy (the documented drift-bug source).
- **2.5 `daemonconfig` package** — typed `Load() (Config, error)` replacing the ~450 lines of inline env parsing in `runDaemon`; unit-testable without booting a daemon. Keeps the `configEnvVars` guard test but anchored to one file instead of regex-scanning all of `cmd/agezt`.
- **2.6 Boot decomposition** — `[]bootStep{name, run}` table for the bind/seed phase; extract HTTP-surface assembly (`buildWebUI/Tunnel/OpenAIAPI/RESTAPI/Webhooks` ~700 lines); move `auto_repair.go` → `kernel/selfrepair`; scheduled system tasks → `kernel/cadence/systemtasks`; `kernelAPIEngine` → `kernel/restapi`. Replace the 9 nilable func-pointer fields on `controlplane.Server` with a `Deps` struct of narrow interfaces validated at `NewServer`.

## Phase 3 — Core decomposition (agentic quality)

- **3.1 `runtime.go` split along existing seams** (~1,400 lines out, near-zero behavior risk): `runctx.go` (24 context keys), `prompt.go` (promptBuilder: 8 injectors + the 220-line injection block in RunWith), `policy.go` (policyHook + approval bundles), `provenance` (Why/Causes → journal package). Then finish July B1: move `{voice,market,mcp,image,rerank,script}tool.go` to their homes; fold `streamlimit` into governor (July B2).
- **3.2 `agent.Run` decomposition** — `gateToolCalls` / `executeToolJobs` / `finalizeToolJobs` / `callProvider` (collapses streaming/non-streaming duplication); hoist the ~10 loop-locals into a `runState` struct so the prompt-injection causal window is reviewable in one place.
- **3.3 `runtime.Config` grouping** — ~~sub-structs for the existing clusters (Memory*, Skill*, World*, ContextBudget*, SubAgent*, Guards*)~~; one `effectiveConfig(ctx)` replacing the 9 ad-hoc per-run override lookups.

  **PLAN CORRECTION (decided during execution): the field grouping is not worth its cost; the override consolidation is, and it was the part hiding real bugs.**

  *Why the grouping was dropped.* `Config` is ~425 lines, but ~85% of that is doc comments, and grouping moves the comments along with the fields — the reading burden barely changes. Against that: `Config` is the kernel's public construction API, so the rename touches `cmd/agezt`, `daemonconfig`, and 15 test files, and every out-of-tree embedder. It buys no behavioral correctness, which is what Phase 3 is for.

  *What the override half actually found.* The same key→type→field knowledge lived in three places (a key list, a validation switch, and a hand-written lookup at each point of use), so a key could pass the doctor and then be silently ignored at run time. Worse, the *reason* those lookups were ad-hoc — every consumer reaching into `k.cfg` directly — meant six of them were reading the BOOT seed of fields that are hot-swappable at run time (`SetModel`/M816, `SetSystem`/M710). After an operator rotated a provider key or switched models, root runs moved to the new model while **delegated sub-agents, workflow LLM nodes, workflow drafting, and memory/profile distillation kept requesting the old one** — normally the model that had just stopped being servable, so those paths failed while chat looked healthy. Delegations likewise appended the boot persona, ignoring persona edits.

  Fixed by making the correct read the easy one: `k.effectiveConfig(ctx)` returns the config a run actually sees (daemon-wide → live edits → this agent's overrides), and reading `k.cfg` inside a run is now the thing that looks wrong. Pinned by mutation-verified tests (`TestSubAgent_ModelFollowsLiveDefault`, `TestSubAgent_SystemFollowsLivePersona` — both confirmed red against the old code).
- **3.4 `ToolDef.Capability`** — capability declared on the tool definition; `edict.CapabilityForToolCall` name-switch becomes fallback only. Retires the `Config.ToolCapabilities` side-channel and the `forge_`/`mcp_` prefix hacks; adding a tool stops requiring an edit in another package.

  **Commit A done — the guard, and the six tools it found dead.** Scoping this phase surfaced the cost of the split the phase exists to close: because the axis is declared in `kernel/edict` rather than on the tool, a tool whose name never reached the switch resolved to an **unknown capability, which Edict default-DENIES**. Forgetting the edit did not degrade a tool, it killed it — every call refused with `no trust level configured for "<tool>"` under the daemon's allow-everything posture. Found dead: `conductor`, `market` (agent-facing; the CLI/HTTP paths worked, which is why the marketplace loop verified green), `voice`, `image_generate`, `rerank`, and `file` `op=glob` (implemented and in the tool's own schema enum, no case in the switch). Mapped each to a real axis (`code.exec`, `market.install` — new, `provider.call`, `file.list`) and added two guards: every boot tool must resolve to a governed capability *for every input its schema allows*, plus the same check over the runtime-registered set (four of the six lived there, invisible to the registry package). Both mutation-verified red. Note for auditors: `conductor` rides `code.exec` because its verifier executes the worker's code through an in-kernel call that never re-enters the policy engine.

  **Commit B (remaining):** move the declaration onto `ToolDef` so the edit happens in the tool's own package. Design constraint found while scoping: 8 of ~30 tools pick their axis from an input field (`op`/`method`/`operation`), so a plain `Capability string` is not enough — the declaration needs a field name plus a value→axis map with an author-chosen default (readers default to their read axis, installers to their gated one). `ToolDef.Effect`'s `json:"-"` is the precedent for governance metadata that stays off the provider wire, and `agent.PolicyToolDefFromContext` already threads the def into `policyHook`, so no new plumbing is needed. Precedence should be declaration → plugin manifest overlay (`Config.ToolCapabilities`) → the name switch, with the same `KnownCapability` validation applied to in-tree declarations that plugin manifests already get, or a typo'd declaration reintroduces exactly the default-deny bug above.
- **3.5 `roster.go` split** — `roster_crud` / `roster_status` (projection + cache) / `roster_activity_text` (UI copy — move toward the presentation layer) / `agent_repair` (state machine) / `agent_cascade` with a `CascadeTarget` interface replacing 18 hand-written impact/mutate function pairs.
- **3.6 Governor polish** — `[]gate` slice for the 245-line `preflightAndRoute` cascade; unify the 3 budget-scope implementations; make pricing catalog per-Governor instead of the package-level `liveCatalog` global (multi-tenant correctness).

## Phase 4 — Product/frontend + hygiene

- **4.1 Frontend god-views** — apply the proven `views/Chat/` pattern to `Roster.tsx` and `Schedules.tsx` first (worst offenders), then the remaining >1,000-line views. `views/roster/` helpers already exist — finish adopting them.
- **4.2 `contract/gen` decision** — implement plugin `register` + kind dispatch (would subsume 2.1/2.2 for out-of-process channel/provider plugins — the strategic option), or delete the unimplemented schema halves and document `kernel/plugin` as tools-only.
- **4.3 CI: webui dist drift check** — job verifying committed `kernel/webui/dist` matches a fresh `npm run build`.
- **4.4 Hygiene** — delete `scripts/dev/` scratch files; move `security-report/` under `docs/` (findings there rot — see memory "stale audit reports"); resolve the `sdk/` vs `plugins/sdk/` naming trap (rename the latter's module path docs, or at least README both); `context.TODO()` sweep in `cmd/agt/overseer.go`.
- **4.5 July leftovers still worth doing** — D1/D2 (agt CLI output unification, controlplane-client-only discipline), B4 (scopedstore — largely satisfied by 1.1), B6 (shared trigger evaluator), G2 (boundary integration tests — do after 2.3 so they test the registry, not the switch).

## Sequencing & dependency notes

```
Phase 0 (all parallel, immediate)
Phase 1.1 ──► 3.5 roster split (uses jsonstore-backed stores)
Phase 1.2/1.3 ──► 2.3 command registry ──► controlplane subpackage split ──► 4.5 D1/D2, G2
Phase 2.1/2.2 ──► main.go shrinks ~2,000 lines ──► 2.5/2.6 tractable
Phase 0.1 ──► 3.1/3.2/3.3 (loop-config builder is the seam they share)
4.2 decision gates whether 2.1/2.2 registries get an out-of-process tier
```

Rough effort: Phase 0 ≈ 8 small PRs; Phase 1 ≈ 6–8 PRs; Phase 2 ≈ 12–15 PRs (2.1 alone is ~5); Phase 3 ≈ 10–12 PRs; Phase 4 ongoing. Every PR independently green and shippable; no big-bang branch.

**Expected end state:** `main.go` < 1,500 lines (load config → build services → mount registries → banner → serve → drain); adding a channel/tool/provider/command touches only its own package; one atomic writer, one JSON store, one SSE writer, one journal-projection helper; sub-agent runs governed identically to top-level runs; retry semantics uniform across all providers.
