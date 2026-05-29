# Agezt — Security, Sandbox & Warden Specification (SPEC-06)
> ⚠️ **AUTHORITY NOTICE:** This document is subordinate to `DECISIONS.md`. Where anything here conflicts with DECISIONS, **DECISIONS wins** — especially the foundational revisions **B0 (transport = stdio + JSON-RPC 2.0, NOT gRPC/protobuf)**, **B0a (plugins default in-process; out-of-process only for isolation)**, **B0b (minimal contract, grows append-only)**, **B0c (mutable state store is first-class alongside the event log)**, and **B0d (DAG is a second layer over a first-party single-agent tool-loop)**. Any mention of gRPC, protobuf, or "all plugins out-of-process" in this file is superseded. The contract source of truth is `agezt-contract.jsonc`.


> Status: Draft v0.1 · Language: English · Domain/Repo: TBD
> Depends on: SPEC-01, SPEC-02 (Edict, control plane), SPEC-04 (plugin isolation)
> Defines the threat model, isolation profiles (Warden), the Edict policy engine in depth, secret handling, and the safety guarantees for autonomous operation. OpenClaw shipped autonomy without this and had a security crisis; Agezt treats security as core, not optional.

---

## 0. Security philosophy

- **Default-deny, escalate-by-default for sensitive actions.** The system starts cautious; autonomy is *earned* via the trust ladder, never assumed.
- **The user is always in control.** A single command (`agt halt`) freezes everything without data loss. Nothing irreversible happens without an auditable decision.
- **Everything is observable.** Every security decision is a journaled event (`EVT_POLICY_DECISION`, approvals). `agt why` reconstructs any action's authorization chain.
- **Untrusted input stays data.** Channel messages, web content, tool output, and plugin results are data, never kernel instructions. Instruction-like content cannot self-authorize privileged actions.
- **Least privilege everywhere.** Plugins are processes with the minimum isolation/permissions needed; secrets never leave the kernel boundary.

---

## 1. Threat model

### 1.1 Assets to protect
- User credentials (provider API keys, channel tokens, OAuth refresh tokens).
- The journal integrity (the audit truth).
- The user's external accounts and systems (no unauthorized actions on their behalf).
- The host machine (no arbitrary code escaping sandboxes).
- The user's attention and trust (no runaway autonomy, no spam, no harmful action).

### 1.2 Adversaries / risks
- **Prompt injection** via channels, web pages (browser tool), emails, file contents, or MCP server output — attempting to make the agent exfiltrate data or take harmful actions.
- **Malicious / buggy third-party plugins** — attempting to read secrets, escape sandbox, or flood the bus.
- **Runaway autonomy** — the agent (especially Pulse/Initiative) acting too much, too expensively, or in a loop.
- **Compromised provider/endpoint** — a model output engineered to trigger unsafe tool calls.
- **Supply chain** — tampered plugin binaries or skills.

### 1.3 Out of scope (documented assumptions)
- Physical access to the host. Host OS compromise below the kernel. (Agezt protects within the host trust boundary; full microVM isolation raises the bar but a rooted host is game over.)

---

## 2. Warden — isolation profiles

The Warden enforces *how* a tool/plugin process runs. Four profiles, selected by Edict/manifest per call:

| Profile | Mechanism | Use for |
|---|---|---|
| `none` | in-process / no isolation | trusted first-party WASM tools, read-only ops |
| `namespace` | Linux namespaces + cgroups (PID/mount/net/user) + seccomp | default for shell/file/http |
| `container` | OCI container, mounted scope only, no host net by default | untrusted third-party tools, generated code |
| `microvm` | lightweight VM (e.g. firecracker-class) | highest-risk / untrusted execution |

### 2.1 What each profile constrains
- **Filesystem:** only explicitly mounted paths are visible/writable. Default workspace is scoped; host root is never mounted.
- **Network:** default-deny egress; allow-list per Edict (e.g. a tool may reach `api.github.com` but not arbitrary hosts).
- **Resources:** CPU/memory/time limits via cgroups; runaway tools are killed (and journaled).
- **Syscalls:** seccomp profiles restrict dangerous syscalls in `namespace`+.

