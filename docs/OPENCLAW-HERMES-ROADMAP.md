# AGEZT OpenClaw / Hermes Competitive Roadmap

Date: 2026-07-01

This roadmap is for making AGEZT clearly stronger than OpenClaw and Hermes Agent
while also covering the capabilities users reasonably expect from both products.
It is based on official project docs and the current AGEZT repository shape, not
on unverifiable marketing claims.

## Source Snapshot

OpenClaw sources checked:

- `https://github.com/openclaw/openclaw`
- `https://openclaw.ai/`
- `https://docs.openclaw.ai/concepts/features`
- `https://docs.openclaw.ai/channels`
- `https://docs.openclaw.ai/gateway/config-channels`
- `https://docs.openclaw.ai/tools`
- `https://docs.openclaw.ai/tools/browser`
- `https://docs.openclaw.ai/tools/browser-control`
- `https://docs.openclaw.ai/tools/browser-login`
- `https://docs.openclaw.ai/tools/skills`
- `https://docs.openclaw.ai/tools/skill-workshop`
- `https://docs.openclaw.ai/tools/skills-config`
- `https://docs.openclaw.ai/automation`
- `https://docs.openclaw.ai/automation/cron-jobs`
- `https://docs.openclaw.ai/automation/standing-orders`
- `https://docs.openclaw.ai/automation/taskflow`
- `https://docs.openclaw.ai/concepts/memory`
- `https://docs.openclaw.ai/concepts/memory-builtin`
- `https://docs.openclaw.ai/concepts/memory-qmd`
- `https://docs.openclaw.ai/concepts/memory-honcho`
- `https://docs.openclaw.ai/clawhub`
- `https://clawhub.ai/`

Hermes Agent sources checked:

- `https://github.com/NousResearch/Hermes-Agent`
- `https://hermes-agent.nousresearch.com/docs/`
- `https://hermes-agent.nousresearch.com/docs/user-guide/features/overview`
- `https://hermes-agent.nousresearch.com/docs/user-guide/features/tools`
- `https://hermes-agent.nousresearch.com/docs/user-guide/features/skills`
- `https://hermes-agent.nousresearch.com/docs/user-guide/features/curator`
- `https://hermes-agent.nousresearch.com/docs/user-guide/features/memory`
- `https://hermes-agent.nousresearch.com/docs/user-guide/features/context-files`
- `https://hermes-agent.nousresearch.com/docs/user-guide/features/tool-search`
- `https://hermes-agent.nousresearch.com/docs/user-guide/features/web-dashboard`
- `https://hermes-agent.nousresearch.com/docs/user-guide/features/kanban`
- `https://hermes-agent.nousresearch.com/docs/user-guide/features/kanban-worker-lanes`
- `https://hermes-agent.nousresearch.com/docs/user-guide/messaging/`

AGEZT local evidence checked:

- `README.md`
- `docs/ARCHITECTURE.md`
- `docs/COMPARISON.md`
- `docs/NEXT.md`
- `plugins/channels/*`
- `plugins/providers/*`
- `plugins/tools/*`
- `plugins/builtinskills/*`
- `kernel/skill/*`
- `kernel/board/*`
- `kernel/workflow/*`
- `plugins/tools/browser/browser.go`
- `plugins/tools/coding/coding.go`

## Positioning

AGEZT should not compete as another chat agent. The durable winning position is:

> AGEZT is an agentic operating system for governed autonomous fleets: durable
> identities, typed authority, wake causality, memory/world/skills, channels,
> plugins, workflows, and every action in an auditable journal.

OpenClaw is strongest as a personal gateway with excellent channel/browser/device
packaging, skill marketplace distribution, and operator-facing setup flows.
Hermes is strongest as an agent CLI/gateway with skill self-improvement,
terminal-backend ergonomics, checkpoints, and a durable multi-agent Kanban queue.

AGEZT already has a broader runtime core than both in several places: Edict
policy, hash-chain journal, durable roster, typed schedules, standing orders,
mailbox wake causality, multi-tenant REST/OpenAI/ACP surfaces, official SDKs,
BLAKE3-pinned plugin/skill installs, vault, budget/cost controls, and a broad
channel/provider/tool surface. The roadmap below turns that core into visible
product parity and then into durable advantage.

## Capability Map

