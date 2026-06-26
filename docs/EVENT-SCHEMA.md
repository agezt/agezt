# AGEZT Event and Journal Schema Compatibility

Generated: 2026-06-21  
Status: compatibility policy for event consumers, demos, audit tooling, and future schema changes.

## Purpose

AGEZT's journal is the audit/replay/forensics truth. CLI commands such as `agt why`, Web UI folds, demos, operations runbooks, and external audit tooling all depend on event subjects, kinds, core fields, and selected payload fields staying understandable over time.

This document makes the compatibility rules explicit. It does not introduce a migration engine by itself; it defines the bar future event changes must meet.

## Source of truth

The implementation-level sources are:

- `kernel/event/event.go` — canonical `event.Event` struct and hash-chain rules.
- `kernel/event/kinds.go` — canonical event-kind constants.
- `kernel/journal/journal.go` — append-only JSONL journal, segment recovery, and hash-chain persistence.
- `cmd/agt/why.go` — operator-facing causality/audit reader.

Important existing invariants from code comments:

- Event kinds grow **append-only**. Do not rename existing kinds.
- `event.Event` field order is part of the wire/hash contract.
- New core event fields must be appended and should use `omitempty`.
- Corrections are new events, not in-place mutation of old events.
- The journal is durable-before-publish: meaningful events are appended before bus delivery.

## Compatibility levels

| Surface | Compatibility expectation |
|---|---|
| Core event fields (`id`, `seq`, `ts_unix_ms`, `prev_hash`, `hash`, `subject`, `actor`, `kind`, `correlation_id`, `causation_id`, `payload`, `tags`) | Stable. Do not remove, rename, reorder, or change semantics casually. |
| Event kind strings | Append-only. New kinds may be added; existing kind strings should not be renamed. |
| Subject strings | Prefer stable hierarchical subjects. Renames require migration notes or dual-write compatibility. |
| Payload fields | Additive by default. Consumers must tolerate unknown fields. Removing/renaming fields requires migration notes. |
| Tags | Additive/free-form. Consumers must tolerate missing and unknown tags. |
| Derived UI/control-plane folds | Internal unless documented as part of `/api/v1` or an SDK surface. |

## Change rules

### Allowed without migration

- Add a new event kind.
- Add a new optional payload field.
- Add a new tag.
- Add a new subject under an existing namespace when existing subjects continue to be emitted.
- Add a new optional core field only at the end of `event.Event` with `omitempty` and tests proving hash compatibility assumptions remain valid.

Current example: `context.compacted` may include optional
`skill_rescued_count` and `skill_rescued_chars` when marked skill/resource tool
outputs were preserved during compaction.

### Requires migration note or compatibility window

- Rename an event kind.
- Rename a subject that demos, UI folds, `agt why`, or SDK examples already consume.
- Rename or remove a payload field that current docs, demos, UI, CLI, or SDK tests reference.
- Change payload type semantics, for example `cost_microcents` number to string.
- Stop emitting an event kind that existing audit workflows rely on.

### Avoid unless versioned explicitly

- Reorder fields in `event.Event`.
- Change canonical JSON hashing behavior.
- Change `prev_hash` / `hash` semantics.
- Change sequence ordering or segment recovery semantics.
- Treat old journal segments as invalid only because a new daemon version added event kinds.

## Consumer rules

Consumers should be liberal readers:

1. Match on `kind` first, then inspect `payload` defensively.
2. Tolerate unknown event kinds.
3. Tolerate unknown payload fields.
4. Treat missing optional fields as unknown, not as false unless documented.
5. Prefer `correlation_id` and `causation_id` for linkage over parsing subject strings.
6. Do not assume payload field order.
7. Do not assume every event has payload.
8. Use `agt why <event_id> --json` for full causality when debugging.

## Producer rules

Producers should be conservative writers:

1. Reuse existing event kinds when semantics match.
2. Add new event kinds when behavior is materially new.
3. Keep payloads JSON object-shaped where practical.
4. Include identifiers needed for joins: `schedule_id`, `agent`, `tool`, `run`, `correlation_id`, or domain-specific IDs.
5. Include outcome fields for lifecycle-like events: `status`, `reason`, `error`, or `answer` where relevant.
6. For autonomous behavior, include enough data for `journal -> status -> UI/CLI` folds without requiring log scraping.
7. When changing a consumed payload field, dual-write old and new fields for at least one compatibility window or document why that is impossible.

## Versioning approach

Current state: event schema compatibility is policy-based, not a global numeric schema version.

Recommended near-term approach:

- Keep event kinds and core fields append-only.
- Add changelog entries for new or changed event subjects/kinds that affect consumers.
- Add docs/tests when a payload field becomes part of a demo, SDK, or operator workflow.
- Introduce a global event schema version only if a breaking migration becomes necessary.

A future global version could be represented as a constant in `kernel/event` and surfaced in diagnostics, but this plan deliberately avoids adding code until there is a concrete migration need.

## Testing expectations

When event schema behavior changes, choose tests based on impact:

- Core event fields/hash behavior: `go test ./kernel/event ./kernel/journal`.
- Causality/forensics output: `go test ./cmd/agt -run Why` plus relevant control-plane tests.
- Runtime events: package-specific tests for the emitting subsystem.
- UI/control-plane folds: tests for the fold path that consumes the event.
- SDK/API visible events: SDK behavioral tests and `docs/SDK-PARITY.md` update when applicable.

## Documentation checklist for event changes

Before merging a meaningful event change:

- [ ] Is this a new kind, new subject, or payload change?
- [ ] Is it additive?
- [ ] Could existing `agt why`, UI, demo, or SDK consumers depend on the old shape?
- [ ] Does `docs/API-STABILITY.md` need an update?
- [ ] Does `CHANGELOG.md` need a note?
- [ ] Do examples/demos need expected-output updates?
- [ ] Are tests updated for both emitters and consumers?

## Current known gap closed by this document

`docs/API-STABILITY.md` previously described event/journal schema versioning as implicit. This document makes compatibility expectations explicit while keeping the implementation policy-based and append-only.