### 2.2 Native implementation
Linux namespaces + cgroups + seccomp implemented natively (the team's Rampart/Karadul experience), Docker/OCI optional for `container`, a microVM backend optional for `microvm`. On non-Linux hosts, the strongest available mechanism is used and the profile downgrade is journaled and surfaced (so the user knows the actual isolation level).

### 2.3 Browser tool specifics
The browser is a major injection surface. It runs in `container` by default, with: domain allow/deny via Edict, no access to host credentials/cookies unless explicitly provisioned, screenshots/extraction returned as data, and sensitive domains (banking, etc.) escalate to approval. Bot-detection/CAPTCHA is never auto-bypassed.

---

## 3. Edict — policy engine in depth

### 3.1 Evaluation points
Edict evaluates at **every kernel→plugin boundary** and **every gate-node**. Nothing privileged happens without passing Edict. Each evaluation → `EVT_POLICY_DECISION`.

### 3.2 Policy structure
```yaml
version: 1
# Immutable hard limits — NOT raisable by trust level.
hard_deny:
  - match: { action: exfiltrate_secret }
  - match: { tool: file, op: delete, scope: outside_workspace }
  - match: { action: disable_audit }
  - match: { tool: shell, command_glob: ["rm -rf /", ":(){ :|:& };:"] }

# Default posture
defaults:
  isolation: namespace
  egress: deny
  approval: none

# Capability trust levels (the ladder; user raises over time)
trust:
  shell:        L2   # act-reversible
  browser:      L1   # propose; act needs approval
  channel.send: L1   # send on behalf needs approval
  coding.merge: L1
  purchase:     L0   # observe/propose only by default
  provider.spend: { ceiling_usd_per_day: 20 }

# Rules (first match wins; escalate = require human approval)
rules:
  - match: { provider: "*", cost_usd_gt: 5 }            ; decision: escalate
  - match: { tool: browser, domain: ["*.bank.*","*account*"] } ; decision: escalate
  - match: { channel: "*", action: send_on_behalf }     ; decision: escalate
  - match: { tool: http, domain_not_in: allowlist }     ; decision: deny
  - match: { node: coding-node, action: ["merge","force_push"] } ; decision: escalate
  - match: { action: open_tunnel, scope: public }       ; decision: escalate
```

### 3.3 Trust ladder (recap from SPEC-02, with mechanics)
```
L0 observe-only     — read/propose only
L1 propose          — draft/plan; execution requires approval
L2 act-reversible   — reversible actions autonomous; irreversible escalate
L3 act-bounded      — autonomous within budget/scope ceilings
L4 trusted          — broad autonomy; only hard_deny applies
```
- The ladder is **per-capability** — you can trust `shell` at L3 while keeping `purchase` at L0.
- The **reflection loop can lower** autonomy on its own but can **never raise** it; raises are always user-approved.
- Initiative (Pulse) reads the ladder to decide solve-vs-ask (SPEC-03 §5).

### 3.4 Approvals
`escalate` → pause node → `EVT_APPROVAL_REQUESTED` → routed to user via preferred Channel with inline affordances → `EVT_APPROVAL_GRANTED|DENIED` resumes/aborts. Approvals can be scoped ("allow this once" / "allow for this task" / "raise trust for this capability"). Time-outs default to deny.

---

## 4. Secret handling

- **Secrets live only in the kernel's Conduit**, encrypted at rest (AES-256-GCM / ChaCha20-Poly1305; team's Kronos crypto experience).
- **Plugins never receive raw long-lived keys.** Provider plugins get scoped, short-lived auth at call time, or the kernel proxies the authenticated call. A compromised plugin process cannot read keys off disk.
- **Redaction:** a `RedactingFormatter` masks secrets (API keys, tokens, auth headers, connection-string passwords, private-key blocks, phone numbers) in all logs and tool output before they enter the journal or reach a provider. Short tokens fully masked; long tokens keep a recognizable prefix/suffix.
- **No secrets in events/URLs/state.** The journal and any URL the system constructs are scrubbed; `EVT_*` payloads pass through redaction.
- **OAuth flows** (subscriptions) use PKCE; refresh tokens stay in the Conduit. The user performs the actual login; the system never types passwords into forms on the user's behalf.