| Area | OpenClaw / Hermes expectation | AGEZT current evidence | Roadmap decision |
|---|---|---|---|
| Chat gateway | OpenClaw and Hermes both support many messaging platforms with allowlists/pairing and media differences. | AGEZT has 25 channel plugins in `plugins/channels`, Connect/Channels docs/UI, and a configured/live/roundtrip readiness matrix in API/CLI/Web UI. | Treat as parity; add per-adapter setup recipes and optional safe live roundtrip probes. |
| Agent identity | OpenClaw uses agents/sessions; Hermes profiles/SOUL; Hermes Kanban workers are named profiles. | AGEZT roster identities have lifecycle, owner/parent, memory, tools, authority, model, workdir, health, self-repair. | Lead with durable identity as a differentiator. Add import/migration from OpenClaw/Hermes profiles. |
| Memory | OpenClaw has Markdown memory, search backends, Honcho; Hermes has MEMORY/USER and providers. | AGEZT has `kernel/memory`, `kernel/worldmodel`, audit CLIs, provenance, retention, vector interfaces. | Add "memory compatibility bridge" for MEMORY.md/USER.md/AGENTS.md import/export and wiki-like compiled views. |
| Skills | OpenClaw has Skill Workshop proposals and ClawHub. Hermes has agent-created skills and Curator. | AGEZT Forge has draft/shadow/active/quarantine/archive, lineage, BLAKE3 IDs, revert, hygiene. | Productize as "Forge Workshop": proposal UI, scanner cards, skill curator, ClawHub/agentskills-compatible import. |
| Browser | OpenClaw and Hermes both expose browser automation with snapshots/click/type/vision. | AGEZT `browser.read` is SSRF-guarded fetch/text extraction; opt-in `browser.action` plus `browser.open/snapshot/click/type/wait/screenshot/downloads/cookies/tabs/close` wrappers can run Playwright actions with compact snapshots, browser events, screenshots/download artifacts, cookie inspection on request, download capture, AGEZT-managed `profile=session` cookie/storage carryover, persistent `tab_id` final-URL refs for URL-less follow-up actions, saved snapshot `ref` resolution with missing-ref errors, saved tab-ref list/close lifecycle, and operator-gated isolated/user-attached/remote-cdp profile policy through the bundled browseruse driver. | Grow the wrappers into live browser-process tab control tools with DOM stale-ref invalidation and E2E tests. |
| Terminal/sandbox | Hermes exposes local/docker/ssh/singularity/modal/daytona terminal backends; OpenClaw has sandbox modes and browser host-control rules. | AGEZT has shell/codeexec, Linux warden, netguard, coding worktree isolation, browser sessions, dockerservices/sshremote built-in skills, a unified execution-profile inventory/API/CLI/Web UI showing requested vs effective isolation for local, warden, worktree, browser, docker, ssh, remote-agezt, modal, daytona, and k8s profiles; `agt run --exec-profile local\|warden\|docker\|ssh\|k8s\|modal\|daytona\|remote-agezt` per-run routing for shell/code execution when Docker/SSH/Daytona are explicitly configured, local/SSH/K8s `code_exec` `.agezt-artifacts/` export into the durable artifact index, K8s shell/code_exec routing through `kubectl exec`/`kubectl cp` into an existing configured pod, Modal shell/code_exec routing through `modal shell --cmd` with `--add-local` workspace mounts for code_exec and bounded artifact return, and Daytona shell/code_exec routing through `daytona exec` with bounded workspace materialization and bounded artifact return; `remote-agezt` whole-run peer delegation through `remote_run` with local lifecycle events, explicit `--peer` pinning for multi-node meshes, structured peer correlation metadata, `agt peers run <peer> <corr>` run drill-down, `agt peers artifacts <peer> <corr>` artifact drill-down, opt-in metadata or redacted-payload peer event mirroring, REST artifact metadata mirroring without bytes, policy-gated remote artifact byte transfer via `AGEZT_REMOTE_ARTIFACT_BYTES=allow`, and Run Detail remote artifact summaries with copyable governed download commands; live `AGEZT_EXEC_PROFILE_ALLOW` / `AGEZT_EXEC_PROFILE_DENY` run-profile policy; live SSH/K8s/Modal/Daytona backend controls; restart-bound Docker/OCI and peer-mesh controls; profile-specific env/secret-env passthrough plus vault-backed temporary secret file mounts for local/warden/docker; explicit remote/cloud `AGEZT_EXEC_REMOTE_SECRET_POLICY` reporting with values denied by default; Config Center and Execution Profiles UI controls; Chat UI execution-profile selection; and `agt exec-profile check` health checks for routing, policy, downgrade, docker/podman, ssh, peer, modal, daytona, kubectl, remote-secret-policy, and remote-artifact-bytes readiness. | Add K8s job lifecycle. |
| Checkpoints/rollback | Hermes snapshots before file changes and exposes `/rollback`. | AGEZT now has run-scoped file checkpoints, skill/workflow/config checkpoints, `agt rollback list/show/dry-run/apply`, Run Detail rollback UI, and audit-only labels for irreversible tools. | Broaden checkpoint hooks to patch/coding/package-update paths and add agent-detail grouping. |
| Durable work queue | Hermes Kanban is a durable SQLite board with profile lanes, dependencies, heartbeats, comments, blocking, and crash recovery. | AGEZT now has typed durable workboard tasks with dependencies, comments, links, claims, heartbeats, retry/escalation policy, journaled transitions, agent-facing tooling, Edict policy, CLI/API/Web UI assignee lane grouping, dedicated Workboard detail UI, dependency-gated async roster-agent dispatch/watch, automatic failed-attempt retry, exhausted-attempt escalation, manual stale-claim reclaim, and optional timer-based stale-claim sweep. | Add graph-style dependencies, inline artifact/diff preview, and process/delegation heartbeat integration. |
| Automation | OpenClaw cron/standing/taskflow; Hermes cron/hooks/batch. | AGEZT typed schedules, standing orders, workflows, pulse, system tasks. | Parity exists. Add side-by-side demos and importers for cron/standing definitions. |
| MCP/tool search | Both expose MCP and progressive tool discovery. Hermes Tool Search defers non-core schemas. | AGEZT has MCP bridge, `tool_search` style contracts, tool catalog, toolforge. | Make deferred tool discovery a core daemon feature with metrics and policy-aware filtering. |
| Marketplace | OpenClaw has ClawHub; Hermes has skills hub and plugin system. | AGEZT supports remote plugin/skill registries with BLAKE3 verification. | Add registry trust UX: signatures, risk cards, scanner results, provenance, quarantine, update policy. |
| Media/voice | OpenClaw has generated media, media understanding, TTS/STT; Hermes gateway supports voice/media by channel. | AGEZT has voice mode, STT/TTS, image provider, sendmedia, inbound media-capable channels, and image/voice in/out capability matrix in API/Web UI. | Add artifact-native image/audio/video review flow in UI. |
| Desktop/device | OpenClaw emphasizes companion apps, device nodes, smart home, desktop/browser control. Hermes has desktop/dashboard. | AGEZT has web UI, Home Assistant, computeruse/browseruse built-in skills, tunnel, voice, `agt peers`, and node registry API/Web UI for the local daemon plus reachable AGEZT peers. | Build tray/PWA/mobile companion on top of the node registry; do not make the daemon feel headless-only. |
| Onboarding | OpenClaw/Hermes have clear setup wizards and dashboards for providers/channels/MCP/skills. | AGEZT has `agt quickstart`, Setup/Connect/ConfigCenter views. | Make setup the first-class product surface with one-pass provider, channel, MCP, skill, sandbox, and demo validation. |
| Safety/audit | All three claim safety; AGEZT has strongest core journal/policy story. | Edict, trust, approvals, hash-chain journal, `agt why`, vault, netguard, warden, plugin pins. | Make audit the moat: every competitor-parity feature must emit policy/effect/provenance evidence. |

