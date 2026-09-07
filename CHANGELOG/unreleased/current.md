# Changelog — current

This file holds the active `[Unreleased]` working set.

### Unclassified

- **Security: the update service trusted the wrong thing.** `verifySignature` chose its trust anchor
  from `s.cfg.Source`. With the shipping default — `SourceGitHub`, no embedded signing key — it
  accepted *any* manifest, on the reasoning that GitHub's TLS pipeline was the anchor and the
  manifest's SHA256 was merely informational.

  The premise silently failed. Both `POST /api/v1/update/apply` and its control-plane twin build an
  `UpdateInfo` entirely from a request body, so the GitHub anchor was being asserted for a URL that
  never came from GitHub, and the downloaded binary was checked against a hash the same caller
  supplied. An admin-token holder could stage an arbitrary executable over `<baseDir>/bin/agezt`,
  persistent across restart and token rotation.

  Provenance now travels *with* the manifest — set only by `checkGitHub` and `checkEndpoint`, never
  by a caller. The zero value is the untrusted one, and that is the whole fix: `Source` has
  `SourceGitHub` at `iota` 0, so adding a `Source` field to `UpdateInfo` would have made every
  hand-built struct inherit the trusted origin by default and reproduced the bug somewhere new.

  **This closes self-update on stock builds** — `Apply` now refuses without a signing key. It was
  never open by a legitimate route: `checkGitHub` populates neither `SHA256` nor `Signature`, and
  `validateSHA256` rejects an empty hash, so `Check → Apply` could not complete either. The only
  functioning path was the unverified one. Wiring the signed path end to end is release engineering.

- **Security: boot cleared the console's strict-password flag one line after raising it.**
  `SetAllowedHosts` auto-raises strict mode (token AND password) for any non-loopback host, because
  the console is then reachable beyond localhost and a guessed password alone must not open data
  routes. Boot then called `SetPasswordStrict(env)` unconditionally, and the env default is false.

  An operator who set `AGEZT_WEB_PASSWORD` and bound beyond loopback believed they had two factors
  and had one; the password alone then opened `/api/run`, `/api/files/*` and `/api/config/set`. A
  wildcard bind was worse still: `webAllowedHosts` skips unspecified IPs, so `0.0.0.0` registered no
  allowed host and the auto-raise never even evaluated — while `hostAllowed` accepts any IP literal,
  leaving the console LAN-reachable. Both paths are fixed, and the env var remains an explicit
  override in *both* directions; it is simply only applied when actually set.

  Boot now reads the effective value back rather than re-deriving it from the environment, since
  re-deriving is what went wrong. That value also feeds the boot banner and the tunnel URL, which
  were therefore describing the wrong posture too.

- **Security: the SDK socket default could not reach the daemon, and failed open.** Both the
  TypeScript and Python SDKs default to `@agezt/agentgw.sock` and passed it straight to `connect`.
  Go maps a leading `@` to the Linux **abstract** namespace; neither Node nor CPython does — both
  copy the string into `sun_path`, making it a CWD-relative *file path*. So the SDKs could never
  reach the daemon on Linux, and an agent subprocess whose working directory an attacker can write
  to would hand `Authorization: Bearer <capability token>` to whatever was planted at that path.
  Both SDKs now translate to the platform's abstract form on Linux and leave the literal alone
  elsewhere, where Go binds the literal too.

- **Security: `npm ci` ran install scripts on the self-hosted runners.** Dependabot opens weekly npm
  bumps for `frontend/` and `sdk/typescript/`; those branches live in this repo, so they pass every
  fork guard and run the full pipeline on persistent, non-ephemeral runners. Six `npm ci`
  invocations lacked `--ignore-scripts`, so an install script from a freshly-bumped transitive
  package executed there before any human read the diff. The flag costs nothing here: the only
  package in either lockfile declaring an install script is `fsevents`, which is optional and
  darwin-only.

- **Security: auto-repair's cooldown and attempt cap never bound.** Both guards key on a fingerprint
  that is supposed to identify the repair *target*, but all six builders embedded the incident's live
  metrics — `failures=%d`, `count=%d`, a generation counter that increments on every forced re-route
  — plus the newest error string. Every one of those changes on recurrence, so the fingerprint
  differed each time, the cooldown never matched, and the attempt count was always zero. An agent
  could be auto-repaired with no delay and no limit. Fingerprints now carry only stable identity;
  the detail lives on in the journaled reason string.

- **Fixed: three `make check` gates were red, and one of them told you to delete a correct document.**
  A full-tree scan found the running system healthy — build, tests, static analysis, vulnerability
  scan, real-daemon e2e and the embedded SPA all green — while the layer that guards it had rotted
  in three places, all invisible because the CI runners were not reporting.

  `tools/sdkparity` extracted the daemon's REST routes by grepping for `mux.HandleFunc("…")`.
  The 2026-07-26 route-auth centralisation changed every registration to
  `router.Handle(path, policy, handler)`, so the pattern matched **nothing** and the generator
  produced an empty route table. `-check` then reported the (still correct) `docs/SDK-PARITY.md` as
  stale and printed a remedy — regenerate with `-out` — that would have deleted all thirteen routes
  and rewritten every SDK's coverage from `9/11` to `0/0`. The extractor now matches the call shape
  rather than one receiver name, and a guard test asserts a non-empty extraction against the real
  `restapi.go`: the absence of that single assertion is what let a seventeen-day regression pass for
  a documentation problem.

  `gitleaks` had been failing since the same commit, on the same day, for a related reason: the new
  `kernel/auth/auth_test.go` carries a synthetic `"0123456789abcdef"`-twice token proving
  `WriteTokenFile` truncates to `0123…cdef`, and `.gitleaks.toml`'s fixture allowlist predates the
  package. One commit took out two gates. The path is now allowlisted with its justification.

  `deadcodecheck` reported fifteen unreachable functions, addressed below.

- **The netguard drift alarm was never armed.** `toolreg.Set.NetguardGaps` reports egress-guarded
  tools whose built instances do not implement `NetguardAware` — their SSRF refusals never reach the
  journal, so a blocked request looks identical to one that was never made. It shipped with Phase 2.2
  and only its own unit test ever called it, against a fixture `Set`; boot never ran it, so a real
  gap was undetectable. It now runs against the real built `Set` in `buildTools` and warns on stderr:
  an unjournaled refusal is a visibility loss, not an open door, so it degrades rather than refusing
  to boot. Against the current tree it fires nothing.

  Found because `deadcodecheck` runs the analyzer **without** `-test`, so "reachable" means
  "reachable from a binary". Adding `-test` would have cleared thirteen of fifteen findings in one
  line and hidden this class of bug permanently, so the strictness stays.

  Deleted rather than kept: `channelwire.BuildAll` (a duplicate of the manifest walk `main.go`
  already performs), `channelwire.Describe` (zero references anywhere), `channelwire.MissingFactories`
  (the real manifest-vs-factory check runs in `plugins/builtinchannels` against actual manifests),
  and the legacy pulse observer shim — `funcObservers`, `SetDiskWatch`, `SetProbeWatch` and the type
  switch they forced on every availability check, which collapses `diskWatchAvailable` and
  `probeWatchAvailable` into one method. Same-package test-only helpers moved into `_test.go` files
  instead of being allowlisted; exactly one symbol could not move (`toolreg.Names`, whose consumer is
  a ratchet in the package that registers its specs) and is pinned by `file|symbol`, never by
  directory, so the next dead function in that file is still reported.

- **Fixed: 31 environment variables the daemon reads were in no configuration surface at all.**
  Neither `agt config show` nor the Config Center knew about them, including
  `AGEZT_AGENTGW_TOKEN_SECRET` (the gateway token signing key) and
  `AGEZT_AWS_CREDENTIAL_PROCESS_ALLOWED` (which permits executing an external binary to mint
  credentials).

  The cause was structural. `TestConfigEnvVars_CoversCmdAgeztReads` scanned a hand-maintained list of
  eight directories, each added reactively after a refactor moved env-reading code somewhere new, so
  every extraction quietly shrank the guard's coverage — the test's own comment already recorded the
  invariant rotting once before. The scan is now an **exclusion** list over `kernel/`, `plugins/`,
  `internal/` and `cmd/agezt`: a new package is covered by default. Its matcher also widened past
  `os.Getenv(…)`, which had been blind to `const TokenSecretEnv = "AGEZT_…"` and
  `envLookup(lookup, "AGEZT_…")` — that blindness is why the hand count was 13 and the real number
  was 31.

  All 31 are in `configEnvVars` (presence only; values are never echoed) and all 31 now have Config
  Center fields, across `provider`, `interfaces`, `security` and two new sections, `files` and
  `run-health`. `AGEZT_CHATGPT_OAUTH` is the one exception to editability: it is the vault key
  holding the OAuth blob that "Sign in with ChatGPT" writes, so it renders read-only rather than
  inviting a pasted token.

- **Every nav view is now mounted against a real daemon.** `frontend/e2e/views.spec.ts` walks all
  eight sections, opens every view the nav offers, and asserts each renders a non-empty `<main>` and
  logs no console error, attributing errors to the view that produced them. Item labels are read from
  the DOM, so a view added to `src/nav.tsx` is covered the day it lands. 66 views, 0 blank, 0 errors.

  It exists for a regression that already happened: a design sweep left `ui/tab-nav.tsx` uncontrolled
  and Dashboard, Runs and Status Overview shipped a blank panel while every unit test passed. The
  components were correct in isolation; only mounting them in a browser against a live daemon showed
  it. The spec prints its own coverage count and guards a floor, because a breadth test that quietly
  stops covering things is worse than one that fails.

- **Removed: `internal/apperrors`.** All four of its functions were `fmt.Errorf("%s: %w", …)` with a
  nil guard, `Wrap` and `Wrapf` took a `context.Context` they discarded, and the `Code` type it
  exported along with eight error-code constants had zero references anywhere in the tree. At 43
  call sites against roughly 250 error returns it was neither a convention nor an abstraction —
  just a second way to spell the first one, and the half-adoption meant a reader had to know both.

  All 43 sites now use `fmt.Errorf` directly; error strings are byte-identical. The nil guard was
  dead weight too — every one of the 43 was already inside an `if err != nil`, including the three
  that returned it bare from a helper (checked individually rather than assumed, since a nil there
  would have turned a `nil` return into a `%!w(<nil>)` error).

  The prose convention the package documented — package-prefixed messages, `Err*` sentinels,
  always `%w` — was the part worth keeping, so it moved to `docs/ARCHITECTURE.md` § Error
  Convention, where it applies to all of the code instead of a sixth of it.

- **Refactor (Phase 4.1): `Schedules.tsx` 2,124 → 1,556 lines.** The view's data model and its
  derivations — the wire shapes the schedule endpoints return, and the pure functions that turn
  them into counts, labels, attention reasons, health passports and filters — moved to
  `views/schedules/shared.ts`, mirroring the existing `views/roster/`. Nothing in there renders, so
  a derivation and its test can now be read without scrolling past six hundred lines of JSX.

  It also fixed a layering inversion: `lib/snapshot.ts` was importing `parseSchedulesJSON` from a
  *view*. Library code depending on a view is backwards; it now imports the model directly. The
  test suite was split the same way, so the twenty pure helpers are tested against the module that
  owns them rather than through the view that happens to render them.

- **Fixed: a daemon-wide budget breach was invisible to anything filtering by scope.** The three
  daily ceilings — daemon-wide, per-task-type, per-agent — each built their own `budget.exceeded`
  payload, and they had drifted: the task and agent ones carried a `"scope"` field, the global one
  did not. An operator or observer filtering budget events by scope therefore saw task and agent
  breaches but never a global one, which is the most important of the three. All three now report
  their scope.

- **Refactor (Phase 3.6): the governor's pre-dispatch cascade is a table.** `preflightAndRoute` was
  245 lines of eight sequential concerns, and their ORDER is load-bearing but was discoverable only
  by reading the whole function — down-routing must precede the strict capability gate so a
  remappable request is remapped rather than rejected, and the rate gate must precede the budget
  gates so a call blocked on frequency never touches the spend ledger. Both are now stated where
  the order lives. Each step is its own named, independently testable method in `preflight.go`.

  The three budget ceilings collapsed into one `budgetScope` shape (`budgetgate.go`). They were
  three different shapes for the same decision: two returned `(exceeded, spent, ceiling)` triples
  that each caller unpacked and journaled itself, and the third was written inline in the middle of
  the cascade — which is how the payload drift above went unnoticed. `budgetExceeded` and
  `taskBudgetExceeded` survive as thin wrappers over the table, deliberately: the existing boundary
  tests drive them directly, and keeping them means those tests still exercise the real path.
  Guarded by a test that every scope journals its identity, mutation-verified against the old shape.

- **Refactor (Phase 3.5): `controlplane/roster.go` 4,949 → 2,957 lines.** One file fused six
  responsibilities. Four are now their own: the journal projection behind the roster's status
  column (`roster_status.go`), the event→English copy for an agent's activity timeline
  (`roster_activity_text.go` — presentation, so a wording change no longer sits in the same file as
  a policy change), the read-only teardown preview (`roster_cascade.go`), and the teardown
  mutations that actually delete (`roster_teardown.go`).

  The preview also stopped being copy-paste. Each subsystem an agent teardown touches used to be
  four hand-written things — a lister, a sub-agent lister that differed only in which lister it
  called, and two payload keys wired into a forty-entry map literal — so adding one meant four
  edits with nothing checking them against each other. Now one `cascadeSubsystems` entry, and the
  eight character-identical sub-agent listers collapsed into a single fan-out. Kept separate from
  the removal path on purpose: two subsystems (workflow references, mailbox threads) are reported
  but deliberately never cleaned, because a workflow node or a message thread belongs to the
  workflow or the conversation rather than to the agent that names it.

  Guarded by pinning the `agent_impact` wire payload. That matters more than it sounds: the console
  defaults a missing key to an empty list, so a renamed key doesn't error — it silently empties one
  subsystem's row in the confirmation dialog an operator is reading to decide whether a removal is
  safe. Mutation-verified.

- **Refactor (Phase 3.4): a tool now declares its own policy axis.** New `ToolDef.Capability`,
  governance metadata like `Effect` so it stays off the provider wire. All 39 built-in tools
  declare; `edict.CapabilityForToolCall`'s name switch drops to a fallback for the surfaces no
  static declaration can cover — forged `forge_<name>` script tools and bridged
  `mcp_<server>_<tool>` calls, whose defs are synthesized per attachment. Adding a tool no longer
  requires an edit in the policy package, which is what let six tools ship dead (above).

  A capability isn't always a constant: eight tools pick their axis from an input field, so the
  declaration carries an optional field name plus a value→axis map. Choosing the fallback is a real
  decision, and the tool's author is the one placed to make it — `artifacts` falls back to its READ
  axis so a garbled call can't gain delete, `mcp` falls back to its INSTALL axis so a garbled call
  can't slip past the grant. `file` deliberately declares no fallback: an op outside its map is
  also outside its schema enum, so deferring leaves it on the unknown-capability path, which
  denies — the right answer for a call nobody can service.

  Resolution order is declaration → plugin capability manifest (M900, for tools whose def crossed
  a process boundary) → name switch. A declaration naming an axis Edict doesn't govern is
  **ignored**, not honoured, and resolution continues: honouring a typo would resolve to an unknown
  capability, i.e. default-deny, silently killing the tool — exactly the failure this field exists
  to end. Tested per-call, so one bogus value in a multi-axis map degrades that call to the
  fallback while its siblings keep working.

  Two parity guards make the migration verifiable rather than hopeful: for every real boot tool and
  every input its schema allows, and again over the runtime-registered set, what the tool declares
  must equal what the switch resolved before. Both held green through all 39 annotations.

