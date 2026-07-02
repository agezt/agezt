# AGEZT Competitor Parity Ledger

Date: 2026-07-01

This is the checkable companion to
[`OPENCLAW-HERMES-ROADMAP.md`](OPENCLAW-HERMES-ROADMAP.md). It records what AGEZT
already covers from OpenClaw and Hermes Agent, what is partial, and what is not
present yet. The ledger is intentionally evidence-backed: every supported or
partial row maps to local repository paths.

Run:

```bash
agt compare audit
agt compare audit --target openclaw
agt compare audit --target hermes --json
```

The command is read-only and offline. It walks from the current directory to the
AGEZT repo root, then verifies the local evidence paths.

## Current Summary

As of this checkout:

| Target | Total | Supported | Partial | Missing | Evidence missing |
|---|---:|---:|---:|---:|---:|
| all | 19 | 12 | 6 | 1 | 0 |
| openclaw | 16 | 11 | 4 | 1 | 0 |
| hermes | 17 | 12 | 5 | 0 | 0 |

Status meanings:

- `supported`: AGEZT has a local implementation surface and evidence paths.
- `partial`: AGEZT has a real foundation, but not the same productized UX or
  complete parity surface.
- `missing`: AGEZT does not currently have the named product capability.

## Ledger

| ID | Targets | Status | Meaning | Next move |
|---|---|---|---|---|
| `channels-gateway` | OpenClaw, Hermes | supported | Broad messaging gateway with channel plugins, Connect docs, Channels UI, and a configured/live/roundtrip readiness matrix in API/CLI/Web UI. | Add per-adapter setup recipes and optional safe live roundtrip probes. |
| `provider-api-gateway` | OpenClaw, Hermes | supported | Provider families plus OpenAI-compatible, REST, and ACP APIs. | Keep SDK/model-routing parity tested. |
| `durable-agent-identity` | OpenClaw, Hermes | supported | Durable roster identities with lifecycle, authority, ownership, and routing. | Add profile importers. |
| `memory-world` | OpenClaw, Hermes | supported | Memory plus entity/relation world model and audit commands. | Add MEMORY.md/USER.md/SOUL.md import/export. |
| `skill-lifecycle-core` | OpenClaw, Hermes | supported | Content-addressed skill lifecycle with lineage, shadow, quarantine, archive, revert, CLI workshop gates, deterministic proposal scanning, parent diffs, and stale-skill curation. | Add stronger provenance cards and optional LLM consolidation. |
| `skill-workshop-ux` | OpenClaw, Hermes | partial | `agt skill workshop list/inspect/scan/diff/curate/apply/reject/quarantine/propose-create/propose-update` exists over Forge; Web UI pending-proposal apply/reject, scanner risk chips, parent diff previews, and hash/source/parent chips exist; optional LLM consolidation is not complete. | Add richer provenance cards and optional LLM consolidation jobs. |
| `browser-actions` | OpenClaw, Hermes | partial | Safe browser read plus opt-in first-party `browser.action` and `browser.open/snapshot/click/type/wait/screenshot/downloads/cookies/tabs/close` wrappers exist with compact snapshots, browser events, screenshot/download artifacts, cookie inspection on request, download capture, AGEZT-managed `profile=session` cookie/storage carryover, persistent `tab_id` final-URL refs for URL-less follow-up actions, saved snapshot `ref` resolution with missing-ref errors, saved tab-ref list/close lifecycle, and operator-gated isolated/user-attached/remote-cdp profile policy; live browser-process tab lifecycle and DOM-level stale-ref invalidation are not complete. | Add live browser-process tab lifecycle, DOM stale-ref invalidation, and full Playwright E2E browser fixtures. |
| `automation-schedules` | OpenClaw, Hermes | supported | Typed schedules, standing orders, workflows, and schedule/standing tools. | Add import/demos for cron and standing definitions. |
| `mcp-tool-discovery` | OpenClaw, Hermes | supported | MCP bridge, MCP registry, tool catalog, toolforge, and runtime tool search. | Make deferred/policy-aware discovery a named feature. |
| `marketplace-registry` | OpenClaw, Hermes | supported | Remote market, skill, and plugin registry flows with verification primitives. | Unify trust cards. |
| `marketplace-trust-ux` | OpenClaw, Hermes | partial | Verification exists; full scanner-backed risk-card UX is incomplete. | Add package trust cards and quarantine/update flows. |
| `checkpoint-rollback` | Hermes | supported | `agt rollback list/show/dry-run/apply` restores workshop skill status checkpoints, workflow snapshots, daemon file snapshots, and `agt config set` snapshots; `agt rollback list --run <id>` groups run-scoped file checkpoints; Run Detail has a rollback drawer; irreversible tools are labeled `audit only`; skill/workflow restores are journaled as `skill.restored` / `workflow.restored`. | Broaden checkpoint hooks to patch/coding/package-update paths and add agent-detail grouping. |
| `durable-workboard` | Hermes | partial | Durable typed `kernel/workboard` store, runtime/control-plane mutations, journaled task events, `agt workboard` CLI including lanes/depend/reclaim/sweep/policy/fail/dispatch/watch, agent-facing `workboard` tool, and separate Edict capability exist for tasks, dependencies, comments, links, claims, heartbeats, idempotency keys, retry/escalation policy, status transitions, async roster-agent dispatch, automatic failed-attempt retry, exhausted-attempt escalation, run correlation links, review handoff, task/run event watch, dependency-gated dispatch, assignee lane grouping in CLI/API/Web UI, dedicated Workboard detail UI for dependencies, attempts, links/artifacts, events, comments, and operator actions, manual stale-claim reclaim, and optional timer-based stale-claim sweep via `AGEZT_WORKBOARD_SWEEP_EVERY`. | Add graph-style dependency visualization, inline artifact/diff preview, and process/delegation heartbeat integration. |
| `execution-profiles` | Hermes | partial | Warden, netguard, codeexec/shell, worktree coding, browser sessions, Docker/SSH skills, remote peers, active K8s pod shell/code_exec plus Modal shell/code_exec and Daytona shell/code_exec adapters now have a unified `kernel/executionprofile` inventory with requested/effective isolation, routed tools, filesystem/network/env/secret/limit semantics, structured remote/cloud `AGEZT_EXEC_REMOTE_SECRET_POLICY` reporting, control-plane/Web API routes, `agt exec-profile list\|show\|check`, `agt run --exec-profile local\|warden\|docker\|ssh\|k8s\|modal\|daytona\|remote-agezt` per-run routing for shell/code execution when Docker/SSH/K8s/Modal/Daytona backends are explicitly configured, local/SSH/K8s `code_exec` `.agezt-artifacts/` export into the durable artifact index, Modal shell/code_exec routing through `modal shell --cmd` with `--add-local` workspace mounts for code_exec and bounded artifact return, Daytona shell/code_exec routing through `daytona exec` with bounded workspace materialization and bounded artifact return, and whole-run peer delegation when `remote_run` is registered, `agt run --exec-profile remote-agezt --peer <name>` peer pinning for multi-node meshes, live `AGEZT_EXEC_PROFILE_ALLOW` / `AGEZT_EXEC_PROFILE_DENY` run-profile policy, live SSH/K8s/Modal/Daytona backend controls, restart-bound Docker/OCI and peer-mesh controls, live profile-specific env/secret-env passthrough controls and vault-backed temporary secret file mounts for local/warden/docker shell/code_exec child processes, Config Center and Execution Profiles UI controls, Chat UI execution-profile selection, dedicated Execution Profiles inventory/health UI, optional Docker/OCI warden routing, SSH shell/code_exec remote workspace routing, K8s shell/code_exec routing with scrubbed local kubectl env, remote workspace copy, artifact copy-back, and no daemon secret forwarding into pods, Modal shell/code_exec and Daytona shell/code_exec routing with scrubbed local CLI env and no daemon secret forwarding into cloud sandboxes, remote AGEZT peer routing through policy-gated `Kernel.RunTool` with local task lifecycle events, structured peer correlation metadata, `agt peers run <peer> <corr>` metadata-only remote run drill-down, `agt peers artifacts <peer> <corr>` metadata-only remote artifact drill-down, opt-in metadata/redacted payload peer event mirroring via `AGEZT_REMOTE_EVENT_MIRROR=metadata|redacted`, peer artifact metadata mirroring through `/api/v1/artifacts?corr=...` without artifact bytes, opt-in policy-gated artifact byte transfer via `AGEZT_REMOTE_ARTIFACT_BYTES=allow`, REST `/api/v1/artifacts/{id}/bytes`, `agt peers artifact-get <peer> <artifact_id> <out_file>`, and Run Detail remote artifact summaries with copyable governed download commands, plus health checks for policy/downgrade/docker/podman/ssh/peer/modal/daytona/kubectl/remote-secret-policy/remote-artifact-bytes readiness; K8s job lifecycle is not complete yet. | Add K8s job lifecycle. |
| `media-voice` | OpenClaw, Hermes | supported | STT/TTS, voice mode, image provider, artifact store, media send tool, media-capable channel adapters, and image/voice in/out capability matrix in API/Web UI. | Add artifact-native media review flows in the console. |
| `device-companion` | OpenClaw | partial | Web UI, Home Assistant, tunnel, voice, computer/browser skills, peers, `agt peers`, and node registry API/Web UI exist; native tray/mobile companion distribution does not. | Build tray/PWA/mobile companion on top of the node registry. |
| `onboarding-console` | OpenClaw, Hermes | supported | Quickstart, Setup, Config Center, Channels, Connect docs, and Web UI console. | Add one-pass validation for provider/channel/MCP/skill/sandbox. |
| `audit-policy-safety` | OpenClaw, Hermes | supported | Edict, approvals, hash-chain journal, netguard, warden, vault, and `agt why`. | Require policy/effect/provenance events for all parity features. |
| `native-mobile-tray` | OpenClaw | missing | No native tray/mobile companion distribution today. | Build the tray/PWA/mobile companion product layer on top of node registry. |

## Evidence Contract

The ledger should stay conservative:

- Do not mark a row `supported` without implementation files, tests, commands, or
  docs that demonstrate the claim.
- Keep partial rows partial until the user-facing path is productized, not merely
  possible through a hidden skill or manual workaround.
- If a local evidence path disappears, `agt compare audit` must report
  `evidence_missing > 0`.
- Competitor names should be used only for parity tracking. Product positioning
  should still lead with AGEZT's own model: durable identities, authority,
  wake causality, audit, and reversible governance.
