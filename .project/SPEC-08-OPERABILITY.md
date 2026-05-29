# Agezt — Operability: Updates, Migrations, Contributions & Changelog (SPEC-08)
> ⚠️ **AUTHORITY NOTICE:** This document is subordinate to `DECISIONS.md`. Where anything here conflicts with DECISIONS, **DECISIONS wins** — especially the foundational revisions **B0 (transport = stdio + JSON-RPC 2.0, NOT gRPC/protobuf)**, **B0a (plugins default in-process; out-of-process only for isolation)**, **B0b (minimal contract, grows append-only)**, **B0c (mutable state store is first-class alongside the event log)**, and **B0d (DAG is a second layer over a first-party single-agent tool-loop)**. Any mention of gRPC, protobuf, or "all plugins out-of-process" in this file is superseded. The contract source of truth is `agezt-contract.jsonc`.


> Status: Draft v0.1 · Language: English · Domain/Repo: TBD
> Depends on: SPEC-01, SPEC-02, SPEC-04, POLICY
> Defines how the system updates itself, how plugins migrate their own schemas and contribute endpoints/CLI/UI, and how change is tracked. This is the "living system" layer that makes Agezt maintainable in production — the layer OpenClaw/Hermes treat lightly.

---

## 1. Plugin contribution model (plugins extend the system, not just respond)

A plugin is not a passive callee. On registration it declares **contributions** — capabilities it adds to the running system. The kernel mounts them on install/enable and unmounts cleanly on disable/remove.

```yaml
# plugin.yaml (extended)
id: github
binary: ./github-plugin        # or image: ghcr.io/<author>/agezt-plugin-github:1.2.0
version: 1.2.0
protocol_version: 1
capabilities: [ ... ]
contributes:
  migrations: ./migrations/      # versioned schema migrations this plugin owns
  api_routes:                    # endpoints mounted under a namespace
    - path: /v1/plugins/github/webhook
      methods: [POST]
      auth: token
  cli_commands:                  # subcommands added to `agt`
    - name: github
      subcommands: [pr, issue, repo]
  ui_widgets: ./widgets/         # render contributions (SPEC-12)
  event_subjects: [github.>]     # subjects it emits/owns
  cron_jobs: ./cron.yaml         # scheduled jobs it wants registered
  world_entities: [repo, pr, issue]  # world-model entity types it introduces
isolation: container
```

