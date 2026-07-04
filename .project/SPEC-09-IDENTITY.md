# Agezt — Identity, Export/Import & Backup Specification (SPEC-09)
> ⚠️ **AUTHORITY NOTICE:** This document is subordinate to `DECISIONS.md`. Where anything here conflicts with DECISIONS, **DECISIONS wins** — especially the foundational revisions **B0 (transport = stdio + JSON-RPC 2.0, NOT gRPC/protobuf)**, **B0a (plugins default in-process; out-of-process only for isolation)**, **B0b (minimal contract, grows append-only)**, **B0c (mutable state store is first-class alongside the event log)**, and **B0d (DAG is a second layer over a first-party single-agent tool-loop)**. Any mention of gRPC, protobuf, or "all plugins out-of-process" in this file is superseded. The contract source of truth is `agezt-contract.jsonc`.


> Status: Active · Domain: github.com/agezt/agezt · License: MIT
> Depends on: SPEC-01, SPEC-02, SPEC-08
> Defines the identity scheme (ULID/UUID everywhere), the export/import bundle format, granular extraction, point-in-time restore, and how this underpins backup, migration, and the future mesh.

---

## 1. Identity scheme — IDs everywhere

### 1.1 Two ID kinds
- **ULID** for everything time-ordered and entity-like: events, agents, tasks, sessions, plugins-instances, skills (instance refs), memory records, channel sessions, standing orders, migrations, jobs. ULIDs are globally unique **and** lexicographically sortable (they embed a timestamp) — so the journal, ordering, and IDs speak the same language.
- **Content-address (BLAKE3)** for immutable content: skill bodies, artifacts, snapshots, plugin/image digests. Identical content → identical id (natural dedup, integrity, cache).

### 1.2 Why IDs everywhere matter
- **Export/import** needs stable, collision-free identity to move things between instances.
- **Mesh** (future) needs globally-unique IDs so two nodes never clash (no central allocator).
- **References & provenance** (`causation_id`, `correlation_id`, `source_event`) are ID-based; the whole audit graph depends on it.
- **Idempotency:** operations keyed by ID are safe to retry.

### 1.3 Rule
Anything that is created, referenced, exported, or migrated carries a ULID at creation (assigned by the kernel, never by plugins — same discipline as event ids). Immutable content additionally carries its content-address.

---

## 2. The export bundle

The whole system is reconstructable from a small set of components, so export is well-defined and complete:

```
bundle/
  manifest.json          # bundle version, source instance id, created_at, contents, integrity
  journal/               # the event log (full or a scoped slice)
  snapshots/             # projection snapshots (optional; speeds import)
  secrets.enc            # encrypted secrets (only if explicitly included; separate passphrase)
  plugins.json           # installed plugin manifests + versions + digests (not the binaries by default)
  config.yaml            # config (redacted unless secrets included)
  world-model/           # the context graph (or its journal events)
  signature             # bundle signature over the content-address of everything above
```

- **Content-addressed & signed:** every file is hashed; the bundle is signed; import verifies integrity end-to-end (tamper-evident, same chain-of-custody as the journal).
- **Versioned:** `manifest.json` records the bundle format + protocol major; import refuses incompatible majors or runs a documented translation.
- **Secrets are opt-in and separately protected:** by default a bundle is portable *without* credentials (you re-auth on the target); including them requires an explicit flag and a separate passphrase.

---

## 3. Granular export (because everything is ID'd and causally linked)

Event-sourcing + IDs make surgical extraction natural — you can "cut" any subgraph by following `correlation_id`/`causation_id`:

| Scope | What it includes |
|---|---|
| **Full system** | entire journal + snapshots + plugins + config + world model |
| **Tenant** | one tenant's events/memory/world-model slice (multi-tenant) |
| **Agent** | one agent's correlation-scoped event slice + its state + relevant memory |
| **Task/workflow** | a single task's DAG + its events (reproducible bundle of one run) |
| **Skill** | a skill's versioned body + lineage + metrics |
| **Standing order** | its config (observers + overrides + initiative scope) |
| **Memory subset** | a topic/entity neighborhood of the world model |

`agt export --scope agent:<ulid>` etc. Each scope produces a self-contained, signed bundle.

---

## 4. Import

`agt import <bundle>`:
1. Verify signature + content-addresses (refuse on tamper).
2. Check protocol/bundle version compatibility.
3. Install/verify required plugins (by digest) or report what's missing.
4. Run plugin + core migrations to the bundle's schema state.
5. Replay/merge the journal slice; rebuild projections.
6. Re-key nothing — ULIDs are globally unique, so imported IDs never collide with the target's.
7. Resume: imported agents/standing orders can pick up where they left off (state folded from the imported events).
8. Emit `EVT_IMPORTED`.

Merge semantics: importing into a fresh instance = restore; importing into a live instance = additive merge (IDs are unique, so no overwrite). Conflicting *config/policy* is surfaced for the operator to resolve, not silently merged.

---

## 5. Backup & point-in-time restore

- **Backup = export.** Scheduled (Chronos) full or incremental bundles to a destination (local, object store, via a Tunnel to remote). Incremental = journal segments since the last backup + a fresh snapshot.
- **Restore = import.** Into a fresh instance or to recover a damaged one.
- **Point-in-time restore (free from event-sourcing):** "return to the state as of last Tuesday 14:00" = replay the journal up to that sequence/timestamp. No special backup format needed; the journal *is* the time machine. `agt restore --at <timestamp|seq>` produces a projection at that point (non-destructively — it appends, never rewrites; you can branch a recovered state).
- **Integrity:** `agt journal verify` before/after restore proves the chain is intact.

---

## 6. Relationship to migration & mesh

- **`agt migrate openclaw|hermes`** (SPEC-02/TASKS P9) uses the **same import pipeline**: an adapter converts their settings/memories/skills into a Agezt bundle, which then imports normally. One import path serves restore, cross-instance move, and competitor migration.
- **Mesh agent migration** (future) ships an agent's correlation-scoped event slice (a granular export) to another node, which imports and resumes it. ID uniqueness is what makes this safe without a coordinator.
- **Multi-tenant** export/import operates at tenant scope with isolation preserved.

---

## 7. New event kinds (added to SPEC-01 §7)

`EVT_EXPORTED`, `EVT_IMPORTED`, `EVT_BACKUP_CREATED`, `EVT_RESTORED`, `EVT_RESTORE_POINT_CREATED`. All carry the scope + bundle content-address for audit.

---

## 8. CLI surface

```
agt export [--scope full|tenant:<id>|agent:<id>|task:<id>|skill:<id>|standing:<id>|memory:<query>] [--with-secrets] [--out file]
agt import <bundle> [--merge|--restore]
agt backup [--full|--incremental] [--to <dest>]
agt restore <bundle> | --at <timestamp|seq>
agt journal verify
agt migrate openclaw|hermes <source>
```

---

## 9. Phase placement

- ULID/content-address discipline: **Phase 0** (must be right from the start; retrofitting IDs is painful).
- Export/import (full + agent scope) + backup: **Phase 2–3**.
- Point-in-time restore: **Phase 3** (rides on journal replay already built).
- Granular scopes (skill/standing/memory): **Phase 5–6**.
- Competitor migration: **Phase 9**.

---

## 10. Open questions

1. Incremental backup granularity (segment-level vs event-level) and dedup across backups.
2. Secret portability: re-auth-on-target (safer) as the strong default vs encrypted-secret transport for true clones.
3. Live-merge conflict policy for config/policy collisions.
4. Bundle size management for very long journals (compaction vs full history in export).

---

*Next: SPEC-10 (LLM, Context & Routing).*
