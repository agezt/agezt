# 21 — Migration Roadmap (from today to the target architecture)

> **Companion of** [20-target-architecture.md](20-target-architecture.md). **Strategy:** module-by-module
> strangler. Each module's boundary is redesigned and its internals are rewritten where they are the
> problem. `main` stays green after every PR. There is no long-lived rewrite branch.

---

## 1. Working rules (apply to every PR in every wave)

1. **One concern per PR**, independently shippable. Gates:
   - `go build ./... && go vet ./...`
   - `GOMAXPROCS=4 go test ./...`
   - `git ls-files '*.go' | xargs gofmt -l` (empty)
   - frontend `npm test` + `npm run build` when touched
   - `tools/archcheck` (from W0 on)
2. **Run the SOURCE package's tests**, not only the new package's. An extraction once shipped red because
   only the destination was tested.
3. **Moves before rewrites.** First move code verbatim behind the new boundary, using Go type aliases or
   forwarding shims so importers do not churn. Then rewrite internals in a separate PR. A diff that both
   moves and changes is unreviewable.
4. **Regression tests are mutation-verified.** Every bug fixed on the way gets a test that is shown red
   against the old code. Restore from a file copy, never `git checkout --`.
5. **Delete the shim in the same wave it was introduced.** Shims are tracked in the archcheck allowlist
   with an owner wave; the allowlist may only shrink.
6. **Update the codemap** (`architecture/0x-*.md`) in the PR that changes the structure it describes.
7. **Behaviour changes are explicit.** A PR either preserves behaviour (says so) or changes it (says what,
   why, and how the operator notices).

---

## 2. Waves

```
 W0 Guardrails + P0 fixes ──► W1 Contracts & platform ──► W2 App layer & single ingress
                                                     └──► W3 Dissolve Kernel into modules ──► W4 Autonomy/channels/config
                                                                                          └──► W5 Generated surfaces & cleanup
```

W2 and W3 can interleave per domain once W1 is done: migrate domain X's ops into `app`, then extract
domain X's module.

### W0 — Guardrails and the P0 defects (start immediately)