Mounting rules:
- **Namespaced:** all contributions live under the plugin's id (routes under `/v1/plugins/github/…`, CLI under `agt github …`) to prevent collisions.
- **Policy-gated:** mounted routes/commands still pass through Edict + auth. A plugin cannot bypass governance by adding an endpoint.
- **Reversible:** disabling a plugin unmounts every contribution; the system returns to its prior surface exactly. (Tracked as events, so it's auditable.)
- **Discovered dynamically:** the CLI/gateway query active plugins at startup and register their contributions; nothing is hardcoded.

This is what "whatever it needs — its own DB, its own endpoint, its own CLI args" means: a plugin ships code + schema + endpoints + commands + UI as one self-contained extension.

---

## 2. Migration system

### 2.1 Ownership
The kernel owns **core migrations** (journal/state/projection schema). Each plugin owns **its own migrations** (its DB tables/collections, whatever backend it uses). The kernel provides a **migration runner**; plugins provide migration sets.

### 2.2 Migration format
```
migrations/
  0001_init.up.sql     0001_init.down.sql
  0002_add_index.up.sql 0002_add_index.down.sql
```
(SQL shown; a plugin using a non-SQL store provides equivalent up/down steps via the runner interface.) Each migration has: a sequence number, a checksum, and a UUID. Applied migrations are recorded — which plugin, which version, which migration, when, checksum — as journal events.

### 2.3 Runner guarantees
- **Ordered & idempotent:** runs pending migrations in sequence; re-running is a no-op.
- **Versioned:** on plugin update, only new migrations run; on downgrade, `down` migrations roll back.
- **Checksummed:** a changed already-applied migration is detected and refused (drift protection).
- **Transactional where the backend allows;** otherwise the runner records partial progress so a failed migration can be diagnosed and resumed/rolled back.
- **Journaled:** `EVT_MIGRATION_APPLIED` / `EVT_MIGRATION_REVERTED` — the migration history is part of the tamper-evident audit log.

### 2.4 Core migrations
Schema/projection changes in the kernel use the same runner. A core update (§3) triggers core migrations + a projection rebuild if the projection shape changed (the journal itself is append-only and never migrated destructively — only projections are rebuilt).

---

## 3. Update mechanism

Two independent update tracks (per POLICY §3 independent versioning):

### 3.1 Core update
`agt update` (or image pull): fetch new `agezt`/`agt`/`agezt-gateway` binaries (or image), verify signature + digest, stop accepting new work (drain), apply core migrations, rebuild projections if needed, restart, emit `EVT_CORE_UPDATED`. Gateway can auto-restart with the daemon. Rollback: keep the previous binary + the journal is intact, so reverting the binary and replaying is safe.

### 3.2 Plugin update
`agt plugin update <id>` (or marketplace/GHCR pull): fetch new plugin binary/image, verify signature + digest, run the plugin's new migrations, hot-swap the plugin process (the kernel keeps running — crash isolation makes this safe), emit `EVT_PLUGIN_UPDATED`. A plugin update never touches the core. This is the operational backbone of the "won't blow up / easy to maintain" promise: you update one capability without risking the heart.

### 3.3 Distribution channels
- Binaries/images via GHCR (§6) and/or direct release artifacts.
- Plugins via local path, URL, or marketplace ref (signed, content-addressed).
- `agt doctor` checks version skew across kernel/plugins/SDK and flags incompatibilities.

---

## 4. Changelog & version tracking

Two complementary changelogs (per POLICY §3):

### 4.1 Release changelog (human, per component)
Each component (kernel, each plugin, each SDK) keeps a `CHANGELOG.md` (Keep-a-Changelog format, semver). `agt changelog` **aggregates** the installed components into one view: "here's everything installed, its version, and what changed in the last update." Solves the distributed-versioning visibility problem.

### 4.2 System changelog (machine, from the journal)
A filtered projection of the journal as a human-readable, **tamper-evident** timeline of what actually happened to *this* system:
- plugin installed/updated/removed (version X→Y, which migrations ran)
- migration applied/reverted
- skill promoted/quarantined/reverted (Forge)
- trust-ladder change, policy update
- core update, projection rebuild
- export/import/restore

`agt changelog --system` renders it; the Web UI shows it as a **System Timeline** (SPEC-07 addition). Because it's the hash-chained journal, "did this change really happen, when, and what caused it" is provable — something a static CHANGELOG.md can never offer. `agt why` works on any timeline entry.

### 4.3 Machine-readable use
The changelog is also a system signal: the reflection loop reads it ("error rate rose after the last update"), rollback decisions read "what changed last," and mesh nodes query each other's versions. Changelog is both documentation and telemetry.

### 4.4 New event kinds (added to SPEC-01 §7)
`EVT_PLUGIN_INSTALLED`, `EVT_PLUGIN_UPDATED`, `EVT_PLUGIN_REMOVED`, `EVT_PLUGIN_ENABLED`, `EVT_PLUGIN_DISABLED`, `EVT_MIGRATION_APPLIED`, `EVT_MIGRATION_REVERTED`, `EVT_CORE_UPDATED`, `EVT_CONTRIBUTION_MOUNTED`, `EVT_CONTRIBUTION_UNMOUNTED`. (Export/import/restore kinds defined in SPEC-09.)

---

## 5. Dynamic surface management

Because contributions mount/unmount at runtime:
- **API router** rebuilds its route table on plugin enable/disable; namespaced; policy-wrapped.
- **CLI** resolves plugin subcommands at invocation (a registry the daemon exposes over the socket); `agt help` reflects currently-active plugins.
- **UI** loads widget/panel contributions dynamically (SPEC-12); a missing plugin simply means its UI isn't present.
- **Cron** registers/unregisters plugin jobs through Chronos; jobs survive restarts (reloaded from the journal) and disappear on plugin removal.

---

## 6. GHCR as a distribution channel

GHCR plays three roles (full deployment detail in SPEC-11):
- **Core image:** `ghcr.io/<org-TBD>/agezt:<version>` — multi-arch, built in CI per release.
- **Sandbox base images:** `ghcr.io/<org>/agezt-sandbox-<flavor>` for the Warden container profile (SPEC-06).
- **Plugin images (key):** a plugin may ship as an OCI image. `agt plugin add ghcr.io/<author>/agezt-plugin-x:1.2.0` pulls it, verifies **digest + signature** (SPEC-06 supply chain), and runs it as a sandboxed container. Plugin distribution = OCI image distribution; versioning = image tag + digest (content-addressed, matching our philosophy). This makes the marketplace (§7) ride on standard registry infrastructure.

Images are **digest-pinned and signature-verified**; unknown images require explicit trust and run in `container`/`microvm` with default-deny egress.

---

## 7. Marketplace (rides on the above)

Plugins, skills, standing-order templates, and Flow Studio workflows are distributed as signed, content-addressed, versioned artifacts (via GHCR for OCI plugins, a registry index for the rest). `agt plugin add <ref>` resolves local path / URL / marketplace / GHCR ref uniformly. Importing an artifact that includes autonomous actions surfaces its required trust levels for approval before activation (SPEC-06).

---

## 8. Phase placement (updates TASKS / IMPLEMENTATION)

- Migration runner + plugin-contribution mounting: **Phase 1–2** (needed as soon as plugins have state/surfaces).
- Changelog (release + system) + version tracking: **Phase 2–3** (system changelog rides on existing events).
- Core/plugin update mechanism: **Phase 6–7** (after the plugin model is mature).
- GHCR distribution + marketplace: **Phase 7–8**.

---

## 9. Open questions

1. Migration backend abstraction: SQL-centric runner vs a generic up/down step interface for non-SQL plugin stores.
2. Hot-swap semantics for a plugin mid-task: drain in-flight invocations vs checkpoint-and-resume.
3. Core update with breaking projection changes: always rebuild vs incremental projection migration.
4. Marketplace index hosting (TBD infra) and signing key management.

---

*Next: SPEC-09 (Identity, Export/Import & Backup).*
