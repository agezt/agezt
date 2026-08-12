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