## Roadmap

### Phase 0: Evidence And Parity Audit

Goal: turn "we can do it" into a checked parity ledger.

Current artifact:

- `agt compare audit [--target openclaw|hermes|all] [--json]`
- [`COMPETITOR-PARITY.md`](COMPETITOR-PARITY.md)

Deliverables:

- Keep `agt compare audit --target openclaw|hermes|all` checking local
  capability evidence: channels, tools, provider families, browser mode, MCP,
  skills, workflow, schedule, checkpoint, gateway, media, sandbox, SDK.
- Keep `docs/COMPETITOR-PARITY.md` aligned with the audit plus manually reviewed
  notes.
- Add smoke recipes for:
  - channel inbound + outbound
  - browser read/render/action
  - skill proposal -> review -> activation -> revert
  - cron/standing wake -> journal -> UI
  - subagent/delegation -> result -> audit
  - durable board task -> worker -> blocked/done
- Add a public "AGEZT does everything X can" demo script only after the smoke
  recipes pass locally.

Acceptance criteria:

- The audit command prints supported / partial / missing with file or API
  evidence for each row.
- No checklist row can be marked supported without a local test, fixture, command,
  or documented manual recipe.

### Phase 1: Browser And Web Action Parity

Goal: close the most visible OpenClaw/Hermes gap: interactive browser control.

Current AGEZT state:

