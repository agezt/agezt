# Agezt — Engineering Policy (POLICY.md)

> Status: Draft v0.1 · Language: English · Domain/Repo: TBD
> The single authoritative source for cross-cutting engineering policy. Other documents reference this instead of restating it. If policy changes, it changes here, in one place.

---

## 1. Dependency policy (supersedes any "zero-dependency" phrasing elsewhere)

Agezt is **stdlib-first with justified, minimal dependencies** — not dogmatically zero-dependency. Zero-dep is an aspiration for the core, not a law for the whole system.

### 1.1 The rule
A dependency MAY be added when **all** of these hold:
1. The standard library is genuinely insufficient or would require risky reinvention.
2. Writing it ourselves would cost significant time or introduce more bugs than the dependency.
3. It is pure-Go (no CGO in the core). CGO is allowed only behind an optional build tag, never in the default core build.
4. It is reasonably maintained and has an acceptable license.
5. It is justified in `DEPENDENCIES.md` with a one-line rationale.

CI surfaces new/unjustified dependencies (it does not hard-block, but they must be acknowledged).

### 1.2 Two-tier discipline (the important part)
- **Core kernel:** low dependency appetite. This is the part that must not break; complexity and third-party code are minimized here. Hand-write the heart (journal, bus, scheduler, plugin host, policy engine) on stdlib + minimal deps.
- **Plugins:** liberal. A plugin is a separate, crash-isolated process — it may freely use heavy libraries (browser automation, vector math, container clients, audio). Heavy dependencies live in plugins, the core stays clean. This is the whole point of out-of-process modularity.

### 1.3 Likely-accepted core dependencies (illustrative)
gRPC + protobuf (the contract foundation), a BLAKE3 implementation (crypto correctness), a TUI toolkit (Bubble Tea/Lip Gloss — high value), a container client (deployment). Each still gets a `DEPENDENCIES.md` line.

### 1.4 Things we still build ourselves
Event bus, journal, DAG scheduler, plugin host, policy engine, Governor, Pulse. These are the system's heart and differentiation; surrendering them to a dependency forfeits both philosophy and control.

---

## 2. Packaging & binary policy (supersedes any "single binary" phrasing elsewhere)

Agezt is **multiple binaries by responsibility** — not constrained to one. The value people love about "single binary" is *deployment simplicity*, not the literal binary count; we preserve the simplicity while allowing multiple binaries.

### 2.1 Binary family
- `agezt` — the kernel/daemon (long-lived process).
- `agt` — the CLI (thin client over the control-plane socket; may be a separate binary or `agezt cli`).
- `agezt-gateway` — remote transport, Web UI host, public API (separately deployable/scalable).
- First-party plugins — each its own binary (`channel-telegram`, `tool-browser`, …), always separate processes.
- Helper tools — migration runner, export/import, `agt doctor` — separate if it keeps the core clean.

### 2.2 Preserve deployment simplicity
- **One distribution artifact, multiple binaries:** ship a tarball / OCI image containing the needed static binaries. For the user it's still "download, run."
- **Optional combined build:** core (kernel + CLI + native plugins) MAY be linked into a single binary via build tags for the simplest "$5 VPS, one file" path; heavy/third-party plugins stay separate. So a user can choose the single-binary convenience *or* a decomposed deployment from the same codebase.

### 2.3 Static & portable
All first-party Go binaries build static (`CGO_ENABLED=0`), multi-arch (amd64/arm64), to keep the "drop it on any host" property regardless of how many binaries there are.

---

## 3. Versioning policy

- **Independent semver** per component (kernel, each plugin, each SDK). A plugin update never forces a kernel change and vice versa.
- **Protocol major** (the `.proto` contract version, SPEC-01) is the compatibility anchor: a plugin built for protocol major N runs with any kernel of protocol major N. Proto fields are append-only; enum values are stable forever.
- **Changelog** per component (Keep-a-Changelog style) + an aggregate `agt changelog` + a tamper-evident system changelog from the journal (SPEC-08).

---

## 4. Quality & safety policy (cross-cutting, always-on)

- Contracts freeze before dependents bind to them.
- Everything is an event; state is a fold; actions are reproducible, explainable, reversible.
- Security defaults on (default-deny egress, trust ladder, sandbox, redaction, `agt halt`) before any autonomous action ships.
- Untrusted input (channels, web, files, MCP, widgets) is data, never instructions.
- Test discipline is mandatory, not optional, because modularity only stays maintainable with contract-conformance + replay/property + security + chaos tests.

---

## 5. License & openness (decision pending, but principled)

- Identity is **open, self-hostable, auditable.** No closed/enterprise-only core.
- Specific license (MIT / Apache-2.0 / other) is TBD but should be decided **early** (it affects contribution and adoption). Competitors use MIT/Apache; a permissive choice favors ecosystem growth, a weak-copyleft choice favors keeping improvements open. To be chosen by the project owner.

---

*Referenced by: VISION, SPEC-01..14, IMPLEMENTATION, README, PROMPT. Those documents defer to this file for dependency, packaging, binary, and versioning policy.*