---

## 5. Autonomous-operation safety (the riskiest mode)

Proactive + autonomous (Pulse Initiative) gets extra guards beyond Edict:

- **Action rate limiting:** a ceiling on autonomous actions per window; exceeding forces everything to `ask` and raises an anomaly.
- **Anomaly auto-halt:** detectors watch tool-call rate, spend rate, error rate, and repetition. A spike auto-engages `agt halt` and notifies the user. (Watches Pulse's own rate too.)
- **No-repeat guard:** Initiative will not repeat the same autonomous action without escalating (prevents thrash/loops).
- **Reversibility requirement:** if Initiative can't identify a reversal path for an action, it downgrades `act`→`ask`.
- **Budget ceilings:** Governor caps per-task and per-day spend; loop-nodes honor ceilings; breach → stop + surface.
- **Dead-man's switch:** `agt halt` suspends all agents + Pulse + scheduler, persists state, keeps journal/host alive. `agt resume` restores from journal. Optional heartbeat-based auto-halt (if the operator's presence check fails for high-autonomy deployments).

---

## 6. Plugin supply-chain security

- **Signed, content-addressed plugins.** `agt plugin add` verifies signature + hash; unsigned/unknown plugins require explicit `--trust` and run in `container`/`microvm` by default.
- **Capability scoping at install.** A plugin only gets the capabilities and isolation declared in its manifest and approved at install; it cannot escalate at runtime.
- **Marketplace artifacts** (skills, workflows, standing orders) are likewise signed/versioned; importing a standing order that includes autonomous actions surfaces its required trust levels for approval before activation.

---

## 7. Audit & forensics

- **Tamper-evident journal:** BLAKE3 hash chain; `agt journal verify` proves no event was altered/removed.
- **Full provenance:** `causation_id` chains let `agt why <event>` reconstruct: trigger → salience → plan → policy decision → action → outcome.
- **Decision log:** every allow/deny/escalate and every approval is journaled with the matched rule and trust level.
- **Replayable:** any incident can be reconstructed by replaying the journal to the relevant sequence.

---

## 8. Privacy (cross-ref SPEC-05 §8)

- PII redaction before external calls (configurable).
- Local-first mode: local provider + embedded storage + local embeddings = no egress.
- Per-entity sensitivity in the world model restricts which providers/tools may see marked data.
- Cookie/consent handling: the system chooses the most privacy-preserving option; accepting terms/agreements is an escalate action.

---

## 9. Secure defaults checklist (what ships on day one)

- Egress default-deny; allow-list required.
- Tools default to `namespace`; browser to `container`.
- Trust ladder defaults: read/propose only for send-on-behalf, purchase, merge, public tunnels.
- Provider spend ceiling on by default.
- Secret redaction on by default.
- Anomaly auto-halt on by default.
- `agt halt` always available, even mid-action.

---

## 10. Open questions

1. microVM backend choice and its footprint vs the "single static binary / $5 VPS" goal — ship as optional plugin?
2. Cross-platform isolation: how much can we guarantee on macOS/Windows-WSL vs Linux; how loudly to warn on downgrade.
3. Approval UX latency: how to make escalation fast enough that L1–L2 autonomy still feels responsive.
4. Heartbeat dead-man's switch: opt-in for high-autonomy deployments — default off?
5. Anomaly thresholds: fixed defaults vs learned via reflection (and how to prevent reflection from loosening safety).

---

*Next: SPEC-07 (UI & Surfaces) — Flow Studio (React Flow), Unified Inbox, Live Monitor, Memory Explorer, ambient surfaces, and the gateway/API. Then the build docs: IMPLEMENTATION → TASKS → BRANDING → README → PROMPT.md.*