- `browser.read` is strong for safe text fetch and SSRF resistance.
- `browser.action` is an opt-in first-party wrapper for Playwright
  actions, compact snapshots, browser events, screenshots/download artifacts,
  cookie inspection on request, download capture, AGEZT-managed
  `profile=session` cookie/storage carryover, persistent `tab_id` final-URL refs
  for URL-less follow-up actions, saved snapshot `ref` resolution with
  missing-ref errors, saved tab-ref list/close lifecycle, and
  operator-gated isolated/user-attached/remote-cdp profile policy
  through the browseruse driver; it is not yet a live browser-process tab tool
  family.
- `browser.open`, `browser.snapshot`, `browser.click`, `browser.type`,
  `browser.wait`, `browser.screenshot`, `browser.downloads`, `browser.cookies`,
  `browser.tabs`, and `browser.close` now exist as first-class wrappers over the
  same governed engine.

Build:

- Live browser-process tab versions of `browser.open`, `browser.snapshot`, `browser.click`,
  `browser.type`, `browser.wait`, `browser.screenshot`, `browser.downloads`,
  plus richer cookie/state inspection.
- Ref model: extend saved snapshot refs into live tabs with DOM-level stale-ref
  invalidation. Stale refs must fail clearly and ask for a fresh snapshot.
- Extend AGEZT-managed session carryover from persistent URL/ref state into live
  browser-process tabs, with explicit profile state visibility.
- Vision path: screenshot descriptions through configured vision provider when
  the active model is text-only.
- Browser event stream: requests, console errors, downloads, screenshots and
  final artifacts in journal/artifact store.
- E2E tests against local test pages: login form, iframe, file download, SPA,
  cookie carryover, blocked private network, failed stale ref.

Acceptance criteria:

- AGEZT can complete the same class of "open page -> inspect -> click -> type ->
  wait -> screenshot" tasks as OpenClaw/Hermes through first-class governed tool
  names; durable multi-step tab sessions still gate full support.
- Every browser action has effect metadata, policy decision, journal event, and
  artifact links.

### Phase 2: Forge Workshop And Skill Curator

Goal: beat OpenClaw Skill Workshop and Hermes Curator with stricter provenance.

Current AGEZT state:

- `kernel/skill` already has content-addressed skills, lineage, draft/shadow/
  active/quarantine/archive, revert, metrics, shadow eval, auto-quarantine.

Build:

- Done: `agt skill workshop propose-create|propose-update|list|inspect|scan|diff|curate|apply|reject|quarantine`
  over the existing journaled Forge lifecycle, with deterministic risk scan output.
- Done: Web UI pending-proposal apply/reject panel and scanner risk chips on
  skill cards.
- Done: Web UI parent diff previews and hash/source/parent/resource chips.
- Add `revise` as a higher-level assisted edit flow after scanner metadata exists.
- Web UI Workshop next: support files, stronger hash binding, richer risk card,
  and provenance path to `agt why`.
- Agent-generated proposals after successful runs, gated by profile authority.
- Skill scanner pack:
  - prompt injection patterns
  - shell/network/file effects
  - secret exfiltration hints
  - unpinned dependency installs
  - suspicious URLs
  - cross-workspace writes
- Curator job:
  - Done: deterministic stale-skill dry-run/execute via `agt skill workshop curate`
  - optional LLM consolidation in shadow mode
  - never deletes; always archive/revertable
- Import/export:
  - agentskills.io / ClawHub-style `SKILL.md`
  - Hermes skill directory
  - OpenClaw workspace skills

Acceptance criteria:

- Agent-drafted skills cannot become active without an auditable transition.
- Applying a proposal binds to the target hash and rejects stale proposals.
- Curator activity is visible in journal, CLI, and UI.

### Phase 3: Run Checkpoints And Rollback

Goal: cover Hermes `/rollback` while keeping AGEZT honest about irreversible
tools.

Implemented Phase 3 foundation:

- `agt rollback list/show/dry-run/apply` exists for local mutation checkpoints.
- Forge Workshop `apply/reject/quarantine/curate --execute` writes pre-mutation
  `skill.status` checkpoints before changing a skill.
- CLI workflow `save/remove/enable/disable` and saved `draft/refine/templates`
  edits write pre-mutation `workflow.snapshot` checkpoints before changing a
  workflow graph.
- The daemon-registered file tool writes pre-mutation `file.snapshot`
  checkpoints for `write`, `append`, `replace`, and `delete`; `agt rollback
  apply` restores the prior bytes or removes a file that did not exist at the
  checkpoint.