- **Fixed: six tools were refused by policy on every call.** A tool's policy axis is declared in
  `kernel/edict`'s name switch — a different package from the tool — and nothing connected the
  two. A tool whose name never reached that switch resolved to a capability Edict doesn't govern,
  and an unknown capability is **default-denied**. So forgetting the edict edit didn't degrade a
  tool, it killed it: every call refused with `no trust level configured for "<tool>"`, on a daemon
  whose declared posture is allow-everything-unless-turned-off. Dead this way: **`conductor`**
  (the thinker/worker/verifier tool), **`market`** (the whole capability marketplace, for agents —
  the CLI and HTTP paths were fine, which is why the loop verified green), **`voice`**,
  **`image_generate`**, **`rerank`**, and **`file` with `op=glob`** (implemented and advertised in
  the tool's own schema, but the switch had no case for it). Each now rides a real axis: conductor
  on `code.exec`, voice/image/rerank on `provider.call`, glob on `file.list`, and the marketplace
  on a new `market.install` — its own axis rather than borrowing the MCP-specific one, since a pack
  can carry MCP servers *and* skills *and* host tools. Like every governed capability it ships at
  L4 (allow) per the max-autonomy posture.

  Two guard tests make the omission fail next to the tool registry instead of at run time: every
  boot tool must resolve to a governed capability *for every input its schema allows* (that clause
  is what catches an unmapped `op`), and the runtime's own tool set — which the registry package
  can't see, and which is where four of the six lived — gets the same check. Both were confirmed
  red against the old code.

  Worth noting for anyone auditing: `conductor` rides `code.exec` because its verifier runs the
  worker's code through an in-kernel call that never returns to the policy engine for a second
  decision. Gating the tool on the code-exec grant is what keeps "deny code.exec" from leaving a
  path that still executes code.

- **Fixed: a hot-swapped model or persona only reached some of the daemon.** `SetModel` (M816)
  and `SetSystem` (M710) exist so an operator can change the default model or identity without a
  restart — a provider reload after a key rotation calls the first, the persona surface calls the
  second. But six call sites read those fields off the boot config instead of the live one, so
  after such a change **delegated sub-agents, workflow LLM nodes, workflow drafting, and both
  memory-consolidation passes kept requesting the model the daemon started with**, and delegated
  runs kept appending the persona it started with. The usual reason to switch models is that the
  old one stopped being servable, which made this fail in the least legible way possible: chat
  worked, delegation and workflows failed on a model the operator thought they had replaced, and
  `KeyedModelChain`'s last-resort fallback fell back to the same dead model. All six now resolve
  through the run's effective config (below). Pinned by mutation-verified tests — both were
  confirmed red against the old code, one showing the lead on the new model and its child on the
  old one in the same run.

- **Refactor (Phase 3.3): one table for per-agent config overrides, one `effectiveConfig` per run.**
  A named agent can retune runtime knobs for its own runs via `ConfigOverrides`, and that surface
  was described in three separate places: a list of valid keys, a validation switch mapping each
  key to a value type, and a hand-written lookup at each point of use. Nothing tied them together,
  so a key could be advertised, pass the agent doctor, and then be silently ignored at run
  time — and the per-site lookups are what let the boot-vs-live drift above accumulate unnoticed.
  Replaced by a single `agentOverrides` table (key, the message the doctor shows, and one `Apply`
  that both validates and assigns), so validation and application cannot disagree about what a
  value means, and by `k.effectiveConfig(ctx)`, which returns the config a run actually sees:
  daemon-wide config, then the operator's live edits, then this agent's overrides. Consumers read
  resolved fields, so reading `k.cfg` inside a run is now the thing that looks wrong. Four
  single-key context wrappers deleted. Operator-visible behaviour is unchanged except the fixes
  above: doctor messages are byte-identical, and a malformed value is still reported and skipped
  rather than zeroing the knob (now covered by a test, along with sibling-typo isolation). The
  plan's other half — grouping `Config`'s field clusters into sub-structs — was dropped
  deliberately; the reasoning is recorded in `docs/REFACTORING-SCAN-2026-08.md`.

- **Refactor (Phase 3.2): `agent.Run` decomposed, 833 → 309 lines.** The agent loop was one
  function carrying twelve concerns, including the system's most safety-critical logic. Split into
  `run_setup.go` (the prologue as pure functions — config validate/normalize, the `task.received`
  provenance map, tool-schema linting, the cached elision summarizer), `run_provider.go`
  (`callProvider` collapses the streaming and non-streaming branches to their one real difference —
  when the reasoning text is known — so the nil-response contract check, error wrapping, and
  ephemeral `llm.token`/`llm.reasoning` publishing are shared by construction rather than by two
  copies staying in sync), and `run_tools.go` (the three tool phases as methods on a new `runState`).
  `runState` owns the prompt-injection causal window, which is a two-sided invariant — finalize
  records *when* a directive-like untrusted observation arrived, gate decides whether a proposed
  action is still inside the window — and reviewing it used to mean reading 300 interleaved lines;
  `directiveActive` is now four lines with a unit test. It deliberately does **not** hold the
  conversation: the loop appends to that from four places and compaction rewrites it wholesale, so a
  second copy would silently drop turns; `finalizeToolJobs` takes it and returns it, pinned by a
  test. New internal tests cover what previously needed a whole run to reach (causal-window decay,
  the M605 denial ladder, loop-guard refusal, refusals short-circuiting execution,
  tool-error-vs-panic classification). Behaviour held constant: kernel + plugins + cmd suites green,
  `kernel/agent` coverage 79.5% → 80.5%.

- **Console: the trust layer.** Alarm surfaces were lying about time and currency. Alerts and the
  dashboard's "Needs attention" now carry day-aware timestamps (`fmtWhen`: bare clock today,
  "yesterday HH:MM", date beyond) and alerts are dismissable (persisted per browser, with a
  show/restore toggle). A journal-backfilled "daemon halted" with no later resume event haunted the
  cockpit after every restart — a restart clears the kernel flag without journaling a resume — so
  the attention helpers now accept the live `halted` flag from `/api/status`, which wins over event
  archaeology. Provider/model fallback status reports `last_ms`; Health anchors its failover message
  to *when* it last happened and downgrades to info past 24h, so yesterday's failover storm no
  longer reads as a live incident. Success/error-rate cards on Dashboard, Health, and Insights label
  their windows, so the three pages stop appearing to contradict each other.

- **Fix: `MetricGrid`'s Tailwind-class `cols` were applied as an invalid inline
  `grid-template-columns` value,** which the browser dropped — silently collapsing the Activity and
  Insights metric rows to one full-width card each. The class form now routes to `className`
  (regression-tested).

- **Fix: the web console forwarded every query argument as a string,** but the control plane's typed
  accessors (`argLimit`/`argFloat64`/`scheduleArgNumber`) reject string numbers. Memory's list
  hard-failed with `502 args.limit must be a number`, Overseer showed "0/0 agents" because its
  `/api/agents?limit=200` call was rejected, and every other `limit`/`since_ms`/`count` query arg
  silently fell back to its default (so "load more" page sizes never applied). Known numeric keys
  are now coerced at the proxy; an unparseable value still rides through as a string so the server's
  error names the real problem.

- **Console: humane run titles.** Chat-transcript intents ("User: … Assistant: … User: …") now title
  by their newest user message and composed prompts ("== QUESTION ==") by the actual ask, in Runs,
  Activity, Replay, and the Agents run cards — the lists used to open with
  "You are AGEZT's observability analyst, embedded in a running agent operating system…". Search
  still matches the raw intent; the full text stays in the hover title.

- **Console declutter sweep.** Policy's 36 identical `L4` rows read as one sentence
  ("all 36 capabilities at L4 · allow") with the grid behind a disclosure, and a mixed posture leads
  with the exceptions. Catalog cards show a tool's first sentence with the full doc folded. Roster
  guardian cards dropped the remedy sentence each of them repeated (it lives in the roll-up panel
  that owns the quiet action). Board chips stopped saying the same thing three times, and
  "All agents ()" lost its empty parens. Autonomy folds consecutive same-shaped events (the 16
  built-in skills promoted at boot) into one row with a ×N badge.

- **Console: pages that carry information at rest.** Mission Control's rolling rates were all zeros
  when idle — it now also shows active runs (click-through to Runs) and a recent-activity panel of
  notable events. Jarvis ended at the pillar row, leaving half a screen empty: the initiative feed is
  permanent and explains *why* it is empty in terms of the switch that governs it (off vs paused vs
  disarmed vs armed-quiet), and the distilled operator profile gets a full panel. Council gained a
  "Past convenings" strip — every deliberation is journaled but was unreachable once the live stream
  moved on, and re-opening one folds its events through the same reducer as the live path, so it
  renders identically; a single-seat council now says it is a monologue instead of quietly degrading.

- **Console: connect-a-channel wizard.** A new guided flow leads with the five channels operators
  wire first, searches the full ~34-channel tail, and drills into the existing Channels connect form
  (reused, not reimplemented) — completing the journey the Inbox empty state now starts with a
  "Connect a channel" button. The Channels page itself orders live/configured first, then the popular
  five, and offers a start-here hero when nothing is connected. Models sorts keyed providers first,
  so the one provider you can actually use is not buried among ~180 catalog entries.

- **Fix: ChatGPT/Codex provider rejected every request that offered a dotted tool name.** The
  `openairesponses` adapter sent `agent.ToolDef` names verbatim, so `browser.read` / `browser.action`
  drew `400 "Invalid 'tools[N].name': string does not match pattern '^[a-zA-Z0-9_-]+$'"` — the whole
  request failed, and with ChatGPT as the only arm the run died with `all providers failed`. Every other
  tool-calling adapter (OpenAI, Anthropic, Bedrock, Cohere, Google, Vertex) already routes names through
  `plugins/providers/internal/toolname`; this one never adopted it. Now it does: `toolname.Maps` on
  encode for both the `tools` array and replayed `function_call` items, `toolname.RestoreCalls` on the
  response so a `tool_call` still routes to the real tool. Verified live: a request offering
  `browser.read` + `browser.action` is accepted and the model's call comes back as `browser.read`.

