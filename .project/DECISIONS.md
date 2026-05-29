# Agezt — Frozen Decisions (DECISIONS.md)

> Status: Frozen v1.0 · Language: English
> Closes every open question across the spec suite so implementation can begin without ambiguity. Each decision is final for v1; revisit only with a documented superseding decision.

---

## A. Project-level

- **A1. Name:** **Agezt**. Binary `agezt`, CLI `agt`, env prefix `AGEZT_`, config dir `~/.agezt/`. The name string lives in one constants file (`internal/brand`) for any future rename.
- **A2. License:** **MIT**. Open source. `LICENSE` at repo root; SPDX headers (`// SPDX-License-Identifier: MIT`) in source files.
- **A3. Repo/domain:** TBD (owner decides later); never hardcoded — referenced via the brand constants file.
- **A4. Language/runtime:** Go (latest stable) for kernel/plugins/SDK-go; TypeScript + React 19 + Vite for the Web UI; SDKs in Go/TS/Python/Rust. `CGO_ENABLED=0` for all core binaries.

## B. Contracts & identity

> **Foundational revision (supersedes earlier gRPC / all-out-of-process / fold-everything choices).** These were changed deliberately to keep the base small, fast, and debuggable. Earlier specs that mention gRPC/protobuf or "all plugins out-of-process" defer to B0–B0d.

- **B0. Transport:** plugin/kernel transport is **stdio + JSON-RPC 2.0** with newline-delimited JSON for streaming — **not gRPC/protobuf**. Rationale: zero heavy deps, human-readable/debuggable, shares one wire format with ACP (SPEC-15) so plugins and ACP speak the same protocol. Contracts defined as **JSON Schema** → per-language SDK types. gRPC is out of the base.
- **B0a. Plugin execution model:** **default in-process; out-of-process only when isolation is required.** Core trusted tools (shell/file/http) run in-process for speed and easy debugging. Untrusted/third-party/heavy plugins run as subprocesses over stdio/JSON-RPC with crash isolation. Isolation is paid where needed, not always.
- **B0b. Contract scope:** the base contract starts **minimal** — `Event`, the Kernel callback API, and the `Tool` + `Provider` interfaces. The other five interfaces (Channel/CodingAgent/Memory/Storage/Tunnel) and the full event-kind set are added append-only as those layers are built. Don't freeze the whole surface before code exists.
- **B0c. Persistence model:** the **event log is the audit/replay/revert truth**, but a **mutable state store is first-class** (not merely a cache). Frequently-read state is read directly; the log guarantees auditability/reproducibility/reversibility. The "all state is a fold" rule is relaxed.
- **B0d. Orchestration layering:** the base is a **single-agent tool-loop orchestration core — written entirely by us, depending on no third-party agent SDK** (a deliberate, non-negotiable decision; external SDKs would constrain us). The DAG scheduler is a **second layer** on top of that loop, not part of the base. All LLM orchestration (tool-loop, tool-calling normalization, context management, provider abstraction, Pulse) is 100% first-party.

- **B1. Protocol version:** integer major, starts at `1`. Proto fields append-only; enum values never renumbered; removed fields `reserved`.
- **B2. IDs:** **ULID** for all time-ordered entities (events, agents, tasks, sessions, plugins-instances, skills-instances, memory records, jobs, migrations, standing orders). **BLAKE3 content-address** for immutable content (skill bodies, artifacts, snapshots, image/plugin digests). Kernel assigns IDs; plugins never do.
- **B3. Event hashing:** canonical encoding = protobuf serialized with deterministic field ordering, then BLAKE3. `hash = BLAKE3(prev_hash || canonical_bytes)`.
- **B4. Sub-agent result:** **typed** `AgentResult { status, summary, artifacts[], outputs(map), error? }` — not free-form.
- **B5. Attachment threshold:** payloads/attachments **≤ 32 KiB inline** in events; larger → content-addressed blob in Storage, referenced by digest. (32 KiB chosen to keep journal segments lean.)
- **B6. Cross-major migration:** require plugin rebuild on major bump (no translation shim in v1); kernel rejects mismatched majors at registration.

## C. Governor & LLM