- File checkpoints carry `run_id` when invoked from an agent run, direct tool
  run, or workflow run; `agt rollback list --run <id>` filters to that run.
- `agt config set` writes pre-mutation `config.setting` checkpoints. Non-secret
  values and unset secrets can be restored; already-set secrets are marked
  audit-only because the daemon never reveals their previous value.
- Rollback apply restores the checkpointed skill status through the daemon and
  restores checkpointed workflow graphs through the daemon, appending journaled
  `skill.restored` / `workflow.restored` events.
- Web UI Run Detail has a rollback drawer that lists run-scoped checkpoints and
  can apply supported checkpoints after operator confirmation.
- Tool catalog rows expose `effect_class`, `rollback_mode`, and
  `rollback_notes`; irreversible tools are labeled `audit only` instead of
  promising undo.

Build:

- Broaden run-scoped checkpoint hooks beyond file mutations to patches, coding
  worktree outputs, package updates, and other first-party mutators.
- Add Agent Detail rollback grouping across recent runs.
- Tool contract upgrade:
  - reversible: exact reverse operation available
  - compensable: restore from checkpoint or apply inverse patch
  - irreversible: explicitly not rollbackable, with reason
- Add old/new hashes and operator identity to rollback journal payloads where
  the restored resource supports hashing.

Acceptance criteria:

- A file-changing run can be reverted in a fresh repo fixture.
- A skill proposal apply can be reverted to the prior active version.
- Irreversible tools never promise rollback; UI shows "audit only" instead.

### Phase 4: AGEZT Workboard

Goal: surpass Hermes Kanban with AGEZT-native durable multi-agent work.

Design constraints:

- This must be a typed work queue, not hidden prompt storage.
- Tasks are not agents. Agents own, claim, block, complete, delegate, or review
  tasks.
- Workflow nodes may create tasks; schedules may create tasks; channels may
  create tasks; agents may create tasks under policy.

Build:

- Done: `kernel/workboard` store with tasks, links, comments, claims,
  heartbeats, attempts, status, priority, tenant, assignee, idempotency key,
  artifacts, retry policy, atomic JSON persistence, and restart recovery.
- Statuses: `triage`, `todo`, `ready`, `running`, `blocked`, `review`, `done`,
  `archived`.
- Done: `agt workboard create|list|lanes|show|claim|heartbeat|comment|block|unblock|complete|archive|link`.
- Done: `agt workboard policy|fail` for task-level retry/escalation policy
  and failed-attempt transitions.
- Done: `agt workboard depend|reclaim|sweep` for dependency edges,
  dependency-gated dispatch, manual stale-claim recovery, and bulk stale-claim
  sweep.
- Done: optional daemon auto-sweep through `AGEZT_WORKBOARD_SWEEP_EVERY` and
  `AGEZT_WORKBOARD_STALE_AFTER`.
- Done: `agt workboard dispatch|watch` for async roster-agent dispatch,
  automatic failed-attempt retry, exhausted-attempt escalation,
  run-correlation linking, review handoff, and task/run event watch.
- Done: runtime/control-plane mutations journal `workboard.task.*` events.
- Done: agent-facing `workboard` tool with list/show/create/claim/heartbeat/
  comment/block/fail/unblock/complete/archive/link/policy/depend/reclaim ops.
- Done: Edict `workboard` capability maps the agent-facing tool to its own
  policy axis.
- Worker lanes:
  - Done: backend/API/CLI grouping by assignee lane.
  - Done: compact Web UI lane strip grouped by assignee.
  - coding worktree lane
  - external ACP/Codex/Claude lane through existing `coding`/`acp_agent` bridge
  - no silent fallback if assignee is unresolved
- UI: compact lane strip plus dedicated Workboard detail view are done,
  including dependencies, blocked dependencies, run attempt history,
  links/artifacts, journal events, comments, block/fail/unblock/complete,
  retry-policy, and dispatch actions. Still needed: graph-style dependency
  edges, inline artifact/diff preview, and a fuller "ask human" flow.
- Crash recovery: Done for manual and timer-based stale claim reclaim plus
  policy-driven failure retry/escalation. Still needed: process/delegation
  heartbeat integration and richer operator UI for repeated failures.

Acceptance criteria:

- A multi-step engineering pipeline can survive daemon restart and continue.
- A worker can block for human input; the operator can unblock it in UI/CLI.
- Every transition is journaled and queryable through `agt why`.

### Phase 5: Execution Profiles

Goal: match Hermes terminal backends and OpenClaw sandbox modes with one AGEZT
control plane.

Build:

- Execution profiles:
  - `local`
  - `warden`
  - `browser-session`
  - `docker`
  - `ssh`
  - `remote-agezt`
  - `worktree-coding`
  - `modal` (partial: shell/code_exec routing)
  - `daytona` (partial: shell/code_exec routing)
  - `k8s` (partial: existing-pod shell/code_exec routing)
- Done: `kernel/executionprofile` inventory names the current/planned profiles
  and records requested vs effective isolation, routed tools, filesystem,
  network, env, secrets, limits, browser access, cleanup, and policy axis.
- Done: control-plane/Web API routes expose `execution_profiles` and
  `execution_profile_show`.
- Done: `agt exec-profile list|show` renders the same inventory from CLI.
- Done: `agt run --exec-profile local|warden|docker|ssh|k8s|remote-agezt` and control-plane
  `execution_profile` select the requested warden profile for shell/codeexec;
  Docker is accepted only when its backend is active, SSH only when configured,
  K8s only when an existing target pod is configured, and `remote-agezt` only
  when `remote_run` is registered from configured AGEZT peers. Unsupported
  profiles fail clearly instead of falling back.
- Done: optional Docker/OCI warden backend (`AGEZT_WARDEN_DOCKER=1`) routes
  shell/code_exec through `docker|podman run --rm`, mounts the tool workdir at
  `/workspace`, forwards only scrubbed tool env, maps common host interpreter
  paths to container commands, and keeps warden timeout/output/audit events.
- Done: optional SSH execution profile (`AGEZT_EXEC_SSH=1` and
  `AGEZT_EXEC_SSH_TARGET=user@host`) routes shell commands through the system
  `ssh` client with `BatchMode=yes`, optional remote workdir/port/key/host-key
  settings, and routes `code_exec` by syncing the generated workspace with
  `scp`, running the program remotely, copying `.agezt-artifacts/` outputs
  into the durable artifact index, and cleaning ephemeral remote dirs.
- Done: optional Kubernetes execution profile (`AGEZT_EXEC_K8S=1` and
  `AGEZT_EXEC_K8S_POD=<pod>`) routes shell commands through `kubectl exec`
  and routes `code_exec` by copying the generated workspace with `kubectl cp`
  before running inside an existing pod, with optional
  context/namespace/container/workdir settings, `.agezt-artifacts/` artifact
  copy-back into the durable artifact index, scrubbed local kubectl env, and no
  daemon secret forwarding into the pod.
- Done: optional Modal execution profile (`AGEZT_EXEC_MODAL=1`) routes shell
  commands through `modal shell --cmd` and routes `code_exec` by mounting the
  generated workspace with `modal shell --add-local`, then copying
  `.agezt-artifacts/` back through a bounded archive; optional
  ref/image/environment/workdir settings and local CLI config are supported, and
  local daemon token/key/secret env vars are not forwarded.
- Done: optional Daytona execution profile (`AGEZT_EXEC_DAYTONA=1` and
  `AGEZT_EXEC_DAYTONA_SANDBOX=<id-or-name>`) routes shell commands through
  `daytona exec` and routes `code_exec` by materializing the generated
  workspace through bounded `daytona exec` writes before running in the sandbox,
  then copies `.agezt-artifacts/` back through a bounded archive; optional
  working directory and local CLI config are supported, and local daemon
  token/key/secret env vars are not forwarded.
- Done: `remote-agezt` execution profile delegates the whole run through
  `Kernel.RunTool(..., "remote_run", ...)` to a configured AGEZT peer; the
  delegating node policy-gates `remote_run`, the peer executes with its own
  policy/journal/tools, and the local run emits `task.received`, delegation
  `info`, and `task.completed`/`task.failed` lifecycle events with structured
  `remote_peer`, `remote_model`, and `remote_correlation` metadata while
  returning the peer answer fail-closed.
- Done: `AGEZT_REMOTE_EVENT_MIRROR=metadata|redacted` opt-in fetches the peer
  run's `/api/v1/runs/{correlation}` event arc after a `remote-agezt` run and
  journals token-free remote event metadata locally; `redacted` additionally
  mirrors `payload_redacted` after local pattern redaction.
- Done: REST `GET /api/v1/artifacts?corr=<correlation>` exposes bounded,
  token-authed artifact index metadata with no bytes, and remote event mirroring
  best-effort includes peer artifact metadata for the delegated correlation.
- Done: `agt peers artifacts <peer> <correlation>` fetches the same remote
  artifact metadata on demand without printing peer tokens or artifact bytes.