- **Fix: "Sign in with ChatGPT" served a frozen, dead model list.** The provider's models were a
  constant written when the adapter shipped (`gpt-5-codex`, `gpt-5`, `gpt-5-mini`) and `SeedChatGPTCatalog`
  wrote the catalog entry exactly once, so a signed-in install kept offering ids the backend had
  retired — the default `gpt-5-codex` now answers `400 "The 'gpt-5-codex' model is not supported when
  using Codex with a ChatGPT account."` (verified live), which the governor reads as unservable, taking
  the whole provider down. Model ids are now **discovered**:
  - `openairesponses.ListModels` calls `GET /backend-api/codex/models?client_version=…` — the same call
    Codex CLI makes (the `client_version` query param is required; omitting it is a 400) — with the same
    401 refresh-and-retry as `Complete`, and orders the reply by the backend's own priority.
  - `providerboot` resolves the surface from the backend, else the Codex CLI's `models_cache.json`, else
    a builtin snapshot; hidden (`visibility: "hide"`) entries stay out of the picker; results are memoized
    for 6h so reloads and sign-in status polls don't mean a request per call. A discovery failure degrades
    to the next source and never blocks boot.
  - `SeedChatGPTCatalog` now **refreshes** an existing entry when the set is authoritative (a builtin/offline
    set still never clobbers what's on disk), and a sign-in triggers a catalog refresh via the new
    `controlplane.Deps.ChatGPTSync` hook — the kernel still never imports the provider layer.
  - Per-model prompts: the backend now serves distinct `base_instructions` per model, so the adapter sends
    each model its own and falls back to the vendored `instructions.md` only for models discovery missed.
  - The sign-in responses carry `models` + `default_model`, so Setup pins a live model instead of the
    hardcoded `gpt-5-codex`, and `agt provider chatgpt status` prints the served list.
  - Verified live against a real subscription in an isolated home: 7 models discovered (default
    `gpt-5.6-sol`, 17.7 KB of per-model instructions), and a completion on the discovered default returns
    normally where `gpt-5-codex` 400s.

- **Fix frontend `lib/language.ts` regressions introduced during the C2 P2 `lib/languages.ts → lib/language.ts` rename.** The rename collapsed three behavioural guarantees that downstream tests (`languages.test.ts`) and consumers (`markdown.ts` inline file-mention parser, `FileMention.tsx`) depended on:
  - `extOf(".gitignore")` returned `"gitignore"` — restored to `""` (a leading dotfile has no extension).
  - `extOf("foo.dir/bar")` returned `"dir"` — restored to `""` (when the last `/` comes after the last `.`, the segment is a directory, not an extension).
  - `fileMentionRegex()` greedily consumed surrounding characters, producing tokens like `"see notes/x.md"` (whitespace) or `"(notes/x.md"` (punctuation); URLs like `https://example.com/x.md` matched the path portion after `://`. Restored the lookbehind/lookahead regex (`(?<=^|\s|[\(\["'])…(?=$|\s|[\)\]"'. ,;:!?])`) so the match is the exact path and the inline file-mention pipeline in `lib/markdown.ts` emits clean `{ t: "file", v: "notes/x.md" }` tokens.
  - Memoised the compiled regex (previous implementation rebuilt it on every call).
  - Updated the now-orphaned `lib/languages.test.ts` import to `./language` so `tsc --noEmit` is clean.
  - Verification: `tsc --noEmit` clean, `vitest run` 177 files / 1461 tests pass (including `languages.test.ts` 9/9, `markdown.test.ts` 25/25, `FileMention.test.tsx` 3/3), `go build ./...` and `go vet ./...` clean (frontend-only change).

- **A daily workflow trigger could be saved, shown, and never fire.** `dailyAtRe` accepts a
  one-digit hour (`9:05`), but the trigger runner decided "has today's time arrived?" by comparing
  that stored value **as a string** against `now.Format("15:04")`, which is always two-digit. Every
  possible clock value begins with `0`, `1` or `2`, and all three sort below `9` (`0x39`), so
  `"9:05"` read as "not yet due" for all 1440 minutes of the day.

  The failure was total and silent: `Save` accepted the config, `Validate` passed it, the console
  rendered "daily at 9:05", and the workflow never ran — no error, no journal record, nothing to
  point at. Any one-digit hour was affected; the zero-padded form operators copied from the field's
  `09:00` placeholder was always fine, which is why the defect survived review.

  Fixed at the single parse chokepoint rather than at the call site: `canonicalDailyAt` pads the
  hour in `TriggerSpec`, so the runner and the console detail view compare like with like. This
  also repairs rows already on disk — they are normalized on read, so no store migration is needed.
  `Validate` deliberately still accepts `9:05`: tightening the regex would have turned a
  previously-valid saved config into an error, and would have left existing broken rows broken.

  Proof: a full-day sweep through the real `Store.Save` → `StartTriggers` → `onTick` path, with a
  zero-padded control in the same store so a harness fault could not masquerade as the bug. Pre-fix
  the control fired once and `9:05` fired 0 times (exit 1); post-fix both fire once and the fire
  reason reads `cron daily 09:05`. Promoted as `TestCanonicalDailyAt`,
  `TestTriggerSpec_NormalizesDailyAt` and `TestCronTrigger_UnpaddedDailyAtFires` (which also pins the
  09:04 boundary, so padding cannot degrade into "always due").

- **The expense view's "This month" total could silently ignore a valid date.** A datalake `date` is free text. Verified on every write path: `Lake.Insert` documents "Fields are stored verbatim", `Lake.Update` merges the patch as-is, `handleDataInsert` (kernel/controlplane/datalake.go) forwards the raw JSON map, the agent's `db` tool (plugins/tools/db) forwards `in.Record`, and the console editor's own `coerce()` handles `number`/`money`/`bool`/`tags` but falls through to `return s` for a `date` field — there is no `type="date"` input in the frontend at all. So `"2026-9-5"` is a legal stored value from an agent or a keyboard, while `ExpenseView` assumed the canonical `YYYY-MM-DD` width.

  Two consequences, both silent. `String(date).startsWith("2026-09")` is false for `"2026-9-20"`, so that row vanished from the **money total** the owner reads on the dashboard. And the "recent" list used `localeCompare` on the raw string, where `"2026-9-5" > "2026-10-01"` because `'9'` (0x39) beats `'1'` at index 5 — September outranked October.

  Same root cause as the workflow `daily_at` defect fixed this round: a value compared as a string whose format was never canonicalized. `lib/datalakedate.ts` now holds the rule — `dayKey` pads to fixed-width `YYYY-MM-DD` and returns `""` for anything without an unambiguous calendar date, `monthKey` is its `YYYY-MM` prefix. Equal width plus zero padding is what makes lexicographic order equal chronological order; `""` is the lowest key, so an unreadable date sinks to the end of the list and matches no month rather than being guessed into one. Stored values are still displayed as written — only the comparison changed.

  Proof: both symptoms fail against the unfixed view — `'This month10,00' to contain '15,00'` and the ordering assertion returning `0`. Assertions compare money through the component's own `toLocaleString` options, because this machine's ICU renders 15 as `15,00` and a literal `"15.00"` would have tested the locale, not the code. Pinned as `datalakedate.test.ts` (16 cases, including a totality check that any key it returns is fixed-width, and one pinning the documented scope that month/day are range-checked but not calendar-validated per month) plus two `<Data/>` render tests. The same comparison survived in `CalendarView`, which the follow-on entry below closes.

  Both date suites pin the clock (`vi.useFakeTimers({ toFake: ["Date"] })` + `vi.setSystemTime`) with fixed literals rather than deriving fixtures from the wall clock. The first version derived the non-canonical spelling from the real date, which yields `2026-10-20` in October: that satisfies `startsWith("2026-10")`, so the month assertion would have PASSED against unfixed code purely depending on the day CI ran — verified by evaluating the old expression across months 09–12. A fixture that only bites on some dates proves nothing. Only `Date` is faked, so `findByText` still polls real timers.

- **The calendar agenda could file a past event as Upcoming.** `CalendarView` made three comparisons against the raw stored `date` string: an ascending `localeCompare` for the agenda order, plus `>= today` and `< today` to split Upcoming from Past. With `date` free text (see the preceding entry), `"2026-9-20" >= "2026-10-05"` is true because `'9'` (0x39) beats `'1'` at index 5 — so an event ten days in the past was listed as upcoming, and `"2026-10-20"` sorted above `"2026-10-9"`, putting the 20th before the 9th.

  Now every date in the view is routed through the shared `dayKey` once (`const day = (r) => dayKey(r.fields?.date)`) and all three comparisons use it, so the split and the ordering are chronological rather than textual. Reusing `lib/datalakedate` rather than re-deriving a parser here is the point: two views, one rule, no way for them to drift.

  One deliberate consequence: a date that cannot be read keys to `""`, which is below every real day, so it now lands in Past. Previously a garbage value like `"xyz"` outranked today's date ('x' > '2') and appeared in the upcoming list — an unreadable event is not evidence of something scheduled, so sinking it is the safer side.

  Proof: two failing tests captured before the change — `expected 2 to be 1` for the Upcoming count, and the soonest-first ordering returning `0`. Both are locale-proof: the counts come from the view's own `Upcoming · N` / `Past · N` headers, which render a plain integer, and no assertion touches `toLocaleString`. The clock is pinned with `vi.useFakeTimers({ toFake: ["Date"] })` + `vi.setSystemTime`, because fixtures derived from the *real* date only discriminate on some days — a test that passes pre-fix depending on when CI runs proves nothing. Only `Date` is faked, so `findByText` still polls real timers. A third test in the same suite pins the sink-to-Past behaviour described above, so the deliberate change is enforced rather than merely asserted.

- **"This month" and the agenda split moved when the operator crossed a timezone.** Both bespoke data views derived "now" with `new Date().toISOString()`, which reports **UTC**, and then compared it against a stored `date` — a bare LOCAL calendar day carrying no zone. Within a day of a month edge the two frames name different months, so the answer depended on the operator's offset rather than on the data.

  The two views failed in opposite directions, which is why this is not a rounding detail. East of UTC (e.g. UTC+14) UTC still says September after the operator's October has begun, so the dashboard's "This month" money total dropped the entire current month. West of UTC the reverse: UTC already says October while the operator is still in September, so a future month's spend was reported as already spent, and the operator's OWN today sorted below `today` — today's events vanished from the agenda into Past. A fixed offset would correct exactly one of the two.

  `lib/datalakedate` gained `localDayKey` / `localMonthKey`, which read `getFullYear` / `getMonth` / `getDate` — the local frame the stored values are written in, and already the repo's idiom for a wall date (`unixToLocalInput` in `views/Schedules.tsx`). An invalid `Date` yields `""`, matching `dayKey`'s lowest-key contract. Both views call these once instead of inlining `toISOString()`, so the frame is decided in exactly one place.

  Proof: four tests fix ONE absolute instant and vary only `process.env.TZ`, with `Pacific/Kiritimati` (UTC+14) and `America/New_York` (UTC-4) chosen precisely because they straddle the boundary in opposite directions. Every stored date in them is already canonical `YYYY-MM-DD`, so padding cannot account for a failure and the timezone is the only axis under test. Pre-fix deltas were `expected 'This month5,00' to contain '40,00'`, `expected 'This month40,00' to contain '5,00'`, and Upcoming counts of `2` and `0` where `1` was correct. A date-independent `Total` assertion passed in all four cases, which is what rules out a render or harness fault rather than the bug. `process.env.TZ` was first verified to take effect mid-process on this platform (offsets `+840` / `-240` observed in a single run); each suite restores it, because TZ is process-wide state and `= undefined` would leave Node reading the literal string `"undefined"`.

- **Non-canonical dates are stopped at the source, on both write seams.** The three entries above made every consumer tolerant of any spelling; this one removes the need to be. A `date` field is free text on every write path — the console editor's `coerce()` handled number/money/bool/tags but fell through to raw text for `date`, and the kernel stored fields verbatim — so `"2026-9-5"` could still be written by an operator's freehand text or by an agent, leaving every future consumer to pay the canonicalization tax.

  The kernel is the authoritative seam: `handleDataInsert` (kernel/controlplane) and the agent's `db` tool (plugins/tools/db) both call `Lake.Insert`/`Lake.Update` directly, so a UI-only fix would have left agent-written rows non-canonical. `canonicalizeDateFields` normalizes the string values of fields the schema DECLARES as `date` — `"2026-9-5"` and `"2026/9/5"` become `"2026-09-05"`. Declaration is the trigger because the schema is explicitly advisory and records may carry extra keys: something that merely looks like a date in a text field is the operator's own content. The rule is deliberately stricter than the read-side `dayKey`: `dayKey` may truncate a timestamp to its day because it only feeds a comparison, but rewriting stored data that way would destroy the time the operator typed, so only a value that is EXACTLY one whole date is touched. Out-of-range and unparseable values are stored as written — never guessed at, and never rejected, so no previously-accepted write becomes an error. It is copy-on-write: `Insert` has always aliased the caller's map into the stored Record, and rewriting it in place would have mutated the caller's data as a side effect (pinned by test). `Update` normalizes only the patch, so an unrelated edit never quietly rewrites a stored value.

  The console editor's `coerce()` gained the same branch via `lib/datalakedate.canonicalDate`, so a freehand date leaves the UI already canonical and the optimistic row never shows the raw spelling. The two implementations mirror each other and their fixture tables are identical case for case (`datalakedate.test.ts` and `datewrite_test.go`), so they cannot drift silently across the language seam.

  Proof, kernel first: `datewrite_test.go` failed pre-fix with `stored date = 2026-9-5, want 2026-09-05`, `re-read date = 2026-9-5` (the raw form is what persisted to disk), and `patched date = 2026-10-2, want 2026-10-02`; four tests pass after, including the guards that an unrelated update does not rewrite a stored value, a schema-less collection stays verbatim, and a numeric value in a date field survives. Frontend: the editor-submit test drove the real modal and failed pre-fix with `expected to be called with ['/api/data/insert', …]` (Number of calls: 1 — the submit happened, with the raw spelling), plus two `canonicalDate` unit cases that failed on the missing export; all pass after. Two harness faults were caught and fixed on the way rather than trusted: the field label renders the name PLUS a type badge ("date date"), so an exact `getByLabelText` query missed it, and the new `coerce` branch initially called `canonicalDate` without importing it — surfaced by the test as a `ReferenceError` and confirmed by `tsc` as exactly the class of defect typecheck catches.

- **"Update available" meant merely "different", so a rolled-back catalogue offered a downgrade as an update.** `Manager.List` set `UpdateAvailable` from string inequality between the installed and catalogued versions. Inequality carries no ordering, so whenever the catalogue held an OLDER version than the installed one — a marketplace that rolled back, or shipped a prerelease of a release already installed — the listing advertised an update, and an operator following the badge would silently lose code. The field's own doc comment ("true when installed at a different version than catalogued") was the bug stated as a contract.

  There was no version comparator anywhere in the Go tree, and the market's own `semverRe` admits exactly the inputs where a string compare lies: multi-digit components (`"1.10.0"` sorts below `"1.9.0"`, since `'1' < '9'`) and prereleases (`"2.0.0-rc.1"` does not sort below `"2.0.0"` at all).

  `kernel/market/version.go` adds `compareVersions` with semver precedence: the numeric core first, then prerelease where absence beats presence, dot-separated identifiers compare numerically when both are numeric, numeric identifiers rank below alphanumeric ones, and a longer identifier list is higher when every shared identifier matches. Build metadata is ignored per spec. A version that does not parse falls back to plain string order rather than an error, because versions also arrive from `installed.json`, which an operator can hand-edit, and a browse listing must never fail — or panic — on malformed provenance. `List` now requires the catalogued version to be strictly newer.

  Proof: `TestListUpdateAvailableRequiresStrictlyNewer` drives the real `List` path over a stub `Library` (that interface exists precisely so catalogue sources plug in without market importing them) and failed pre-fix on exactly the ordering cases — `installed "2.0.0" vs catalog "1.9.0" (catalogue rolled back): UpdateAvailable = true, want false`, and identically for `"2.0.0" vs "2.0.0-rc.1"` and `"1.0.0" vs "1.0.0-beta.2"` — while the equal and genuinely-newer rows passed, which is what pins the failure to the ordering semantics rather than the harness. The comparator itself is pinned by the spec's own precedence chain (`1.0.0-alpha < 1.0.0-alpha.1 < 1.0.0-alpha.beta < 1.0.0-beta < 1.0.0-beta.2 < 1.0.0-beta.11 < 1.0.0-rc.1 < 1.0.0`), asserted in both argument orders, plus multi-digit cores and a malformed-input table that must never panic. Gates: the focused market suite is green with no regression to the existing install/uninstall/listing tests, and a full `go test ./...` run (sequentially, not concurrently with the frontend suite) is green across every package.

- **`market install` could silently downgrade an installed pack.** `Manager.Install` never consulted the existing install record: it resolved the pack, materialized it, then `RecordInstall` upserted by name — so installing an older pack overwrote provenance with no error anywhere, and `List`, which orders versions, would afterwards report nothing to update either. The agent tool always installs the catalogued version with no version argument at all, so a marketplace that rolled back would downgrade every agent-initiated install invisibly.

  `Install` now refuses a strictly-older version, reusing `compareVersions` — the one ordering rule — and its error names both versions and the supported path ("uninstall first", because Uninstall reverses the installed footprint before a fresh install records its own). The guard runs after `Validate` and before signature verification, vetting and materialization, so a refusal leaves no partial footprint and streams no misleading progress events. Re-installing the SAME version still works: "Idempotent: re-installing updates the record" is a documented contract, and a guard that over-refused would have silently broken it.

  Proof: three tests over the package's own fakes (`installguard_test.go`), sharing one store while the catalogue serves different versions of the same pack — the real `Install` path, no shortcuts. Pre-fix: `installing 1.9.0 over 2.0.0 succeeded: silent downgrade`, and `catalogue rollback to a previously-installed version succeeded` for the update-then-rollback sequence; the control test asserting same-version re-install and genuine updates succeed PASSED pre-fix, which is what makes it a guard against over-refusal rather than a third copy of the same assertion. Post-fix all three pass, together with assertions that the install record still names the newer version and that no second skill or MCP server was materialized. Gates: the full market suite is green, `go vet` and gofmt clean, and a full `go test ./...` run is green across every package — including the control-plane and CLI packages whose handlers call Install.

- **An unqualified pack resolution picked a marketplace alphabetically, not by version.** `compositeLibrary.ResolvePack` with no marketplace qualifier walked the synced marketplaces in the order `CachedMarketplaces` returns them — which is sorted by marketplace NAME — and returned the first hit. When two remotes carried the same pack, which version an operator or agent received was decided by the marketplaces' names: a marketplace sorting earlier held the clash even while carrying the older pack. The agent tool installs with no marketplace qualifier at all, so for a pack published by several sources the version was an alphabetical accident.

  Resolution now picks the NEWEST version across synced marketplaces, reusing `compareVersions`. Two policies are preserved and pinned by test: the built-in Official seed still wins any name clash ("a remote can't shadow Official"), even when a remote carries a strictly newer version — newest-wins deliberately does not reach past that pre-existing policy — and a QUALIFIED marketplace still returns exactly that marketplace's pack even when another carries a newer one, because qualifying is an explicit choice of source. Ties keep the alphabetically-first marketplace, so the choice stays deterministic.

  Proof: pre-fix exactly one test failed — `unqualified resolve with older pack in the alphabetically-first marketplace: got version 1.9.0, want 2.0.0` — while the mirror case (newer pack in the alphabetically-first marketplace) and both policy pins PASSED pre-fix, which is what makes them controls rather than restatements of the bug. The fixture drives the real sync path: two httptest marketplaces synced into one store under source names chosen so that iteration order and version order disagree. One fixture fault was caught before being trusted: the shadow-policy test initially synced a pack the builtin did not carry, so the builtin "won" by being the only source — vacuous. Both remotes now carry the builtin's own pack at a strictly newer version, so the policy is genuinely exercised. Gates: the full market suite is green, `go vet` and gofmt clean, and a full `go test ./...` run is green across every package.

  Known limitation, flagged rather than fixed: an explicit `version` argument for a REMOTE pack is still not honored — it was ignored before this change too (resolution was by marketplace name order); it now resolves deterministically to the newest. Honoring it needs a version-aware cache lookup and is a separate change — closed by the follow-on entry below.

- **An explicit version request for a remote pack is now honored; previously both remote branches silently ignored it.** `ResolvePack` dropped its `version` argument twice: unqualified resolution returned the newest version regardless of what was asked, and a qualified marketplace returned whatever it happened to have cached. An operator asking to install a pack at 1.9.0 could silently materialize 2.0.0 — the requested version never reached the resolution at all, so the install record and the skills that materialized were for a version nobody asked for.

  Both branches now honor the argument through `compareVersions` (so `+build` metadata is precedence-neutral, as everywhere else): an exact match is returned; anything else is an ERROR naming what was asked and what actually exists — `pack "x" not found at version 3.0.0 (cached: aaa-market@1.9.0, zzz-market@2.0.0)` unqualified, and `acme caches "x" at 1.9.0, not 2.0.0` when qualified — never a silent substitute. Resolution without a version is unchanged: newest across synced marketplaces, builtin Official still shadowing every clash.

  Proof: three tests over the same real-sync fixture (`libraryversion_test.go`, two httptest marketplaces synced into one store). Pre-fix: `asked for 1.9.0: got 2.0.0`, `asked for 2.0.0-rc.1: got 2.0.0` (a prerelease is not its release), `asked for 1.9.0: got 1.10.0` (multi-digit, where string order also lies), `asked for absent version 3.0.0, silently resolved 2.0.0`, and `qualified resolve silently returned a version other than the one requested` — while the row asking for the NEWEST passed pre-fix, the control that pins honor-or-error rather than blanket refusal. Post-fix every case passes, including assertions that the absent-version error names the requested and both cached versions so an operator can act. Gates: the full market suite is green, `go vet` and gofmt clean, deadcodecheck clean, and a full `go test ./...` run is green across every package — including the control-plane and CLI packages that pass `version` through from operator arguments.

- **The builtin Official catalogue silently ignored an explicit version request too — and it ran FIRST, so it shadowed the fix above.** `builtinmarket.ResolvePack` discarded its version parameter outright (`func (l *Library) ResolvePack(_, name, _ string)`): the builtin index is name → one pack and every builtin carries `1.0.0`, so asking for any other version silently returned 1.0.0. Worse, the composite library consults the builtin before the synced marketplaces, so a version a remote genuinely carries would have been silently substituted by the builtin's 1.0.0 before the version-honoring remote branches could run at all.

  `ResolvePack` now treats an explicit version as an exact request — honored when Official carries it, otherwise an error naming what Official actually has (`Official carries "x" at 1.0.0, not 2.0.0`), never a silent substitute. The refusal composes correctly: an unqualified composite lookup swallows a builtin miss and continues to the synced marketplaces, so a version Official lacks now resolves from a marketplace that has it, while a lookup qualified as `official` returns the error. To compare from another package without forking the rule, the comparator is exported as `market.CompareVersions` (17 call sites renamed — one rule, one name, no second implementation).

  Proof: `TestResolvePackHonorsExplicitVersion` over the real `New()` catalogue failed pre-fix with `asked "browseruse-pack" at 9.9.9, silently resolved 1.0.0`, while the controls — no version requested, and the catalogue's own version — passed before it. Post-fix the refusal's error names both the requested and the catalogue's version, so an operator can act. Gates: both touched packages green (`kernel/market`, `plugins/builtinmarket`), `go vet` clean, gofmt clean across all six changed files, deadcodecheck clean (the newly exported symbol is used cross-package, not dead), and a full `go test ./...` run is green across every package — including the control-plane and CLI packages that pass `version` through.

- **The generated changelog indexes would have listed an m1000+ bucket between m100-m199 and m200-m299.** `tools/changelog-split` ordered the bucket keys in both generated indexes — the README "Layout" listing and the reorg log's "Unreleased slices" — with byte sort. Bucket names are fixed-width only while every milestone is three digits: once the tree carries an m1000+ bucket, byte order places it after m100-m199 (`'-'` < `'0'` at index 4) but before m200-m299 (`'1'` < `'2'`), so an operator scanning either index for where recent work landed would find the newest slice buried in the middle of the list. The tree does not carry that bucket yet; it is one purely-four-digit subsection in the working set away from it.

  Both sorts now go through `bucketLess`, which orders keys by the milestone `bucketFor` itself names (parsing the leading number), with byte order as the deterministic tie-break for the two m600 buckets that share a starting hundred. Non-milestone keys (`current`) keep sorting first, as before. Because every bucket on disk today is three-digit, numeric and byte order agree on the current set — the emitted files do not change a byte, so emit idempotency over the existing tree is untouched, and the fix bites exactly when the four-digit bucket appears.

  Proof: two tests call the real renderers (`bucketorder_test.go` — pure functions taking the bucket map) with a fixture carrying every on-disk bucket plus m1000+. Pre-fix both failed with the predicted sequence (`[m100-m199 m1000+ m200-m299 …]`, wanting m1000+ last); post-fix both pass with the exact expected order. The idempotency and loss-gate suites still pass because the loss gate indexes generated documents as a set of lines, so reordering lines within a generated file cannot orphan content. Gates: the full changelog-split package (47 tests) green, `go vet` and gofmt clean, deadcodecheck clean, and a full `go test ./...` run green across every package.

- **A Library implementation that ignored an explicit version argument could silently substitute its own.** The market `Library` interface makes an explicit version an exact request, and both production Libraries honor it — but an interface cannot force a third-party implementation to. `compositeLibrary.ResolvePack` returned whatever the builtin seed handed back whenever it came with `err == nil`, so a contract-violating Library answered a request it did not satisfy with its own version, and `Install` would have materialized that unasked-for version.

  The composite now verifies what it was handed via `market.CompareVersions`: the builtin's answer is returned only when it carries the requested version (or no version was requested). A mismatch is treated as the miss it should have been — for an `official`-qualified lookup that is an error naming the returned and the requested version; for an unqualified lookup resolution falls through to the synced marketplaces, exactly as it does when a contract-honoring builtin answers "not at that version", so a remote genuinely carrying the version still serves the operator. The builtin shadow policy for unversioned lookups is untouched.

  Proof: two tests (`librarydefensive_test.go`) drive the composite with `fakeLib`, the package's existing one-pack Library stub, which by construction ignores the version argument — precisely the contract violation being guarded against. Pre-fix both failed on the substitution (`version-ignoring builtin silently substituted 1.0.0 for requested 9.9.9`; `builtin's 1.0.0 shadowed a marketplace carrying the requested 9.9.9`) while the no-version and own-version controls passed; post-fix the qualified case errors naming both versions, the unqualified case resolves the remote's 9.9.9, and with no remote carrying it the request still fails loudly instead of falling back. Gates: the full market package green, `go vet` and gofmt clean, deadcodecheck clean, and a full `go test ./...` run green across every package.

### Added — positioning, security, and SDK parity documentation

- **`docs/COMPARISON.md`** — positions AGEZT against generic agent frameworks without unverifiable
  competitor claims: durable identity, governance as runtime enforcement, typed schedules,
  auditable wake causality, and plugin trust. Includes a related-documentation cross-reference
  table and a priority roadmap. Linked from the README top status block.

- **`docs/THREAT-MODEL.md`** — T1–T10 threats with repo-grounded controls and explicit limitations:
  prompt injection, tool misuse, process/code-exec isolation (with Windows/macOS caveats), secret
  exposure, control-plane/API tokens (including query-string and tunnel caveats), inbound channel
  abuse, plugin/marketplace compromise, tenant boundary, network egress/SSRF, and workspace escape.
  Includes a trust-boundary diagram and an operator deployment checklist.

- **`docs/OPERATIONS.md`** — day-2 operations guide: health/readiness probes, metrics, cost
  management, policy/governance triage, event audit/forensics, backup/restore with a drill
  runbook, vault management, halt/resume/shutdown, live monitoring, five incident triage runbooks,
  and a monitoring checklist.

- **`docs/PLUGIN-SECURITY.md`** — P1–P8 plugin trust model: BLAKE3-256 binary pinning, process
  isolation + crash recovery, tool allowlists, frame/callback/tool-count bounds, host/invoke
  governance, registry install verification, MCP bridge security, and environment/secret isolation.

- **`docs/API-STABILITY.md`** — public/private surface stability matrix, versioning policy for
  REST/OpenAI/control-plane/plugin/SDK surfaces, and SDK parity rules. Links to the generated
  parity report and a release checklist for API changes.

- **`docs/SDK-PARITY.md`** (generated by `tools/sdkparity`) — static `/api/v1` route coverage
  matrix across Go, Python, TypeScript, and Rust SDKs. CI checks staleness via
  `go run ./tools/sdkparity -check docs/SDK-PARITY.md`.

- **`docs/index.md`** — documentation index linking all positioning, security, operations, API,
  SDK, and demo docs. Linked from README.

- **Four runnable positioning demos** under `examples/autonomous/`:
  - `policy-denial-audit/` — governance is runtime-enforced and auditable.
  - `mailbox-delegation/` — durable identity, authority, wake causality, agent hierarchy.
  - `typed-schedule-system-task/` — typed schedules, not cron-wrapped prompts; prompt-smuggling
    resistance.
  - `plugin-governance/` — governed tools, plugin pin hashing, audit surfaces.

- **`agt agent authority <slug> [--json]`** — effective runtime policy proof: merges agent profile
  with live Edict policy snapshot into a single view (tool allow/deny, trust ceiling, capability
  levels with ceiling-cap annotations, hard-deny floor, approval mode, memory scope, config
  access). Client-side; no control-plane protocol change.

- **High-risk approval visibility in agent diagnostics.** The approval log (`/api/v1/approvals_log`
  and `agt approvals log`) now includes `actor` and `correlation_id` per row, so approvals are
  attributable to specific agents. `agt agent show <slug>` surfaces a per-agent approval summary
  (total, pending, granted, denied, timeout, last status). The Web UI Diagnostics tab renders a
  per-agent human-approval list alongside existing policy denials.

- **SDK parity CI check.** `tools/sdkparity` extracts `/api/v1` routes from the REST handler and
  checks route-string presence across all four SDK source trees. CI fails if the report is stale.

- **Dependency/docs alignment.** `README.md`, `DEPENDENCIES.md`, `Makefile`, `docs/ARCHITECTURE.md`,
  and `docs/ARCHITECTURAL-REPORT.md` updated to match current `go.mod`, `frontend/package.json`, and
  `frontend/.nvmrc`. `tools/depscheck/allowlist.txt` expanded to cover the full resolved module
  graph.

- **OAuth connect flow for Slack and Mastodon channels** — `frontend/src/views/Channels.tsx` adds
  OAuth client id/secret entry and start/poll flow; `plugins/builtinchannels/builtinchannels.go`
  switches Slack from `token` to `oauth` connect method and adds Mastodon OAuth setup steps.

- **`agt skill hygiene [--idle-days N] [--json]`** — surfaces idle/unused skills (epistemic
  hygiene) from the existing `CmdSkillHygiene` control-plane command: total, active, idle counts
  plus per-skill name, use count, and last-used timestamp. Operators can now see which skills are
  collecting dust before they mislead the agent.

- **`agt world audit [--json]`** — world-model health summary: entity/relation counts, untyped
  entities, decayed entities (30-day staleness), superseded entities, and a kind distribution
  table. Client-side aggregation over `CmdWorldList` so no new protocol command was needed.

- **SDK behavioral parity tests closed.** Python and TypeScript SDK tests now assert all four
  `health` fields (`status`, `version`, `default_model`, `model_count`) and the `getRun`
  `correlation_id` field, matching what Rust already asserted. All three SDKs now have identical
  behavioral coverage across 20 dimensions: health, models, runs (sync/stream/failure), get_run,
  auth, tenant header, and the full mailbox surface.

### Fixed
- **`overseer op=repair` could target a protected system guardian.** `op=edit`
  has refused System agents for exactly this reason, but `op=repair` did not — and
  a repair *runs* the target against a brief whose resolution can rewrite that
  agent's own `Soul`, which boot reconcile does not re-clamp. So an arbitrary
  agent could aim a repair at a guardian and behaviourally defang the fleet that
  supervises it, going around the `op=edit` guard rather than through it.
  Auto-repair already excluded System agents; the agent-reachable tool path now
  matches. Guarded in the tool layer, not in the shared kernel source: the
  operator's console/CLI repair button uses the same method, and an operator
  repairing their own guardian is legitimate — the same split `op=edit` documents.
- **The operator "Repair" button ran on an unguarded goroutine.** A fourth
  WF-001 site, missed in the original sweep because it is a closure inside a
  roster handler rather than in one of the runner packages: it answers "accepted"
  immediately and then drives a full governed run with nothing above it able to
  recover. A contained panic now reports as a `failed` phase on the same
  `agent.repair` operator-action event, so the console shows a failure instead of
  a request that never completes.
- **An AWS `credential_process` helper inherited the daemon's entire
  environment.** `cmd.Env` was never set, so an external credential helper — very
  often a third-party binary named by a config file, invoked *precisely* because
  the daemon should not hold the credential itself — was handed the vault
  passphrase, every provider API key and the console password. Its environment is
  now scrubbed. The non-secret AWS selectors that tell a helper *which* identity
  to produce (`AWS_PROFILE`, `AWS_REGION`, the config-file paths, …) are still
  forwarded, since dropping them would break the feature rather than secure it;
  `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`/`AWS_SESSION_TOKEN` deliberately are
  not, because producing those is the helper's job. Helpers configured through
  their own environment (a Vault-backed helper needing `VAULT_ADDR`, say) can be
  accommodated without weakening the default via the new
  `AGEZT_AWS_CREDENTIAL_PROCESS_ENV`, a comma-separated allowlist of extra names.
  The feature remains opt-in behind `AGEZT_AWS_CREDENTIAL_PROCESS_ALLOWED`.
- **Outbound webhook sink URLs were journaled and served verbatim, and for the
  three most common providers the URL *is* the credential.** A Slack, Discord or
  Teams incoming-webhook URL carries no separate token — possession of the URL is
  full authority to post as that integration — and none of the redactor's
  token-shaped rules matched a bare `https://` URL, so every sink appeared in the
  journal and on `/api/webhook_log` in full. Three templated redactor rules now
  mask the credential-bearing tail. Fixed in the redactor rather than at the API,
  so it applies before the journal write and covers `agt journal` and every other
  reader. The host and non-secret identifiers survive, because *which* sink failed
  is the whole point of the webhook log. **Note:** the journal is append-only and
  hash-chained, so URLs logged before this change cannot be scrubbed — rotate any
  sink URL that was already recorded.
- **Secrets scoped to the isolated execution tier were delivered into an
  un-isolated child process.** `shell` and `code_exec` chose their credential
  bucket from the isolation profile they *requested*, never from what the host
  would actually run. The requested profile defaults to `namespace`, but every
  non-Linux host downgrades that to no isolation at all — so on **Windows and
  macOS, on the default path**, `*_WARDEN` secret files and secret env vars (the
  ones an operator put in the isolated tier precisely because they did not trust
  the code) were mounted into a plain `cmd /S /C` child. The same held for a
  `*_DOCKER` bucket whenever the container backend was unavailable. The bucket
  now follows the *effective* profile. **Behaviour change, deliberately
  fail-closed:** on a host that cannot isolate, `*_WARDEN`/`*_DOCKER` secrets now
  stay home rather than arriving un-isolated — move such entries to `*_LOCAL`,
  which is an accurate statement of what was already happening. The engine is
  still asked for the original profile, so the existing
  `warden.profile_downgraded` event remains the operator's notice.
- **A vault could dictate its own unlock cost, without an upper bound.** Vault
  decryption validated a *floor* on the envelope's stored `kdf_iter` and then
  derived the key with that same attacker-supplied number. PBKDF2 is O(iterations)
  by design, so `kdf_iter: 2000000000` was not a slow unlock but a hang — and
  because the vault is opened during daemon **boot**, one edited integer wedged
  the entire service with no error. There is now a ceiling as well as a floor,
  checked before the derivation, with 50× headroom so a future release can raise
  the shipped count without stranding vaults it already wrote.
- **The ChatGPT sign-in status poll raced the OAuth callback.** The handler took
  the mutex, copied the login pointer out, released it, and only then read the
  `status`/`errMsg` fields the callback goroutine writes under that same mutex.
  Both reads moved inside the critical section. (Confirmed by Go's race detector.)
- **`kernel/warden`'s package doc promised isolation it does not implement.** The
  profile table read as a description of behaviour — "`ProfileNamespace`: Linux
  namespaces + cgroups + seccomp" — when what ships is `setpgid` plus best-effort
  `prlimit` on Linux, and nothing whatsoever anywhere else. `warden_linux.go` was
  already explicit about this; the package header a reader hits first was not, and
  it is the document that made the credential-bucket bug above look safe. Now
  states what each profile actually does on each platform.
- **A panicking auto-repair or detached workflow run took the whole daemon
  down.** Completes the WF-001 panic-firewall sweep at the three sites the
  workflow-runner fix explicitly left open. **Auto-repair** (`kernel/selfrepair`)
  had no `recover()` anywhere: its coordinator loop runs on a bare `go`, and each
  repair is dispatched on another one, driving providers, tools, plugin
  subprocesses, MCP servers and a mailbox post — so a panic in the fleet's own
  healer terminated the process. Both are now firewalled, and because `dispatch`
  is also what releases the per-agent in-flight claim, a contained panic no
  longer leaks the claim and wedges every later repair of that agent; the tick
  recover releases any candidate that was claimed but never launched. **Detached
  workflow runs** (`kernel/controlplane`) — the webhook fire-and-return path and
  `async: true` runs — reach the engine on a bare `go` *without* passing through
  the runner's `safeFire`, so one bad node killed the daemon after the caller had
  already been answered "accepted". A contained auto-repair panic is journaled as
  the new `selfrepair.panic` kind; a detached workflow panic reuses
  `workflow.panic`. Both regression tests were verified to fail without the fix,
  and they assert containment rather than survival: the auto-repair test requires
  the *next* repair to actually run, not merely that the process is still alive.
- **Governor routing/capability/budget events are now linked to their run.** The
  Governor's per-call decision events — `routing.decision`, `provider.fallback`,
  `rate.limited`, `budget.exceeded`, `capability.rerouted`, `capability.rejected`
  — were emitted without a correlation id, so they were orphaned from the run
  that triggered them: they didn't appear in the run timeline and `agt why
  <event-id>` on one resolved nothing. They now carry the request's correlation
  (matching `budget.consumed` and the new `capability.degraded`), so the full
  routing/spend story for a run is reachable from `agt why` and the run-detail
  view.
- **Rune-safe display truncation (codebase-wide).** A shared `strutil.Ellipsis`
  helper now backs every text truncation that reaches a user or the model: the
  provider-fallback reason in `agt status`, generated-plan node snippets, and AWS
  SSO/STS/web-identity error excerpts were all converted from byte slices to
  rune-safe cuts (joining the schedule-intent, coding-diff, and browser-text fixes
  below). No multi-byte UTF-8 rune (Turkish ç/ş/ğ, CJK, emoji, …) can be split
  into invalid output anywhere.
- **Rune-safe display truncation.** Three user-facing truncations — the schedule-
  intent shortener in `agt schedule` / cadence logs, the coding tool's diff
  output, and the **browser tool's extracted page text** sent to the model —
  sliced on a byte boundary, which could split a multi-byte UTF-8 rune (e.g. a
  Turkish ç/ş/ğ, or any non-English web page) into invalid output. All now cut on
  a rune boundary, so truncated intents, diffs, and fetched web text are always
  valid UTF-8. (The journal's own answer truncation was already rune-safe.)

### Fixed
- **`warden.executed` / `warden.profile_downgraded` events were not linked to
  their run.** They now carry the originating run's correlation id, so they show
  in the run timeline and are reachable from `agt why <event-id>`.

### Added
- **OpenAI-compatible `GET /v1/models/{id}` (retrieve model).** The OpenAI surface
  already listed models at `GET /v1/models`; it now also answers a single-model
  retrieve — what the official SDKs' `models.retrieve(id)` calls for capability
  probing. A routable id (the default model or a catalog id, the same set the list
  advertises) returns the model object; an unknown id returns a `404` with an
  OpenAI-shaped error, so a client distinguishes "unknown model" from "endpoint
  missing" (SPEC-15 §3 / SPEC-16 §1.1).
- **Import agentskills.io / ClawHub `SKILL.md` files.** `agt skill import` now
  accepts a `.md` file written to the open agentskills.io standard (YAML-ish
  frontmatter — name/description/triggers/tools_required — plus a Markdown body),
  parses it with a dependency-free frontmatter reader, and installs it as a fresh
  draft skill (content-addressed, journaled, never auto-active). The hundreds of
  existing community skills load into Agezt without rewriting — and gain
  versioning, shadow-testing, and reversibility on top (SPEC-13 §1.2). A Agezt
  `.skill.json` export bundle still imports as before (content-address verified).
- **`agt provider check --caps` advertises prompt caching.** The capability
  report (and its `--json` `prompt_cache` field) now shows whether a model
  supports prompt caching — derived from its catalog cache-read price, the same
  signal the cache-aware billing uses. Completes the SPEC-15 §1.2 advertised
  capability set (tool-use / reasoning / vision / JSON-mode / prompt-caching);
  free/local models report `no`.
- **Web UI: context inspector in run detail.** Each `llm.request` row in the
  run-detail arc now shows a compact context summary (`N ctx chars · system …,
  user …`) and expands (▸/▾) to a full per-source breakdown — answering "how big
  was the context and where did it come from" right in the Live Monitor
  (SPEC-07 / SPEC-10 §3.5). Renders the `context_by_role` field added this
  release; XSS-safe by construction (textContent only).
- **Context size is recorded on every LLM call.** The `llm.request` journal
  event now carries `context_chars` (the assembled context size) and
  `context_by_role` (a per-source breakdown: system / user / assistant / tool) —
  the SPEC-10 §3.5 context-observability foundation. An operator (or `agt why`)
  can now see how big each call's context was and where it came from — the #1
  driver of cost and "lost in the middle" quality loss. Image attachments are
  excluded (a separate modality).
- **Point-in-time restore: `agt restore --at <seq|timestamp> --to <dir>`.** The
  journal is a time machine — this replays the source home's journal up to a
  sequence or RFC3339 timestamp into a fresh `--to` home, "branching a recovered
  state" (SPEC-09 §5). Non-destructive: the source journal is opened read-only
  and untouched; the cutoff prefix is chain-verified before write and the
  resulting home is confirmed to boot. A cutoff past the head restores
  everything; a target that already has a journal is refused.
- **Anomaly auto-halts appear in the system changelog.** `agt changelog` (the
  tamper-evident system timeline, SPEC-08 §4.2) now surfaces a `system.anomaly`
  event as "anomaly auto-halt" with its reason, alongside the `halt` it triggers
  — so an operator sees *why* the daemon stopped itself, not just that it did.
- **Anomaly auto-halt: a runaway circuit breaker.** A new always-on safety
  guard (SPEC-06 §5) watches the global tool-call rate across every run, channel,
  and Pulse; if it exceeds a ceiling within a window — the signature of a runaway
  or looping agent — it auto-engages `halt` (cancelling in-flight runs, blocking
  new ones) and journals a `system.anomaly` event explaining why. This is a
  daemon-wide backstop above the per-run loop guard. On by default (>120 tool
  calls / 10s); tune with `AGEZT_ANOMALY_MAX_TOOLCALLS` (0 disables) and
  `AGEZT_ANOMALY_WINDOW`. The boot banner shows the active setting.
- **`agt why` now shows causation provenance.** Alongside the events sharing a
  correlation, `agt why <event>` renders a "caused by (provenance, root first)"
  section that walks the `causation_id` chain back to the root cause — the
  provenance graph SPEC-01 §7.1 describes. This crosses correlation boundaries
  the correlation list cannot: e.g. a Pulse initiative carries its own
  correlation but links to the originating tick (a different correlation) only
  via `causation_id`, so the tick is now reachable. The chain is also in the
  `--json` output (`causation_chain`). Read-only; the daemon omits trivial
  single-event chains.
- **Web UI: config inspector panel.** A new "Config" panel answers "what is this
  daemon actually running with?" — the resolved model, system-prompt-set flag,
  tool/plugin counts, ask-policy, base paths, and which `AGEZT_*` env vars are
  set. Privacy-safe by construction: env vars are shown by **presence only**
  (never their values), and the system prompt is a set/unset flag (never its
  text). Backed by the existing `config` control-plane command (also available as
  `agt config`); the web panel makes it visible without shelling in.
- **Web UI: full tool I/O in run detail.** The run-detail modal's event arc now
  lets you expand any `tool.invoked` / `tool.result` row (▸/▾) to reveal the full,
  untruncated tool input (pretty-printed JSON) and output (or error) — the
  actionable half of debugging a run, straight from the browser instead of dropping
  to `agt journal`. Non-tool rows are unchanged. (Assistant message *text* remains
  unshown — it is deliberately not journalled; only tool I/O is.)
- **Email channel (outbound).** Agezt can now deliver Pulse briefs and `agt send`
  messages to operator inboxes over SMTP (stdlib `net/smtp`, no new dependency).
  Enable with `AGEZT_EMAIL_SMTP_ADDR` + `AGEZT_EMAIL_FROM` (+
  `AGEZT_EMAIL_USERNAME`/`_PASSWORD` for SMTP AUTH and `AGEZT_EMAIL_RECIPIENTS` for
  the fail-closed recipient allowlist). Outbound-only — inbound email (IMAP/MX) is
  out of scope. The recipient allowlist means a misconfigured brief can't mail
  arbitrary addresses; credentials are never logged.
- **Generic webhook channel.** A vendor-neutral inbound/outbound HTTP channel
  (SPEC-04): any external system can drive an Agezt agent by POSTing a signed JSON
  message (`{channel_id, sender, text, id, ts_ms}`) and receives the agent's reply
  synchronously in the response — the generic counterpart to the Slack/Discord
  channels, no platform SDK. Enable with `AGEZT_WEBHOOK_SECRET` +
  `AGEZT_WEBHOOK_ADDR` (+ `AGEZT_WEBHOOK_CHANNELS` allowlist); set
  `AGEZT_WEBHOOK_OUTBOUND_URL` for async/proactive delivery (Pulse briefs,
  `agt send`). Security mirrors the other channels: HMAC-SHA256 signature
  (`X-Agezt-Signature`, same scheme as outbound webhooks — empty secret fails
  closed), a timestamp freshness window + id de-duplication for replay protection,
  a fail-closed allowlist of channel ids, and bounded request bodies. The agent's
  tool calls still pass through Edict.
- **`agt pulse --text` shows live content.** The human event tail can now append a
  one-line excerpt of each event's text — the streamed answer tokens and a
  reasoning model's chain of thought — so an operator can watch *what* the agent is
  producing live, not just event kinds. Off by default; the structured one-line
  format is unchanged without the flag. This rounds out reasoning visibility:
  reasoning now reaches editors (ACP), API clients (`reasoning_content`), and the
  operator's own `agt pulse`.
- **DeepSeek-R1 on Bedrock — with its reasoning.** `deepseek.r1-*` models (and
  regional profiles like `us.deepseek.r1-v1:0`) now work through Bedrock. The
  adapter renders DeepSeek's chat-template prompt and splits the model's chain of
  thought (the `<think>…</think>` block) from the answer, feeding the reasoning
  into the same pipeline as every other reasoning model — so it surfaces in
  `agt pulse`, the ACP thought-chunk relay, and the OpenAI-compatible API's
  `reasoning_content`. Token usage comes from the Bedrock response headers.
- **Amazon Nova models on Bedrock.** Agezt's Bedrock provider now speaks the Nova
  `messages-v1` body shape, so `amazon.nova-*` models (Micro / Lite / Pro /
  Premier) and their regional cross-inference profiles (`us.amazon.nova-*`, …)
  work alongside the existing Anthropic, Mistral, Cohere, Meta-Llama, and AI21
  Jamba families. Nova returns token counts inline, so the governor sees real
  spend. The legacy `amazon.titan-*` text models stay intentionally unwired (Nova
  is the current family). Chat-only — like the other non-Anthropic Bedrock
  adapters, tool use is not wired on this path.
- **Reasoning models' chain of thought is now captured.** For DeepSeek-R1 and
  other openai-compatible reasoning models that return `reasoning_content`, the
  reasoning streams live as ephemeral `llm.reasoning` events (visible in
  `agt pulse`) and its size is recorded on the `llm.response` event — previously
  it was discarded. The durable journal stays lean (the reasoning text isn't
  persisted); ordinary models are unaffected.
- **Claude extended thinking** is supported (opt-in via
  `AGEZT_ANTHROPIC_THINKING_BUDGET=<tokens>`). When enabled, the Anthropic
  provider requests extended thinking and captures Claude's chain of thought into
  the same reasoning pipeline (live `llm.reasoning` events). Off by default
  (thinking costs extra tokens).
- **Gemini thinking** is supported (opt-in via
  `AGEZT_GOOGLE_THINKING_BUDGET=<tokens>`; `-1` lets Gemini pick a dynamic
  budget). When enabled, the Google provider requests thought summaries
  (`includeThoughts`) and captures them into the same reasoning pipeline. Gemini
  reports thinking tokens separately from answer tokens but bills them as output,
  so they're folded into the run's output-token count for accurate cost. With
  this, all three major reasoning families — DeepSeek-R1, Claude, Gemini — flow
  through one pipeline. Off by default.
- **Gemini thinking on Vertex AI** is supported too (opt-in via
  `AGEZT_GOOGLE_VERTEX_THINKING_BUDGET=<tokens>`; `-1` for a dynamic budget), so
  the thinking capability now spans *both* Gemini surfaces — the Generative
  Language API and Vertex AI — with the same reasoning capture and output-token
  accounting. Separate env var because Vertex is a distinct billing/credential
  surface. Applies to native-Gemini models on Vertex; off by default.
- **Claude extended thinking on Vertex AI** is supported as well. The same
  `AGEZT_GOOGLE_VERTEX_THINKING_BUDGET` opt-in now drives extended thinking for
  `claude-*` models served through Vertex (`:rawPredict` / `:streamRawPredict`),
  with the budget clamped to Anthropic's 1024-token floor and `max_tokens` bumped
  above it — matching the direct Anthropic adapter. Claude's chain of thought is
  captured into the same reasoning pipeline. With this, *every* reasoning-capable
  provider Agezt speaks — direct Anthropic, direct Gemini, Vertex Gemini, Vertex
  Claude, and openai-compatible DeepSeek-R1 — surfaces its reasoning uniformly.
  Off by default.
- **Reasoning reaches the editor (ACP).** When Agezt runs as an ACP agent (`agt
  acp`, e.g. inside Zed), a reasoning model's chain of thought is now relayed as
  `agent_thought_chunk` session updates — distinct from the answer's
  `agent_message_chunk` — so the editor renders it in its dedicated "thinking" UI.
  Previously the reasoning was captured but dropped at the ACP boundary; only the
  answer streamed through. Non-reasoning runs are unchanged.
- **Reasoning reaches OpenAI-compatible API clients.** When you point a client at
  Agezt's OpenAI-compatible endpoint (`/v1/chat/completions`) and the model
  reasons, its chain of thought is now surfaced as `reasoning_content` — on
  `message.reasoning_content` for non-streaming responses and as
  `delta.reasoning_content` chunks when streaming — the DeepSeek-R1 convention
  many clients already render. Non-reasoning runs omit the field entirely (the
  response is byte-identical to before). With ACP above, the captured reasoning
  now reaches both of Agezt's external surfaces.
- **Reasoning on the Responses API too.** The newer `/v1/responses` surface now
  carries a reasoning model's chain of thought as a `reasoning` output item (with
  a `summary_text`), and streams it as `response.reasoning_summary_text.delta` /
  `.done` events — the Responses-API shape, distinct from the answer's
  `output_text`. Non-reasoning runs are unchanged. Reasoning now spans both
  OpenAI-compatible endpoints (Chat Completions + Responses).

### Fixed
- **Bedrock Mistral/Cohere runs now report real token spend.** Those vendors'
  response bodies carry no token counts, so the governor saw zero spend and
  under-billed them. Agezt now overlays Bedrock's authoritative
  `X-Amzn-Bedrock-Input-Token-Count` / `-Output-Token-Count` response headers onto
  the usage when the decoded body has none — so cost accounting and per-run budget
  caps work for every Bedrock vendor. Vendors that already report inline counts
  (Anthropic, Nova, Meta-Llama, AI21 Jamba) keep their richer body-derived usage.
- **Non-streaming reasoning is no longer dropped.** When a run used a provider's
  non-streaming path (no token streaming), a reasoning model's chain of thought
  was captured on the response but never published as an `llm.reasoning` event —
  so it was invisible to every consumer (`agt pulse`, the ACP thought-chunk relay,
  the OpenAI API's `reasoning_content`); only its character count survived. The
  loop now emits the reasoning as a single ephemeral event on the non-streaming
  path too, so reasoning capture is uniform whether or not the provider streams.
- **Ollama now honours the run's token cap.** `MaxTokens` is forwarded as
  Ollama's `options.num_predict`, so a local model respects the same output limit
  every cloud provider enforces — previously the cap was silently dropped on
  Ollama. Uncapped runs are unchanged.
- **Credential vault: a corrupt or tampered vault file no longer crashes the
  process.** `decryptVault` now validates the nonce length before calling
  AES-GCM `Open` — Go's GCM *panics* (rather than returning an error) on a nonce
  that isn't 12 bytes, so a vault whose `nonce` base64-decodes to the wrong length
  (disk corruption, a truncated write, or deliberate tampering) would have crashed
  the daemon/CLI instead of failing cleanly. It now returns a clear
  "vault corrupt or tampered" error. (Ciphertext and salt lengths were already
  safe — GCM errors on a short ciphertext and PBKDF2 accepts any salt.)

### Fixed
- **The OpenAI-compatible API now reports real provider token usage.** The
  `usage` block on `/v1/chat/completions` and `/v1/responses` was a rough
  whitespace word-count estimate, so a cost-tracking client reading it got wildly
  wrong numbers (e.g. `prompt_tokens: 8` for a run that actually consumed 1406).
  The server now reports the real tokens the provider billed — summed across the
  run's LLM calls, folded from the journal's `budget.consumed` events — falling
  back to the estimate only when no usage was recorded (a free/local/mock model).
  New optional `UsageReporter` engine capability; verified end-to-end against a
  live gpt-5.5 gateway (`1406/11` vs the old `8/1`).
- **OpenAI-compatible providers no longer reject every tool-bearing request.**
  Agezt exposes a dotted tool name (`browser.read`), but OpenAI and strict
  openai-compatible gateways require tool names to match `^[a-zA-Z0-9_-]+$` and
  return a 400 ("does not match pattern") for the whole request. With the
  always-on mock fallback catching that error, **every run against a real
  OpenAI-compatible provider silently fell back to the mock** — invisible unless
  you inspected `provider.fallback` in the journal. The openai adapter now
  sanitises tool names on the wire (`browser.read` → `browser_read`, in both the
  streaming and non-streaming request, and in assistant tool-call history) and
  maps the name back on the response so the tool call still routes to the real
  tool. Verified end-to-end against a live gateway (gpt-5.5): a multi-turn
  tool-using run completed on the real provider with real token spend and **no**
  fallback.
- **`agt skill import` of a skill with no triggers/tools no longer errors.** The
  CLI sent the optional `triggers` / `tools_required` args as an explicit JSON
  `null` when the skill had none, which the daemon's strict array decoder
  rejected ("must be an array"). Those args are now omitted when empty, so a
  minimal skill (name + body only) imports cleanly. Surfaced while building
  `agt skill registry --install`.
- **Corrected stale references to now-shipped features.** `agt provider check
  --stream` printed "provider family X does not yet support streaming (M1.q only
  wires anthropic; others land in M1.q.x)" when a provider lacked a streaming
  adapter — but every first-party family (anthropic, openai, google, bedrock,
  vertex, cohere, ollama, openai-compatible) now streams, so the message was both
  unreachable for real families and wrong; it now accurately points at re-running
  without `--stream`. A credential-vault doc comment that called `agt vault
  encrypt`/`migrate` "(deferred)" was likewise updated — both commands ship.
- **A single oversized ACP message can no longer balloon memory.** Both the ACP
  server (driven by an IDE) and the ACP client (driving an external agent) read
  with a `json.Decoder`, which buffers a whole JSON value with no size limit — so
  one giant message could exhaust memory. ACP is newline-delimited JSON, so both
  now read with a line scanner capped at 8 MiB per message; an over-cap message
  is rejected instead of buffered. Completes the previous fix, which bounded the
  *accumulation* of streamed chunks but not a single huge one.
- **A runaway ACP agent can no longer balloon the daemon's memory.** The
  `acpagent` tool accumulated every streamed chunk into one buffer and only
  truncated it to 60 KiB at the end — so an external agent that streamed without
  end grew the buffer unbounded (and could OOM the daemon, taking every
  concurrent run with it) before the timeout reaped it. Accumulation now stops at
  the 60 KiB cap; the relayed answer is unchanged.
- **Sending an image to a non-vision model via the API or a channel now fails
  fast with a clear message.** The control plane already pre-checked a run's
  model for vision capability before spending a provider call (M91), but the
  OpenAI-compatible API and the chat channels call the run path directly and
  bypassed that gate — so an image attached to a non-vision model produced a
  cryptic downstream provider error (and a wasted call) instead of an actionable
  one. Both paths now run the same confirmed-or-reject vision gate up front: an
  unknown or known-non-vision model is refused with "model … does not support
  vision (image input)".
- **The `browser` tool's host allowlist is now enforced on redirects too.** Same
  gap as the `http` tool: the allowlist was checked only on the initial URL, so
  an allowlisted page that 302-redirected to an arbitrary external host would be
  fetched anyway (netguard still blocked internal IPs). The fetch client now
  re-checks the allowlist on each redirect hop and caps the chain.
- **The `file` tool no longer lets a new file escape through a symlinked parent
  directory.** Writing a not-yet-existing path (e.g. `linkdir/new.txt` where
  `linkdir` is a symlink to a directory outside root) was checked only
  lexically, so the new file could be created outside the workspace. The
  containment check now symlink-resolves the deepest existing ancestor of a new
  path and confirms it is inside root, while still allowing legitimate writes
  that create parent directories.
- **The `file` tool no longer lets an absolute path bypass its symlink
  containment.** A symlink inside the workspace root pointing outside it was
  correctly refused when reached by its relative path, but the absolute-path
  branch of the containment check skipped symlink resolution — so the same
  symlink could be read/written via its absolute path, escaping the workspace.
  Both branches now resolve symlinks and verify the real location is inside root.
- **The `http` tool's host allowlist is now enforced on redirects, not just the
  first URL.** netguard already blocks internal/metadata IPs on every hop, but
  the host allowlist was checked only on the initial URL — so an allowlisted host
  that returned a 302 to an arbitrary external host would send the follow-up
  request (carrying any headers the agent set, including `Authorization`) to a
  host the operator never allowed. The tool now re-checks the allowlist on each
  redirect hop and caps the chain, closing an allowlist-bypass / header-leak via
  open redirects.
- **The Responses API (`/v1/responses`) now accepts image input too.** Chat
  Completions already forwarded `image_url` parts (the prior change); the
  Responses surface ignored its `input_image` parts (where `image_url` is a bare
  string, a different shape). It now extracts them — tolerating both the string
  and `{url}` object forms — and forwards them to the run, so vision input works
  on both OpenAI-compatible endpoints. An image-only Responses input runs with a
  default instruction.
- **An image attached to a Discord slash command now reaches a vision model —
  inbound vision is complete across all three channels.** When a slash command
  carries an `ATTACHMENT` image option, the channel resolves it via
  `data.resolved.attachments`, downloads the CDN file after the fast interaction
  ACK (so the 3-second deadline is never at risk), and forwards it as a `data:`
  URL; an image-only command (no prompt text) is no longer rejected as "nothing
  to do". Non-image attachments are ignored.
- **An image shared in Slack now reaches a vision model.** Like the Telegram
  fix, the Slack channel ignored inbound file attachments. It now downloads each
  shared *image* file (`url_private`, authenticated with the bot token) as a
  `data:` URL and forwards it to the run; non-image files and files from
  non-allowlisted channels are skipped. (Discord slash-command attachments are
  the remaining inbound surface.)
- **A photo sent to the Telegram bot now reaches a vision model.** Inbound
  channel messages only carried text, so a user sending a picture (with or
  without a caption) got a text-only run and the image was lost. The Telegram
  channel now fetches the largest photo size (getFile → download, in the channel
  where the bot token lives), forwards it as a `data:` URL on the unified
  message, and the run threads it to the model via the same path the CLI and API
  use. A photo's caption becomes the message text; an uncaptioned photo runs with
  a default "describe the image" instruction. Photos from non-allowlisted senders
  are never fetched. (Discord/Slack inbound attachments are follow-ups.)
- **Agezt's OpenAI-compatible endpoint now accepts image input from clients.**
  The `/v1/chat/completions` server flattened multimodal content to text and
  silently dropped `image_url` parts, so a client sending a vision request to
  Agezt-as-a-gateway got a text-only run — the mirror of the provider-side gap
  just closed. It now parses `image_url` parts from user messages and forwards
  the URLs to the run (which the providers turn into the model's native image
  input), completing the round trip. An image-only message (no text) runs with a
  default "describe the image" instruction instead of being rejected as empty;
  a message with neither text nor image is still rejected. (Responses-API
  `input_image` parts are a separate shape, still a follow-up.)
- **Vision now also works on Vertex AI — every first-party provider is now
  covered.** Both Vertex encoders dropped image attachments: Anthropic-on-Vertex
  now emits a `type=image` base64 block, and Gemini-on-Vertex now emits an
  `inlineData` part, each before the text. With this, `agt run --image` reaches
  the model on every built-in provider — Anthropic, OpenAI, Gemini, Bedrock, and
  Vertex — plus the OpenAI-compatible vendors that wrap the OpenAI encoder.
- **Vision now also works for Claude-on-Bedrock.** The Anthropic-on-Bedrock
  encoder (the largest Bedrock use case) has its own copy of the Messages-API
  content-block builder, which also dropped image attachments. It now emits a
  `type=image` base64 block before the text block, matching the direct Anthropic
  provider. Covers both Bedrock request paths (streaming and non-streaming share
  the encoder).
- **Vision now also works on the Gemini provider — completing the mainstream
  set.** The Google `generateContent` encoder (`canonicalToGemini`) now emits a
  user message's image attachments as `inlineData` parts (base64 + mimeType)
  before the text part, instead of dropping them. With this, all three first-
  party providers — Anthropic, OpenAI, Gemini — deliver `agt run --image` to the
  model, and the OpenAI-compatible `compat` vendors (Groq, xAI, DeepSeek, …)
  inherit it through the OpenAI encoder. Covers both Gemini request paths
  (streaming and non-streaming share the encoder).
- **Vision now also works on the OpenAI provider.** Following the Anthropic fix,
  the OpenAI provider's `canonicalToOA` now emits a user message's image
  attachments as OpenAI's multimodal content-parts array (a `text` part followed
  by one `image_url` part per attachment, carrying the `data:` URL OpenAI accepts
  natively) instead of a text-only string. The message `content` field became
  polymorphic (string or parts array) without disturbing the text path — a
  tool-call-only assistant message still omits `content`, and a non-URL
  attachment is skipped rather than sent as an invalid `image_url`. Covers both
  the streaming and non-streaming request encoders.
- **`agt run --image` now actually sends the image to the model (Anthropic).**
  The flag stat-checked the file, gated the run against the model's vision
  capability, and journaled an attachment count — but only the *basename*
  travelled to the daemon, which no provider could resolve, so the picture never
  reached the model: vision was silently a no-op. The CLI now reads the bytes
  (the file is on the operator's machine, not the daemon's), forwards a
  self-describing `data:` URL, and the Anthropic provider emits it as a base64
  `image` content block on both the streaming and non-streaming paths. Supported
  types: png, jpeg, gif, webp; oversize files are refused client-side with a
  clear message against the 16 MiB control-plane request cap. (OpenAI/Gemini
  emission lands in follow-up milestones.)
- **A crashed daemon now gives an actionable CLI error, not "connection
  refused".** When the daemon left a stale runtime address (it crashed but its
  addr file remained), every `agt` command failed with a cryptic transport error,
  unlike the clear "start the daemon" hint shown when it was never started. The
  client now does a liveness probe and reports both cases the same way —
  "daemon recorded but not responding … (re)start the daemon". A server-side
  rejection (e.g. a bad token) is distinguished and not misreported as a crash.
- **The ACP server reports the real product version to IDEs.** Its
  `agentInfo.version` was a hardcoded `"0.1.0"`, so an editor connecting to a
  v1.0.0 daemon over the Agent Client Protocol displayed "agezt 0.1.0".
  `agentInfo` now sources its name and version from `internal/brand`, so it
  tracks the actual release (and won't drift on the next bump). The ACP
  `protocolVersion` is unchanged — it's a separate, correctly-constant field.
- **An empty or whitespace-only outbound message is now a no-op, not a failed
  send.** Every channel's send path (Telegram, Discord incl. slash-command
  follow-ups, Slack) returns early on blank text instead of POSTing it — the
  platforms reject an empty message (Telegram 400 "message text is empty", Slack
  "no_text"). This covers the proactive `Send` path (Pulse, `agt send`) — which
  had no guard at all — and whitespace-only agent answers that the inbound reply
  paths' exact-`""` check missed.
- **Long messages to Telegram and Discord are no longer dropped.** A reply over
  the platform's per-message limit (Telegram 4096 UTF-16 code units, Discord 2000
  characters) was sent as a single oversize request, which the API rejects — so
  the agent's answer never arrived. Outbound text is now split into sequential
  in-limit messages (breaking at newline/space boundaries where possible, with a
  hard cut for an unbroken run). A shared `channel.SplitText` does the splitting
  losslessly, counting UTF-16 code units so it's safe for both Telegram (counts
  those) and platforms that count runes/code points. Discord's slash-command
  follow-up path (a long answer to a `/command`) is chunked the same way, and
  Slack (40000-char limit) too — all three channels now split rather than drop.
- **Moonshot AI (Kimi) now works**, and an unrecognised provider package fails
  with an actionable error. Moonshot's official package (`@ai-sdk/moonshotai`)
  hit the same dead end DeepSeek did — classified as an unknown family and
  refused. It's now wired as OpenAI-compatible with its base URL
  (`https://api.moonshot.ai/v1`). And the error for a genuinely-unknown package
  no longer claims (falsely) that the case is "unreachable for any catalog entry";
  it now tells the operator to set the provider's npm to `openai-compatible` in
  `custom.json` if it speaks the OpenAI API — turning a dead end into a one-line fix.
- **DeepSeek now works.** Its official package (`@ai-sdk/deepseek`) classified as
  an unknown family, so `compat.Build` refused it outright with "provider family
  not yet supported" — a vendor named in the README that couldn't actually be
  used. It's now classified as OpenAI-compatible (its wire dialect) with its base
  URL carried, so it works with just a `DEEPSEEK_API_KEY`.

### Added
- **OpenAI-compatible vendors work with just an API key — no `custom.json` URL.**
  Groq, xAI, Cerebras, Together, DeepInfra, Perplexity, Fireworks, and OpenRouter
  are vendors agezt already classifies (`catalog.FamilyFromNPM`), but their base
  URL had to be supplied by hand or the build was refused (to avoid silently
  routing to `api.openai.com`). compat now carries each one's stable
  OpenAI-compatible base URL, so configuring one of them needs only its key. An
  explicit catalog `api` still wins, and an *unrecognised* compat vendor is still
  refused with the `custom.json` hint.

### Security
- **Redaction extended to the Perplexity (`pplx-…`) and Fireworks (`fw_…`) key
  formats** — the two OpenAI-compatible vendors made first-class in this release
  whose keys the earlier rule set didn't catch. (Cerebras `csk-…` is already
  covered by the `sk-…` rule matching its substring.)
- **Plugin stderr is now redacted before it reaches the daemon log.** A
  third-party plugin's stderr is captured and written to the operator's log via
  the plugin logger — a direct path the bus redactor (journaled events only)
  never covered. A plugin that printed a secret (its own API key, etc.) leaked it
  in the clear. Each line now passes through pattern-based redaction first; the
  `[plugin:<name>]` prefix is preserved.
- **Secret redaction now covers the formats agezt's own integrations handle.**
  Added high-confidence patterns for Telegram bot tokens (`<id>:<35-char>`, the
  Telegram channel), Slack app-level tokens (`xapp-…`, complementing the existing
  `xox…`), and Groq (`gsk_…`) and xAI (`xai-…`) API keys — both first-class compat
  providers whose keys the broad `sk-…` rule did not match. Without these, such a
  secret appearing in a log line, tool output, or journal payload would have gone
  out in the clear. False-positive-guarded against ordinary text.

### Fixed
- **`AGEZT_PLUGINS` duplicate prefix is now a hard startup error.** Parsing of
  the plugin spec moved into a testable `plugin.ParsePluginSpec`. Previously two
  entries sharing a prefix (`search=/a,search=/b`) both spawned, and the second
  plugin's tools lost a name conflict to the first, emitting a misleading
  "conflicts with in-process version" warning while a second process ran
  unused. A repeated prefix is a config typo, not a request to run two plugins
  under one namespace, so it is rejected at startup — matching the
  already-strict `AGEZT_PLUGIN_PINS` / `AGEZT_PLUGIN_TOOLS` parsers. Malformed
  entries (missing `=`, empty prefix, empty path) are likewise hard errors now
  rather than silent warn-and-skip, so a typo can't leave the daemon quietly
  running with fewer tools than configured.

### Added
- **`agt plugin new <name>`** — a plugin scaffolder (the ROADMAP's
  `create-agezt-plugin`). It generates a complete, buildable Go tool plugin on
  top of the SDK: a gofmt-clean `main.go` with one example tool (the output is
  run through `go/format`, so it is always valid, formatted Go), a `go.mod`
  requiring the agezt SDK with a local-dev `replace` hint, a README with build
  and `AGEZT_PLUGINS` wiring instructions, and a `.gitignore`. Refuses to write
  into a non-empty directory. Flags: `--dir`, `--module`. Turns the SDK from
  "copy the example by hand" into "one command to a working plugin" — verified
  end-to-end by building a scaffolded plugin against the real SDK and driving
  its protocol.
- **Go plugin SDK** (`plugins/sdk`) — the official authoring kit for tool
  plugins. `sdk.Serve(sdk.Tool{...})` implements the entire line-delimited JSON
  protocol on the author's behalf: initialize/invoke/shutdown dispatch, frame
  demux, goroutine-safe write serialisation, progress streaming (`Emit`),
  host callbacks (`CallHost`), and panic containment (one bad call returns a
  tool error instead of crashing the plugin). A plugin shrinks from the ~260
  lines of hand-rolled protocol in `testdata/echoplugin` to just its tool
  logic. The package is stdlib-only and imports no kernel package, preserving
  the rule that plugins never compile against the daemon (DECISIONS B0). A
  complete runnable example lives at `plugins/sdk/example/greet`; an
  integration test compiles it and drives it through the real kernel plugin
  host (initialize, invoke success/error, progress, host callback). First
  post-1.0 step toward the polyglot SDK story (ROADMAP §5).

### Fixed
- **Three console lists silently dropped rows they could not identify.** Each was a
  de-duplication key built by *coercing* a possibly-absent field, which turns
  absence into a value that then collides with every other absence.

  `useCursorPager` merged every page through `String(r[idKey])`. For a row without
  that field the result is the truthy string `"undefined"`, so the guard written to
  *keep* unidentifiable rows (`if (id && seen.has(id))`) never fired for them: the
  first id-less row poisoned the seen-set and each later one was discarded as a
  phantom duplicate. The hook backs `/api/runs`, `/api/agents`, `/api/inbox`,
  `/api/board`, `/api/memory` and the twelve journal-backed `*_log` endpoints, and
  `AgentActivityRow.seq?` is declared optional — id-less rows are contract-legal
  input, not a hypothetical.

  `recentAttentionAlerts`, `attentionAlertCount` and `notableEvents` keyed each row
  on `e.id`, falling back to a kind-and-seq template. An event carrying neither field
  degrades that to a constant — `"task.failed-"` for every run failure — so two
  distinct failures collapsed into one: the cockpit strip, its badge and the mission
  timeline all undercounted real signals.

  All three now share one rule, `eventDedupKey` (`frontend/src/lib/rundetail.ts`),
  which tests *presence* (`!= null`, so a genuine `seq: 0` — the first journaled
  event — survives), namespaces `s`/`i` so `seq: 1` cannot collide with `id: "1"`,
  and returns `""` for a row that cannot identify itself. Callers must then keep the
  row rather than dedup on a shared constant. `mergeEvents` was already the only site
  getting this right; it is now the single source of truth instead of a lone good
  example, and it rejects a present-but-blank `id: ""`, which carries no information
  and would otherwise hand every blank-id event the same key.

- **Dismissing an alert that had no id or seq never stuck.** `rowOf` minted row
  identity from `e.id`, or else a kind-and-seq template whose seq term fell back to
  `Math.random()` — and that one value serves at once as the `mergeAlerts` dedup key,
  the React key, and the dismissal key persisted to `localStorage`. With neither field
  present it was different on every call, so `dismiss()` stored an id that could never
  match again: the alert resurfaced on the next mount indefinitely, and the 400-entry
  cap filled with orphaned random ids.
  Identity is now a pure function of event content, falling back to an FNV-1a digest
  of `kind|subject|correlation_id|ts_unix_ms`. Deliberately *not* a positional index:
  `rowOf` runs over the seeded live buffer, every SSE append and the journal
  backfill, whose results are merged and re-sorted by timestamp, so a position-derived
  id drifts as history backfills and un-dismisses the row in a new costume. Rows that
  do carry an id or a seq keep their exact previous shape, so dismissals an operator
  already saved still resolve.

- **`tools/changelog-split --emit` could destroy changelog history it cannot
  regenerate.** The split tree, not root `CHANGELOG.md`, holds the only copy of most
  released notes: root carries one version block while the tree holds `v1.0.0.md`,
  `v0.1.0.md` and ten `m*-m*.md` milestone slices, none of which root can rebuild.
  The tool treated root as its sole source anyway and pruned every `.md` outside the
  current write set, so an emit driven by root's two-line pointer deleted the
  tree-only releases and the buckets outright and rewrote `unreleased/current.md`
  from 86,688 bytes to 288 — and `--force`, added to let a maintainer adopt
  hand-held files, discarded that working set silently.
  Emit is now ownership-gated: every tree file the tool writes carries a trailing
  `generated by tools/changelog-split` comment, and a file without it is neither
  overwritten nor deleted. `--force` adopts such files but still cannot discard the
  working set — dropping lines that appear in no generated output needs
  `--discard-working-set`, which first copies `current.md` to a timestamped
  `.bak-` file. Loss is measured line-wise rather than by size, because a release
  slice legitimately shrinks `current.md` by moving `**M###**` chunks into buckets:
  relocated content is accounted for, only content with nowhere to go is refused.

- **Release files were addressed under a doubled name.** `versionHeaderRe` captures
  the tag verbatim, and real headers read `## [v1.1.0]`, so prefixing unconditionally
  produced `vv1.1.0.md`. `--verify` reported a missing release file and `--emit`
  would have written that stray name while pruning the real `v1.1.0.md`. The tag is
  now stripped of a leading `v` before the prefix, so both `## [1.0.0]` and
  `## [v1.1.0]` forms resolve to `v1.0.0.md` / `v1.1.0.md`.

- **The tool could not read its own output, so it was a one-shot migrator.**
  `renderMain` replaced root's released blocks and working-set content with a pointer
  plus a `## Releases` index, but `buildSplit` requires at least one version block and
  takes the working set from root — a second `--emit`, or `--verify` after the first,
  died with `no released version blocks found`. Root now round-trips: the working set
  and each released block are re-emitted verbatim, the pointer is added only when
  missing, and the derived index moves to the end, which required a companion parser
  rule that a version body ends at the next `## ` header of *any* kind — otherwise the
  index was swallowed into the last release's body and copied into its tree file, so
  root never reached a fixed point. This inverts a previously pinned invariant
  ("root should not inline released bodies"): the alternative was a tool that cannot
  be run twice. Two emits against one root now leave it byte-identical, and
  `--verify` reaches green once the tree is adopted.

- **`--verify` listed the same defects in a different order on every run.** It
  iterated the expected-file map directly, and Go randomises map order, so CI logs and
  any diff of gate output were un-reproducible. Paths are now sorted and printed in
  slash form, making the report identical across runs and byte-identical between
  Windows and POSIX.

- **A forced `--emit` could delete milestone history and still report success.** The
  loss gate inspected only one file: it worked out which lines an emit would drop by
  scanning every document it generates, but applied that accounting solely to
  `unreleased/current.md`. `--force` adopts every tree target the run writes, though,
  so a forced emit replaced a hand-held bucket whose content appears in no generated
  output — on a throwaway copy of the real tree one such run cut `m600-m649.md` from
  1,704 lines to 354 and exited 0, and the same path cost two `README.md` lines,
  three in `REORG-LOG.md` and sixty-five in `v1.1.0.md`. Under the decision that the
  split tree is canonical, those were the only copies. The accounting was already
  general, so the repair was to ask its question of every tree file this run writes,
  in sorted order for a reproducible refusal, with the root changelog still excluded
  because regenerating that index is the tool's job. A refusal still lands before the
  prune and before any write, so a blocked emit leaves the whole tree untouched, and
  `--discard-working-set` now backs up each affected file rather than only the working
  set. `treeguard_test.go` pins both halves: a hand-held bucket survives a forced
  emit, and a re-emit over a tree the tool already owns is never refused. The second
  assertion is what caught a regression in the fix itself, where the generated readme
  and reorg log were left out of the index, so an ordinary second run read its own
  `README.md` as entirely orphaned and broke emit idempotency.

- **The Rust SDK could serialize a number that is not valid JSON.** `Value::to_json`
  wrote every float through Rust's `Display`, which renders a non-finite value as
  `inf`, `-inf` or `NaN` — spellings `Value::parse` itself rejects. This is reachable
  through the parser, not only through hand-built values: Rust's `f64` conversion
  saturates instead of failing, so an overflowing exponent in a daemon response yields
  `+inf`, and `Value::Float` is a public variant, so a caller can hold `NaN` directly.
  A `parse` -> `to_json` -> `parse` round trip therefore dropped the value, breaking
  the crate's own round-trip invariant, and a request body assembled with `to_json()`
  was text the daemon could not decode. Non-finite floats now serialize as `null`, the
  choice `JSON.stringify` makes, so `to_json` always yields valid JSON. Returning an
  error instead was rejected on evidence: `to_json` discards the writer's `Result`, so
  an error would truncate the output silently — worse than the bug it reported. Finite
  values are untouched, including huge-but-finite ones and negative zero, and both
  `i64` extremes keep their exact form, so the fix cannot quietly corrupt a real cost
  or usage figure.

- **The Python SDK read one event stream differently from its sibling SDKs.**
  `_parse_sse` built each `data:` value with `.lstrip()`, but the `text/event-stream`
  rule removes exactly one space after the colon and treats everything past it as
  content; further leading spaces, and leading tabs, were destroyed. Leading whitespace
  ahead of a JSON value is invisible to `json.loads`, so the damage surfaced only on the
  `{"raw": ...}` fallback that carries a non-JSON payload — which is where it silently
  rewrote the text, stripping the indentation of every line of a multi-line event. The
  sharper half of the impact was cross-language: the Rust client strips a single space
  and the TypeScript client replaces a single leading space, so one stream parsed to
  different values depending on which SDK consumed it. (The Go client has no stream
  parser to compare against — its only `data:` occurrences are image URLs — so this was
  Python against Rust and TypeScript, not a three-way drift.) The value now loses one
  leading space, which is safe because `pyproject.toml` requires Python 3.9 or newer and
  the package is standard-library only; the asyncio client delegates to this same
  parser, so its streams were affected identically and are fixed by the same line.
  `event:` still trims rather than stripping one space — the same shape, but event names
  carry no whitespace, so it has no observable effect and was left alone rather than
  widening this change.

- **`--emit --force` dropped the releases that root only declares in its index.**
  `renderReadme`, `renderReorgLog` and `renderMain` built their release lists solely from
  root's parsed version blocks, and root carries a single such block, so an adopt run
  rewrote `README.md` and `REORG-LOG.md` without their `v1.0.0.md` and `v0.1.0.md` lines —
  content those files held and no generator could reproduce. Root's hand-written
  `Releases` index already declared the tag and date for both, but nothing parsed it, so
  the same two pointers vanished from root on the next render too.
  `parseDeclaredReleases` + `extraReleases` now feed declared releases into all three
  renderers, which also keeps root a fixed point: a second parse sees the same release set
  instead of a shrunk one.
  - The two pure-derived index files were exempted from the working-set loss gate so
    adoption can proceed at all; prose notes are **not** exempted, and a hand-written
    subsection count still counts as lost, because it is derived data no generator may
    silently rewrite.
  - Release *narrative* is a different kind of content and was not force-fed into an
    index: `v1.1.0.md`'s body lines belong in root's version block, which
    `renderVersion` re-emits verbatim, and moving them there is a content migration
    rather than a code change.
  - Proven on a throwaway copy: `--emit --force` without `--discard-working-set` exits 0
    with zero discarded lines and zero backups, and `--verify` reaches exit 0 after a
    re-emit. `TestGeneratedIndexFilesNameDeclaredReleases` asserts the release lines
    appear in generated **bytes**, so the absorption claim does not lean on the exemption;
    `TestDeclaredReleasesRoundTripIsStable` guards the fixed point and
    `TestAdoptingDerivedIndexFilesNeedsNoDiscardFlag` covers hand-held index files.
  - **The real tree is still not adoptable as committed.** Its working set contains four
    bare milestone references, so a release slice files a large chunk into a hand-held
    bucket and the generalized loss gate refuses the emit — the guard behaving correctly,
    not a regression.

- **`--discard-working-set` turned the changelog gate red on the tree it had just made
  safe.** `backupFile` writes its copy *beside* the file it preserves, producing
  `unreleased/current.md.bak-<UTC stamp>`, while `checkSplitTree` validates `unreleased/`
  against a strict allow-list — `current.md` or a bucket filename — and errors on anything
  else as an unexpected file. So taking the safety copy that the loss gate itself promises
  flipped `go run ./tools/changelog-lint` from green to red, and a committed backup would
  hold the gate red indefinitely. Self-inflicted this session: the backup mechanism and the
  widened gate are both recent additions, and neither had been checked against the other.
  - Fix is one anchored exemption matching the exact reference layout `20060102T150405Z`,
    so a look-alike with a malformed or missing stamp still errors. The backup is
    deliberately not shape-checked, because its purpose is to hold superseded bytes
    verbatim rather than present a valid layout.
  - Proven on the production path, not only a hand-written fixture:
    `--emit --force --discard-working-set` against a scratch copy generated
    `current.md.bak-…` and `v1.1.0.md.bak-…`, both measured as rejected by the old
    allow-list and accepted by the new one, and lint then exits 0 on that tree. An
    attribution control — appending one character to the stamp — sent the same tree red and
    restoring it returned green, so the exit code belongs to the exemption rather than to
    the tree.
  - `TestLintStillRejectsMalformedBackups` is the counter-direction guard: a stamp that is
    not a timestamp, an empty stamp, one missing its trailing `Z`, one with the wrong digit
    count, and the pre-existing unrecognized-file case all still fail the gate.
  - **Two findings reported, not fixed** (one issue per round): the same function silently
    *ignores* an unrecognized `.md` sitting at the split **root**, which is the
    mirror-image false green and exactly how a doubled-prefix release filename once went
    unnoticed; and the bucket allow-list matches any digit width, so a range like
    `m1-m2.md` is accepted.

- **That first finding is now fixed: an unrecognized markdown file at the split root fails
  the gate.** `checkSplitTree`'s root loop matched each filename against `releaseFileRe`,
  shape-checked the ones that hit, and had no rejection branch — anything that failed the
  pattern was passed over without a word, while the `unreleased/` loop three lines below
  already rejects unexpected files. The validator therefore enforced the rule in one
  directory and not the other, which is how `vv1.1.0.md`, the doubled-prefix filename
  `changelog-split` emitted before its filename fix, validated green while being referenced
  by nothing.
  - The new rule is deliberately narrower than the `unreleased/` one. That loop rejects
    *any* unexpected file; the root loop rejects only `.md`, because
    `TestCheckSplitTreeNoReleases` plants a non-markdown `notes.txt` at the root and depends
    on it being ignored. Full symmetry would have broken an existing test for a reason
    nobody asked for.
  - Discard backups keep their exemption. A pre-flight over the real tree confirmed all five
    committed root filenames are still accepted, so closing the hole cannot turn this
    repository's own layout red.
  - Proven by five stray names that each returned nil before the fix and are rejected after
    it. The counter-direction guard `TestLintAcceptsKnownRootFiles` (valid release files,
    root and unreleased backups, the required index pair, a subdirectory, `notes.txt`)
    passes both before and after, which makes it a non-regression check rather than a proof.
    The load-bearing check is end to end: a real `--emit --force --discard-working-set`
    writes `v1.1.0.md.bak-<stamp>` at the root and lint exits 0 on that tree, while planting
    `vv1.1.0.md` in the same tree returns exit 1 and removing it returns green — so the
    verdict belongs to the new rule and not to the tree.
  - **Still open:** the bucket allow-list accepts any digit width, so `m1-m2.md` is treated
    as valid; and neither changelog gate appears in any workflow file, so this stronger
    validator still runs automatically nowhere.

### Added

- **The changelog gates now run in CI** (closing the second item of the note above).
  `changelog-lint` and `changelog-split --verify` existed as tools but appeared in zero
  workflow files, which is precisely how root `CHANGELOG.md` could lose its
  `## [Unreleased]` section and stay red unnoticed, and how a doubled-prefix release
  filename emitted by the sibling tool validated green. Added a `changelog` job to
  `.github/workflows/ci.yml` with the linter as a blocking step and the verifier as an
  advisory one.
  - `--verify` is advisory **on purpose, not by omission**: the committed tree is
    unadopted, so it reports four drifts today. Because the required status check is the
    workflow-level `CI` context — which aggregates *every* job, and which ruleset
    `22206739` enforces with `enforce_admins` and no bypass actors — making that step
    blocking would turn unrelated pull requests red rather than protect the changelog.
    Flipping `continue-on-error` to false is gated on a content migration that absorbs the
    hand-held lines into the generators or into root's version block; CI must never pass
    `--discard-working-set`, the flag that discards the canonical working set.
  - The job runs on `ubuntu-latest` rather than the house `[self-hosted, Linux, X64]`
    label, and the contradiction this entry first recorded is now settled with `gh api`
    evidence rather than argument. The header comment at the top of the same file claimed
    hosted minutes were blocked by Actions billing, and that claim was stale: across the
    twelve most recent runs (measured 2026-09-06) the workflow's one pre-existing hosted
    job, `race-depth (linux, cgo)`, concluded **success six times, failure five, cancelled
    once**, while all twenty self-hosted jobs were cancelled in every one of those runs —
    240 job-cancellations, with zero runners registered. Hosted Linux minutes therefore
    do execute, so the header was corrected to say that instead of moving this job onto a
    label that currently cannot run it; a gate that never gets a runner enforces nothing.
    Hosted macOS and Windows remain unproven, because the demonstrated job is Linux-only,
    so the test matrix legs stay parked. Measurement note, since it bit twice: filtering
    that job by the name `race-depth` matches nothing, because the API reports the display
    name with its ` (linux, cgo)` suffix — aggregate on the verbatim name.
  - A `needs` entry was deliberately *not* added: `needs` orders jobs inside a workflow and
    has no bearing on whether a failing job reddens the `CI` check, so aggregating this job
    into another one would have been churn.
  - Verified with a real YAML parser rather than grep: the file parses, the `on:` trigger
    survived (a bare `on` folding to boolean `true` is a YAML 1.1 parser quirk, not a
    workflow error), seventeen jobs are intact with no duplicate key, every step has
    `uses` or `run`, the referenced local composite action exists, and the only
    `continue-on-error` step in the file is this job's verifier. Both gate commands were
    re-measured as the job invokes them: linter exit 0, verifier exit 1 with four drifts,
    which is exactly the pair the job encodes.

### Fixed

- **AWS `credential_process` credentials now refresh at expiry.** The IMDS and STS AssumeRole
  sources cache temporary credentials only until `Expiration` minus a 60-second refresh lead;
  the `credential_process` source parsed the helper's output but discarded `Expiration` and
  memoized it under a process-lifetime `sync.Once`, so a helper minting temporary credentials
  (the advertised aws-vault / 1Password wrapper case) was used exactly once per daemon life —
  after which every AWS signing call failed with `ExpiredToken` until restart. The helper is
  now re-run when the cached credential nears expiry, and a failed re-run is suppressed
  briefly instead of being retried for every credential name; helpers that advertise no
  expiry still run once.
- **The AWS credential lookups stop re-running doomed fetches.** One chain resolution probes
  three credential names in sequence, and each failed fetch used to be retried for the next
  name — up to three identical network calls per resolution (10-second timeout each) against
  a refused or black-holed endpoint, repeated for the daemon's lifetime. The AssumeRole,
  web-identity and SSO caches now suppress retries for 30 seconds after a failure (mirroring
  the IMDS layer's negative cache), so one resolution costs at most one doomed call and the
  chain still falls through to the next source.
- **Agent subprocess tokens can no longer exceed the parent's burst ceiling.**
  `CreateSubprocessToken` halved the parent's burst without a floor, so a parent capped at
  burst 1 halved to 0 — and the mint path re-defaults a zero burst to 10, handing the child
  ten times the operator's configured allowance. The halving now floors at 1, mirroring the
  expiry clamp (a subprocess never outlives its parent) and the HTTP mint path's identical
  clamp; regression tests cover the floor and the halving above it.
- **SSO role credentials without a session token are rejected.** `GetRoleCredentials` returns
  temporary STS credentials that always carry one, and the STS and web-identity parsers
  already refused token-less responses; the SSO parser accepted them, letting the chain pair
  SSO's key pair with a session token from another source — credentials that cannot sign
  together, surfacing as cryptic `SignatureDoesNotMatch` failures instead of a clear
  malformed-response error.
- **A truncated SSO token cache fails closed.** The AWS CLI always writes `expiresAt` to its
  SSO token cache — its own refresh depends on it — but a cache holding an access token with
  no expiry was previously treated as never-expiring, sending an ancient bearer token to the
  portal (an opaque 401) instead of the actionable error. Such a cache is now rejected at
  read time with the re-login guidance, before any network call.
- **`scripts/build.sh` no longer claims a build artifact it never produces.** The `build`
  target is a compile check — `go build ./...` writes no binaries — but it printed
  `Build complete: $(go env GOBIN)/agezt` and the header promised the same stamped binary as
  `make build`, which emits nothing either. The message and header now describe what the
  script actually does; runnable daemons come from `make install` or the dev scripts.
- **Agent-gateway rate limits are keyed per token, not per subprocess name.** Top-level tokens
  carry no subprocess id, so they all shared one bucket whose limits came from whichever token
  arrived first: a tighter-limited token rode a looser bucket (a limit bypass), or the reverse
  throttled a token on another's budget. Buckets are now keyed by the token's unique id, with
  regression tests proving a 1-request-per-minute token is actually refused past its budget
  while an unrelated token is untouched.
- **The shell tool's 64 KiB output budget is enforced on combined output.** Warden caps stdout
  and stderr each at the limit and the tool concatenated them, so a command with ~40 KiB on
  both streams shipped ~80 KiB to the model with no truncation marker — neither stream alone
  tripped its cap, so warden's truncation flag never fired. The renderer now tail-truncates
  the combined output to the budget and always marks it; under-budget output passes through
  byte-for-byte.
- **The shared HTTP router enforces the route method it records.** `Handle` validated,
  normalized and logged each route's method allowlist but never wired it: every route —
  including the console's POST-only mutation endpoints (`/api/files/delete`,
  `/api/rollback/apply`, `/api/login`, the plan-run stream, the workflow hooks) — accepted any
  HTTP method, and the route inspector reported a tighter policy than the server applied. A
  wrong method now draws 405 with an `Allow` header; the auth wrapper stays outermost, so
  unauthenticated requests keep their 401, and the empty-method migration default still
  accepts everything.
- **The TypeScript SDK leaked the SSE connection on every early stream exit.** `parseSSE()`
  released the stream reader without cancelling it, so a consumer that `break`ed out of
  `runStream()` / `mailboxWatch()` — the natural `if (event === "done") break` shape — left the
  underlying HTTP request and socket open for as long as the upstream kept the stream alive,
  one leaked connection per run; an exception in the consumer's loop body leaked identically.
  The reader is now cancelled in the same `finally`, aborting the request and closing the
  socket (a cancellation rejection is swallowed so the consumer's original error still
  surfaces). Bounded regression tests prove the server observes socket close after an early
  exit at both call sites.