| PR | Content | Exit check |
|---|---|---|
| W0.1 ✅ | **`tools/archcheck`**: declarative layer map (`tools/archcheck/layers.json`: every one of today's 200 packages → L0…L7 + module), rule engine over `go list -json`, ratchet allowlist (`tools/archcheck/allowlist.txt`), `make arch-check` + CI step. **Baseline 2026-10-02: 200 violations: 88 plugin-reach, 70 cross-module, 34 adapter-bypass, 8 upward.** | CI fails on any *new* forbidden edge, on fixed-but-still-listed edges, and on unplaced packages (ratchet verified on the real repo) |
| W0.2 ✅ | **Forbidden-call rules in archcheck** (`tools/archcheck/calls.go`, stdlib `go/parser`, no new dependency): `exec` (`os/exec.Command*`), `http-client` (constructing `http.Client`, `http.DefaultClient`, `http.Get/Post/Head/PostForm`; an *injected* `*http.Client` field is fine), `raw-write` (`os.WriteFile`/`os.Create`). Legitimate homes are declared in `layers.json` `"calls"`; L0/L7 exempt. Per-package count ratchet in `tools/archcheck/calls-allowlist.txt`. **Baseline: 85 sites in 57 packages: 13 exec, 58 http-client, 14 raw-write.** Both ratchets analyse the union of linux/windows/darwin `go list` output, so the result is identical on every host (a host-only view flipped `kernel/creds` between 1 and 2 exec sites). | New unmanaged children/clients/writes fail CI; counts may only go down |
| W0.3 ✅ | Security P0s: REST `/metrics` → admin token, `health`/`models` answer from the bound tenant engine; per-client login lockout + global backstop; channels run under `runChannel` (Start error/panic → instance marked dead + `channel.error`); `channel.Guard` per message on IRC/email/Mastodon; email allowlist case-insensitive (`NewFoldedAllowlist`); plugin children get the `envscrub` base + `AGEZT_PLUGIN_ENV` grants. **Correction:** the web console's unfiltered `/events` is not a tenant leak — the console admits only operator credentials | One regression test per item, each shown red against the old code |
| W0.4 ✅ | Correctness P0s: `IsTransient` honours `TransientError` and the governor stops re-retrying a refused dial; `tool_search` declares `introspect`, `browser.action` + 10 verbs declare `browser.action`, guards build tools with opt-ins **enabled**; update checker drains inside `Apply` (after verification) and resumes on failure; anomaly breaker re-arms on `resume`; anomaly/alerter contain panics per event | Same |
| W0.5 ✅ | Dead code removed: `kernel/runtime/compose`, `kernel/workflowexec`, duplicate `delegation` tool types, double `ErrHalted`/`ErrNoVisionModel` (now one identity), orphaned `update.Service.DrainTimeout`, never-implemented plugin kinds in `agezt-contract.jsonc`; `codegen-in-sync` now builds the generated package. `AGEZT_BROWSER_COOKIES` turned out to be a **lost feature**, not dead config — rewired. `runtime/accessors` deferred to W3 (dissolved with the Kernel) | deadcodecheck clean; archcheck 200 → 196 |

### W1 — Contracts and platform foundations

| PR | Content | Notes |
|---|---|---|
| W1.1 | `kernel/contract/{llm,tool,event,channel}`: **move** the type/interface declarations out of `kernel/agent`, `kernel/event`, `kernel/channel`; leave `type X = llm.X` aliases in the old packages | 67 importers keep compiling; zero behaviour change |
| W1.2 | Repoint `memory`, `worldmodel`, `governor`, `skill`, `catalog` at `contract/*`; remove their `kernel/agent` import | Removes the data→loop edge (archcheck entries drop) |
| W1.3 | `platform/store`: `Registry` + `Collection[T]` (atomic, 0600, path-unique) + cross-process `Lock`; migrate the 13 `jsonstore` users one PR each; vault and settings take the lock | Fixes single-instance hazard, perms, vault races |
| W1.4 | `platform/netout`: `Client(Profile)` + uniform retry; migrate providers (one family per PR), market sync, webhooks, fetch/http/websearch tools | One SSRF posture per profile; fixes `fetch` allowlist |
| W1.5 | `platform/sandbox.Launcher`: wraps warden + envscrub; migrate plugin host, coding/git, acp_agent, browser driver, MCP stdio, code_exec, shell | Docker backend stops putting secrets in argv |
| W1.6 | `platform/eventlog`: **kind registry** (every kind registered with payload schema; ad-hoc kinds become constants; 8 dead kinds deleted or emitted) + **sidecar index** (correlation, cause, kind, subject; rebuildable) + `Project` engine; `why`, channel history and epistemic gate use the index | Removes full scans; corrupt mid-segment → quarantine + read-only, not boot abort |
| W1.7 | `platform/policy`: move Edict + approval + guards (epistemic/intent/injection out of runtime) behind `Decide`; persist pending approvals | Guard code leaves `kernel/runtime` |
| W1.8 | `platform/config` + `platform/secrets`: one precedence, schema registry drives RequiredEnv/UI/env docs; `configcenter` folded in (secrets → vault, plaintext files migrated and deleted) | Two Config Centers become one |

### W2 — Application layer and single ingress

| PR | Content | Notes |
|---|---|---|
| W2.1 | `kernel/app`: `OpSpec`, registry, `Dispatch` pipeline (authn → op → authz → tenant → validate → audit), streaming contract; generic JSON-schema derivation from Go types | Framework only, plus 1 pilot domain (`status`) |
| W2.2 | `app/runs.Start` + `RunRequest`; **re-point all 10 entry points** (`api_engine`, `main_cadence`, `main_channels_handler`, `main_standing*`, controlplane `server_handle_run`, `roster_escalation`, `workboard_dispatch`, `selfrepair_wake_agent`, overseer) | Admission gates now apply to REST/OpenAI/channels/triggers; resume ticket stores `{agent, RunRequest}` |
| W2.3 | `app/tools.Invoke`; re-point agent loop, workflow nodes, `toolexec`, council grounding, conductor verifier | Every tool call journaled + policy-checked; side-path findings closed |
| W2.4…W2.n | **Migrate control-plane domains** into `app` ops, one domain per PR, in the order of [§3](#3-domain-migration-order); control plane keeps a compatibility adapter until its last domain moves | 28 domains, ~321 ops |
| W2.x | Web UI routes generated from `OpSpec.HTTP`; delete the 198-entry table; File Manager + rollback become ops (journaled, policy-checked) | Unjournaled writes fixed |
| W2.y | REST, OpenAI and agentgw re-implemented as `app` adapters; agentgw socket path published to children via env | Tenant leaks and bypass closed; SDK default-socket mismatch fixed |

### W3 — Dissolve `runtime.Kernel` into modules

Extract in **increasing coupling order** (each step = move PR, then rewrite PR if needed):

1. `modules/reasoning` (council, conductor, research): stateless, depends on runs/tools/modelgw only.
2. `modules/knowledge` (memory, world, taste, profile distill). Fix the profile-facet supersede bug in the rewrite PR.
3. `modules/skills` (+ builtin seeding; stop re-promoting operator-quarantined built-ins).
4. `modules/artifacts` (artifact + datalake; GC respects journal `raw_ref` via eventlog index).
5. `modules/board`.
6. `modules/work` (workboard, OKR, proof, assure, seats; fix `"container"` seat → profile mapping).
7. `modules/workflows` (move `runtime/workflowrun*` + draft + test-node; fix trigger `source` labels).
8. `modules/agents` (roster, guardians, selfrepair, reaper, overseer ops). **This removes the
   `kernel/controlplane`/`selfrepair` → `plugins/tools/overseertool` edge**; the overseer tool becomes an
   L6 caller of `app` ops. Fix guardian reconcile resetting operator-lowered caps.
9. `modules/extensions` (mcp, plugin host, toolforge, toolbox, acpcatalog).
10. `modules/providers` (catalog, chatgptauth, providerboot).
11. `modules/runs` last: what remains of `kernel/runtime` + `runexec` + `agent` loop + resume +
    delegation + intervention. `kernel/runtime` is deleted when empty.

**Exit:** `kernel/runtime` and `kernel/controlplane` (handlers) no longer exist; no type has more than 60 methods.

### W4 — Autonomy, channels, config unification

| PR | Content |
|---|---|
| W4.1 | `modules/triggers`: one `Trigger{Source: cron\|once\|event\|webhook\|observation\|continuous, Target: run\|workflow\|systemtask\|tool, Policy: trust ceiling, budget, noise}`. Migrate cadence, standing orders, pulse initiative bindings, workflow triggers and system tasks with lossless store migrations. **System tasks get capabilities and go through policy.** |
| W4.2 | `modules/pulse` absorbs anomaly + alerter; initiative level persisted and live-editable. |
| W4.3 | `modules/channels`: supervisor (start error → backoff restart + health), panic containment for every transport, **conversation store** (append inbound + every reply; replaces the per-message journal fold), accounts derived from config schema (RequiredEnv generated, qq/wechat/zalo get schemas), webhook ingress acks then runs async uniformly. |
| W4.4 | Email inbound: DKIM/SPF verification (Authentication-Results trust or local DKIM verify), case-insensitive allowlists; IRC/Twitch allowlist by nick, not channel. |
| W4.5 | Self-update: release-signing key injection at build, SHA in GitHub path, signature through apply. |

### W5 — Generated surfaces and cleanup

| PR | Content |
|---|---|
| W5.1 | OpenAPI from op registry; generated TS client in `frontend/src/app/api`; delete hand-written wrappers. |
| W5.2 | Generated Python/TS/Rust/Go SDKs + server-produced golden fixtures consumed by every SDK test; retire `sdkparity` freshness check and self-validating fixtures. |
| W5.3 | Frontend boundaries lint (features ↛ features, components/lib ↛ features); `designsystem.test` scans `features/`; delete test-only modules. |
| W5.4 | Final: archcheck allowlist = 0, forbidden-call allowlist = 0, codemap docs regenerated, `docs/` stale architecture docs archived. |

---

## 3. Domain migration order (W2.4…)

Order is by risk × leverage. Low-risk read paths come first to prove the framework; the destructive and
streaming domains come last.

| Order | Domains (control-plane file prefixes) | Why here |
|---|---|---|
| 1 | status, version, catalog, provider | read-mostly, pilot |
| 2 | memory, world, taste, skill | will become modules early in W3 |
| 3 | board, workboard, okr, storage/artifacts | |
| 4 | schedule, standing, workflow, pulse, autonomy | precede W4 triggers |
| 5 | tool, toolforge, toolbox, mcp, market, plugin | depend on W2.3 invoker |
| 6 | config, settings, configcenter, channel(s), webhook, tunnel, update | depend on W1.8 |
| 7 | roster (27 files), steer, runs, journal, edict, tenant, shutdown, remote | highest blast radius, streaming |

---

## 4. Risk register

| Risk | Mitigation |
|---|---|
| Behavioural drift during moves (the B1 lesson) | Move-only PRs with aliases; source-package tests mandatory; boot-reload parity and tenant-boundary tests kept green |
| Lossy store migrations (W1.3, W4.1) | Migrations are versioned, idempotent and keep the old file as `.bak` for one release; round-trip tests on real fixture homes (`.dev-home` snapshots) |
| Long-running dual paths (old control-plane handler + new op) | Each domain PR deletes the old handler in the same PR; the compatibility adapter only routes |
| Op registry becomes a new god table | Ops are registered *by modules* (`ops.go`); the registry holds descriptors only, not logic |
| Generated clients break the console | Generate → typecheck → vitest in the same PR; the console's guard tests stay |
| Performance of pipelines | Pipelines are plain function composition; benchmark `Dispatch` and `Invoke` overhead in W2.1/W2.3 (budget < 50 µs/op excluding audit I/O) |
| Concurrent sessions editing main | Per owner rule work on `main`; use a worktree only when sessions actually overlap |

---

## 5. Owner decisions (DECIDED 2026-10-02)

All five were decided by the owner as recommended: **5.1 (a) strangler · 5.2 (a) under `kernel/` · 5.3 (a) JSON + index · 5.4 (a) delete dead halves · 5.5 (a) persist approvals.** Also approved: **cadence system tasks go through policy** (W4.1).

| # | Decision | Options | Decision |
|---|---|---|---|
| 5.1 | Strategy | (a) module-by-module strangler · (b) parallel `v2` tree rewritten from zero, cut over at the end | **(a)**: keeps 168k test lines as the spec and `main` shippable |
| 5.2 | Package layout | (a) `kernel/{contract,platform,modules,app,adapters}` · (b) new top-level `contract/ platform/ modules/ app/ adapters/` | **(a)**: one root for the core, smaller import-path churn, `plugins/` unchanged |
| 5.3 | Storage engine | (a) keep JSON files + eventlog sidecar index · (b) embedded pure-Go SQLite for stores and index (new large dependency) | **(a)** now; revisit after W1.6 benchmarks |
| 5.4 | Out-of-process channel/provider plugins | (a) delete dead `contract/gen` halves, tools-only protocol · (b) implement `register` + kinds now | **(a)**; reintroduce when there is a real external channel/provider author |
| 5.5 | Approvals across restart | (a) persist pending approvals · (b) keep in-memory (timeout ⇒ deny) | **(a)**: a restart should not silently deny the operator's queue |