- Done: `AGEZT_REMOTE_ARTIFACT_BYTES=allow` opt-in enables authenticated REST
  `GET /api/v1/artifacts/{id}/bytes` with a daemon-side size cap, and
  `agt peers artifact-get <peer> <artifact_id> <out_file>` writes the decoded
  bytes to a local file without printing raw bytes to the terminal.
- Done: Run Detail extracts mirrored peer artifact metadata into a compact
  Remote Artifacts panel and provides copyable `agt peers artifact-get`
  commands instead of embedding remote bytes in the UI.
- Done: `agt peers run <peer> <correlation>` fetches the same remote run event
  arc on demand and prints metadata-only drill-down output; remote payloads and
  peer tokens are not printed.
- Done: `agt run --exec-profile remote-agezt --peer <name>` pins whole-run
  delegation to a configured peer instead of relying on single-peer default or
  model-based auto-routing; dry-run reports the selected remote peer.
- Done: `AGEZT_EXEC_REMOTE_SECRET_POLICY` exposes the remote/cloud secret
  handoff contract in Config Center, Execution Profiles UI, inventory JSON, and
  `agt exec-profile check`. Default/blank denies local secret values and
  metadata; `metadata` allows only names/labels for future adapters while still
  denying values.
- Done: `agt exec-profile check` / `execution_profile_check` reports profile
  routing, warden downgrade state, docker/podman/ssh/kubectl backend
  availability, and peer-run readiness for `remote-agezt`.
- Done: `modal`, `daytona`, and `k8s` appear in the same inventory/health
  surface, check their CLI availability, and become selectable only when their
  profile env is configured.
- Done: `AGEZT_EXEC_PROFILE_ALLOW` / `AGEZT_EXEC_PROFILE_DENY` provide
  explicit execution-profile allow/deny policy; denied profiles are rejected at
  run submission and omitted from health-reported selectable run profiles.
- Done: Config Center schema and the Execution Profiles screen expose editable
  live run-profile policy controls for allow/deny.
- Done: Config Center schema and the Execution Profiles screen expose editable
  SSH, K8s, Modal, and Daytona backend settings live, plus Docker/OCI backend
  settings with a restart boundary.
- Done: Config Center schema exposes `AGEZT_PEERS` / `AGEZT_TENANT_PEERS` as
  restart-bound secret settings, so `remote-agezt` peer mesh tokens are not
  echoed through the UI/API.
- Done: `AGEZT_EXEC_ENV_LOCAL` / `AGEZT_EXEC_ENV_WARDEN` /
  `AGEZT_EXEC_ENV_DOCKER` and matching `AGEZT_EXEC_SECRET_ENV_*` settings
  provide live, profile-specific env passthrough for local/warden/docker
  shell/code_exec child processes; non-secret lists reject secret-shaped names,
  secret-env lists reject `AGEZT_*`, and Docker receives the same scrubbed
  `Spec.Env` through OCI `-e`.
- Done: `AGEZT_EXEC_SECRET_FILES_LOCAL` / `AGEZT_EXEC_SECRET_FILES_WARDEN` /
  `AGEZT_EXEC_SECRET_FILES_DOCKER` mount configured vault keys as temporary
  files for local/warden/docker shell/code_exec runs, expose only
  `SECRET_FILE_<KEY>` path env vars to the child process, map Docker paths to
  `/workspace/.agezt-secrets/...`, and remove the files after each tool call.
- Done: Chat composer can choose tool-default plus the currently routable run
  profiles for the conversation and sends selection through the governed
  `/api/run` path.
- Done: Web UI has a dedicated Execution Profiles screen that joins inventory,
  selectable run profiles, requested/effective isolation, degraded states, and
  health checks in one operator view.
- Still needed: add K8s job lifecycle,
  and extend profile routing to file writes, browser, coding, and workflow tool
  nodes.
- Still needed: richer local streaming of remote peer artifacts only
  where the remote/cloud secret and data policy allows it.

Acceptance criteria:

- Same task can run under local, docker, and ssh profiles with consistent
  journal/effect metadata.
- Secret passthrough is explicit, redacted, and denied by default.
- Policy can deny a high-risk execution profile even if the tool is allowed.

### Phase 6: Gateway, Device, And Companion Layer

Goal: remove the "headless daemon" feel and cover OpenClaw's personal device
experience.

Build:

- Node registry: every AGEZT daemon, browser node, mobile/tray companion, and
  remote worker reports status/capabilities.
- Tray app or lightweight local companion:
  - start/stop daemon
  - provider/channel health
  - approvals
  - voice push-to-talk
  - notifications
  - tunnel status