- **C1. Budget unit:** **USD micro-cents (integer)** as the canonical internal unit; providers report native tokens, Governor converts via a per-model price table. (Integer avoids float drift; USD is the unit humans reason about.) `EVT_BUDGET_CONSUMED` carries both native tokens and computed micro-cents.
- **C2. Routing default order:** subscription-first → quality-floor-for-task-type → cost → latency. Local provider is always an eligible fallback floor.
- **C3. Borderline escalation:** if a structured-output validation fails or the model self-reports low confidence, re-route once to the next-stronger eligible model; journaled; no infinite escalation.
- **C4. Summarization/salience model:** default to the cheapest capable model; configurable per deployment.
- **C5. Embeddings:** local embeddings by default (zero marginal cost); provider embeddings opt-in; budgeted separately from completions.
- **C6. Context budget defaults:** start at 50% of the model's context window as the compression trigger; protect first 3 + last 4 turns; tune empirically (parameters in config).

## D. Storage & persistence

- **D1. Default journal:** segmented JSONL, **64 MiB** segment rotation, sidecar index (`offset,seq,hash`), `fsync` on batch boundary (durable-before-ack). Snapshot every **10,000 events** or 1h, whichever first.
- **D2. Default KV/state:** CobaltDB (embedded B+Tree). **Default vector index:** embedded pure-Go HNSW for semantic memory; Flint Vector plugin for scale. **World-model graph:** adjacency lists in CobaltDB for v1; graph-DB driver optional later.
- **D3. Pluggable:** Postgres (journal+state), Redis (cache), Flint Vector (semantic) behind Storage/Memory contracts.
- **D4. Journal driver consistency requirement:** append atomicity + total order per journal + durable-before-ack. Drivers failing this may serve KV/memory only.

## E. Kernel runtime

- **E1. Actor model:** goroutine + bounded mailbox (default capacity **256**, backpressure on full). Lightweight supervision (no full OTP tree v1).
- **E2. Restart policy default:** `on_crash`, max 5 restarts / 60s window, exponential backoff (base 500ms, factor 2, max 30s, jitter), then quarantine + high-salience event.
- **E3. Agent checkpoint granularity:** per-node-boundary (resume from last completed node via journal fold by `correlation_id`).
- **E4. DAG worker pool:** default **8** concurrent nodes; path-scoped serialization for nodes touching the same resource.
- **E5. loop-node bounds:** default max **25** iterations + a per-task budget ceiling; both configurable, both hard-stop.

## F. Security

- **F1. Default isolation:** tools → `namespace`; browser & untrusted/third-party → `container`; first-party WASM read-only → `none`. microVM optional, separately built (protects lean core).
- **F2. Egress:** default-deny; per-capability allow-list in Edict.
- **F3. Trust ladder defaults:** `shell:L2, file:L2, http:L1, browser:L1, channel.send:L1, coding.merge:L1, purchase:L0`, provider spend ceiling **$20/day** default. Reflection may lower autonomy autonomously, never raise.
- **F4. Hard-deny (non-raisable):** secret exfiltration, audit disable, destructive delete outside workspace, fork-bomb / `rm -rf /` class commands.
- **F5. Secrets:** AES-256-GCM at rest; scoped short-lived issuance to plugins; redaction on by default; OAuth via PKCE; passwords never typed on user's behalf.
- **F6. Anomaly auto-halt:** default thresholds — >300 tool-calls/5min, >$5 spend/5min, >50% error rate/5min, or same autonomous action repeated >3×. Configurable.
- **F7. Docker sandbox mode default:** **sibling containers** via controlled runtime access; socket-mount and DinD are opt-in alternatives, documented with their privilege implications.

## G. CLI & UI

- **G1. CLI framework:** **minimal custom command router** (zero external dep) to honor POLICY §1; not Cobra. TUI uses Bubble Tea + Lip Gloss (justified deps).
- **G2. Generated types:** **build-time generation from JSON Schema** (the contract source per B0) into per-language SDK types; CI verifies in sync. (Supersedes the earlier protobuf/buf codegen choice.)
- **G3. Widget isolation:** **iframe + strict CSP** (stronger isolation than shadow-DOM for untrusted-data rendering).
- **G4. Standing-order authoring:** visual in Flow Studio **compiles to a declarative YAML** form (both; visual is the front-end to the DSL).

## H. Scope discipline

- **H1. v1 build target = the MVP cut (VISION §17) first**, then phases 2→9. The full suite is the destination; the MVP is the first shippable, demoable product.
- **H2. Non-goals (v1):** full OTP supervision tree, LLM training/RL, multi-node mesh implementation (contracts only), multi-tenant (single-instance RBAC only).

---

*These decisions unblock all "Open Questions" sections in SPEC-01..14 and IMPLEMENTATION. Where a spec's open question conflicts with this file, this file wins.*
