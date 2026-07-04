# Agezt — Deployment & Runtime Environments Specification (SPEC-11)
> ⚠️ **AUTHORITY NOTICE:** This document is subordinate to `DECISIONS.md`. Where anything here conflicts with DECISIONS, **DECISIONS wins** — especially the foundational revisions **B0 (transport = stdio + JSON-RPC 2.0, NOT gRPC/protobuf)**, **B0a (plugins default in-process; out-of-process only for isolation)**, **B0b (minimal contract, grows append-only)**, **B0c (mutable state store is first-class alongside the event log)**, and **B0d (DAG is a second layer over a first-party single-agent tool-loop)**. Any mention of gRPC, protobuf, or "all plugins out-of-process" in this file is superseded. The contract source of truth is `agezt-contract.jsonc`.


> Status: Active · Domain: github.com/agezt/agezt · License: MIT · Language: English
> Depends on: SPEC-06 (Warden), SPEC-08 (GHCR distribution), POLICY (packaging)
> Defines where and how Agezt runs: Docker as deployment substrate and as sandbox, GHCR images and CI/CD, multi-arch, and the spectrum from a $5 VPS to a cluster. Docker is an optional capability, never a hard requirement.

---

## 1. Deployment targets (the spectrum)

Agezt must run, with the same codebase, across:
- **Bare host** — a $5 VPS, an old Mac mini, a Raspberry Pi: static binaries, no Docker required (Warden uses `namespace` profile).
- **Docker host** — single-container or compose; `container` sandbox profile available.
- **Kubernetes** — kernel/gateway/plugins as workloads; for scale and isolation.
- **Serverless/ephemeral** — gateway + on-demand sandboxes (Daytona/Modal-class), hibernating between sessions.
- **Mesh** (future) — multiple nodes across the above (SPEC-02 §1.4).

The packaging policy (POLICY §2) makes this possible: static multi-arch binaries, optional single-binary build, one-artifact distribution.

---

## 2. Docker — two distinct roles

### 2.1 Role A: Agezt itself running in Docker (deployment substrate)
- **Image:** `FROM scratch` or distroless + the static binary set → tiny, fast, minimal attack surface (enabled by static Go builds).
- **Compose:** a reference `docker-compose.yml` brings up kernel + gateway + chosen plugins + optional Postgres/Redis. One command to a working system.
- **Volumes:** `~/.agezt` (journal, snapshots, config, secrets) is a persisted volume so state survives container recreation.

### 2.2 Role B: Docker as the sandbox (Warden `container` profile)
When Agezt runs tools/agents in containers, and Agezt itself is in a container, we must run containers from a container. Three options, with a documented default:
- **Sibling containers (recommended default):** Agezt talks to the host's container runtime to launch *sibling* sandbox containers (not nested). Cleanest isolation/perf; requires controlled access to the runtime socket.
- **Socket mount:** mount the runtime socket into Agezt — simplest, but grants broad host privilege; only for trusted single-tenant hosts, and the access is itself policy-noted.
- **DinD (Docker-in-Docker):** fully nested; heaviest; for strict isolation where sibling access isn't acceptable.
The chosen mode is config + journaled; the security implications of each are documented (SPEC-06 addition).

### 2.3 Sandbox image family
Pre-built isolated environments the `container` profile pulls:
- `agezt-sandbox-base` — minimal (shell/file/http tools).
- `agezt-sandbox-browser` — Playwright/Chromium for the browser tool.
- `agezt-sandbox-coding` — git + language toolchains for coding-agent delegation.
- `agezt-sandbox-data` / `-media` — data/audio/video tooling.
Each is digest-pinned and signature-verified before use (SPEC-06). Heavy dependencies live here (POLICY §1.2), keeping the core clean.

---

## 3. GHCR & CI/CD

### 3.1 Images published to GHCR
- **Core:** `ghcr.io/<org-TBD>/agezt:<semver>` + `:latest` + `:sha-<commit>`, multi-arch (amd64/arm64).
- **Gateway:** `ghcr.io/<org>/agezt-gateway:<semver>` (if deployed separately).
- **Sandboxes:** `ghcr.io/<org>/agezt-sandbox-<flavor>:<semver>`.
- **First-party plugins (optional OCI form):** `ghcr.io/<org>/agezt-plugin-<id>:<semver>`.

### 3.2 CI/CD pipeline (GitHub Actions)
- On tag/release: build static multi-arch binaries → build & push images → sign (cosign/sigstore) → generate SBOM → publish release artifacts + `CHANGELOG.md`.
- On PR: build, full test suite (unit + contract-conformance + replay/property + security + chaos, per IMPLEMENTATION §8), lint, dependency-justification check (POLICY §1.1).
- Reproducible builds where feasible; digests recorded so `agt update`/`agt plugin add` can pin them.

### 3.3 Supply chain (cross-ref SPEC-06)
- Images **digest-pinned**, **signature-verified** on pull.
- Unknown/unsigned images require explicit `--trust` and run in `container`/`microvm` with default-deny egress.
- SBOM published per image for auditability.

---

## 4. Runtime configuration & environments

- **Config precedence:** defaults < `config.yaml` < env (`AGEZT_*`) < flags (SPEC-02 §9).
- **Profiles:** named environment profiles (e.g. `local`, `vps`, `cluster`) preset isolation defaults, storage drivers, and resource limits.
- **Resource limits:** per-deployment CPU/mem caps; Pulse cadence adapts to host class (don't hot-loop a Pi).
- **Health/readiness endpoints** on the gateway for orchestrators (k8s liveness/readiness).
- **Graceful shutdown/restart:** drain in-flight work, persist, exit; the journal makes restart = recovery.

---

## 5. Observability for operators (OpenTelemetry)

- Optional **OpenTelemetry** export of traces/metrics/logs (the journal is the source; OTel is an *export*, not a replacement) for integration with existing monitoring stacks.
- Metrics: agent counts, task latency/throughput, token/cost, error rates, plugin health, journal lag.
- This is how Agezt fits into a real ops environment beyond its own Live Monitor.

---

## 6. Networking & exposure

- **Default:** nothing exposed publicly; control plane is a local socket; gateway binds locally.
- **Tunnels (SPEC-04 §7):** Cloudflare/Tailscale/WireRift expose the UI/gateway deliberately (escalate action). Public exposure never includes the raw control-plane socket.
- **mTLS** for remote gateway/mesh connections.

---

## 7. Phase placement

- Static multi-arch builds + scratch/distroless image + compose: **Phase 1** (so it's deployable early).
- GHCR CI/CD pipeline + signing + SBOM: **Phase 1–2** (foundational hygiene).
- Sandbox image family + container sandbox modes: **Phase 6** (with Warden container/microvm).
- k8s manifests/helm + OTel export + profiles: **Phase 7–8**.
- Serverless backends: **Phase 7+**.

---

## 8. Open questions

1. Default sandbox mode (sibling vs socket) trade-off documentation and secure default per deployment profile.
2. microVM backend choice and whether it ships as an optional, separately-built component (protect the lean core, POLICY §1).
3. Helm chart / operator vs plain manifests for k8s.
4. How much of OTel is built-in vs a plugin.

---

*Next: SPEC-12 (Widget System & SDK) and SPEC-13 (Capability Army), then SPEC-14 (Resilience, HITL, Eval, RBAC, Onboarding), then the cross-doc updates and a refreshed master index.*