- Mobile/PWA companion:
  - approvals
  - inbox/alerts
  - voice messages
  - run status
  - share sheet/webhook target
- Device routing policy: which node can run browser, shell, desktop automation,
  Home Assistant, media generation, or channel sends.

Acceptance criteria:

- Operator can approve/deny and inspect run state without opening a terminal.
- A task can be routed to a node by capability and policy, not hard-coded host.

### Phase 7: Marketplace Trust And Distribution

Goal: turn AGEZT registry support into a safer ClawHub/Hermes hub equivalent.

Build:

- Unified marketplace UI for skills, plugins, MCP servers, channel adapters,
  execution profiles, and workflow templates.
- Trust card per package:
  - publisher identity
  - signature status
  - BLAKE3/content hash
  - permissions requested
  - files included
  - install scripts
  - network domains
  - update policy
  - scanner findings
  - last audit timestamp
- Quarantine flow for installed packages.
- Pin/update UX: exact version, semver range, update all, dry-run update.
- Optional ClawHub/agentskills import adapter, with AGEZT scanner before install.

Acceptance criteria:

- Installing an untrusted skill/plugin requires explicit operator consent.
- A package can be installed, updated, rolled back, or quarantined through CLI
  and UI with audit evidence.

### Phase 8: Productized Demos And Migration

Goal: make superiority visible in minutes.

Build these demos:

- "OpenClaw parity": chat channel -> browser action -> message with artifact ->
  schedule follow-up -> memory update -> audit chain.
- "Hermes parity": checkpointed repo edit -> rollback -> skill proposal ->
  curator -> durable workboard pipeline.
- "AGEZT advantage": multi-agent workboard + typed schedules + Edict policy
  denial + `agt why` + rollback + SDK/API client.
- Importers:
  - OpenClaw workspace memory/skills/standing orders where possible
  - Hermes `MEMORY.md`, `USER.md`, `SOUL.md`, skills, cron jobs, profiles
  - AGENTS.md/CLAUDE.md/Cursor rules as project context, with injection scan

Acceptance criteria:

- A new user can run one command and see an end-to-end local demo without
  external paid services, using mock providers where necessary.
- Migration commands are dry-run first and never overwrite without backup.

## Immediate 30-Day Plan

1. Ship the parity ledger.
   - Add `docs/COMPETITOR-PARITY.md`.
   - Add static audit fixtures for OpenClaw/Hermes capability rows.
   - Add `agt compare audit` as a read-only command.

2. Start browser parity.
   - Implement first-class `browser.open/snapshot/click/type/wait/screenshot/downloads/close`
     against Playwright when available, with AGEZT-managed `profile=session`
     state carryover plus persistent `tab_id` URL/snapshot refs.
   - Keep `browser.read` as the safe default text reader.
   - Gate interactive browser control at higher trust than read-only fetch.

3. Productize Forge Workshop.
   - Done: add proposal CLI around existing `kernel/skill` lifecycle.
   - Done: add deterministic scanner output to CLI proposal inspect/scan.
   - Done: add Web UI proposal list/apply/reject and scanner chips.
   - Done: add Web UI parent diff preview and basic hash/source/parent chips.
   - Done: add deterministic stale-skill curation CLI (`--execute` quarantines).
   - Add scanner output to proposal metadata and richer provenance cards.

4. Define checkpoint schema before adding broad rollback.
   - Start with file/apply_patch/workflow/skill/config mutations.
   - Explicitly mark shell/codeexec as non-generic rollback until profile-level
     snapshots exist.

5. Draft Workboard kernel design.
   - Reuse board/mailbox/workflow/journal concepts.
   - Do not clone Hermes Kanban mechanically; AGEZT should preserve identity,
     authority, and wake causality.

## What Not To Do

- Do not bury parity behind skills that are not visible in the tool catalog.
- Do not claim full rollback for shell, browser posts, network calls, or external
  services.
- Do not add another prompt-shaped task list. Workboard tasks must be typed
  state-machine objects.
- Do not treat marketplace installs as trusted because they came from a popular
  registry.
- Do not weaken AGEZT's policy/journal model to copy a faster competitor UX.

## Strategic End State

AGEZT wins when a user can say:

- It can run through the same chat, browser, terminal, skill, cron, gateway, and
  multi-agent workflows as OpenClaw and Hermes.
- It gives me better authority control before action.
- It gives me better causality, provenance, and rollback after action.
- Its agents are durable identities, not just sessions, profiles, or task rows.
- Its marketplace and skill evolution are governed, inspectable, and reversible.
