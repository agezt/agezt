# AGEZT Graveyard Retention Policy

Generated: 2026-06-22  
Status: decision record. No destructive automation is implemented or enabled by this document.

## Purpose

AGEZT supports retiring agents, exporting tombstones, inspecting the graveyard, and running a notify-only retention scan. Automatic deletion of retired agents (or any cascade that removes durable resources on a timer) is intentionally not implemented. This document records the current posture and the design bar any future destructive automation must meet before it ships.

## Current behavior

What exists today:

- Retire/revive/remove are manual operator actions, journaled and auditable through `agt why`.
- `agt agent tombstone <slug> [--json]` exports a read-only death certificate: identity, lifecycle, retirement record, and durable resource footprint.
- `agt agent graveyard [--older-than DAYS] [--json]` lists retired agents oldest-first as a retention-eligibility view.
- The `graveyard_scan` system task can be scheduled and journals `schedule.system_task.graveyard_scan` with counts and eligible slugs. It removes nothing; `AGEZT_GRAVEYARD_RETENTION_DAYS` defaults to `0` (keep forever).
- Removal impact planning and tombstone export are available as safe, non-destructive building blocks.
- Backups (`agt backup`) and restore drills are operator-driven; see `docs/OPERATIONS.md`.

What does NOT exist today:

- No automatic removal of retired agents.
- No automatic cascade that deletes memory, skills, config, workspaces, schedules, standing orders, or sub-agent trees without an explicit operator action.
- No non-destructive "archive" state distinct from "retired".
- No global retention timer that triggers irreversible `RemoveProfile`.

## Default posture

- Retired agents are kept by default.
- The retention scan is advisory only (`action: report_only`).
- Deletion requires an explicit operator action through the existing retire/remove path, scoped to the resources the operator confirms.

## Owner decision required before destructive automation

Turning the eligibility report into automatic removal must not be built unprompted. It requires explicit owner approval and a written policy decision on at least the following:

1. Default retention threshold (`AGEZT_GRAVEYARD_RETENTION_DAYS`) and whether `0` (keep forever) stays the safe default.
2. Scope of automatic removal: identity only, or full cascade including memory, skills, config, workspace, schedules, standing orders, and sub-agent trees.
3. Whether a non-destructive "archive" state is introduced before any deletion, or whether removal stays irreversible.
4. Operator opt-in model: disabled by default, enabled by explicit config, or enabled per tenant.
5. Tenant isolation implications: per-tenant retention windows and tokens.
6. Notification requirements before deletion: mailbox, channel, or operator alert.
7. Legal/compliance retention needs that may override automatic deletion.

## Design bar if destructive automation is approved

If the owner approves automatic removal in the future, the implementation must include all of the following before it is enabled by default:

- Dry-run mode that reports what would be removed without removing it.
- Approval gate that requires explicit operator confirmation before the first real deletion.
- Retention threshold with a documented, safe default and an env override.
- Pre-deletion tombstone export written to a stable location.
- Audit event(s) in the journal capturing exactly what was removed, when, why, and by whom (operator or scheduled task).
- Restore/rollback story: at minimum, a documented restore-from-backup procedure; ideally a best-effort restore path from the tombstone plus a recent backup.
- Tenant-safe behavior: no cross-tenant deletion; per-tenant eligibility windows respected.
- Fail-safe defaults: any misconfiguration or missing approval must default to keep, not delete.
- Tests proving a retired agent survives the scan when auto-removal is disabled, and that an approved removal produces the expected audit/tombstone artifacts.

## Recommended next steps

1. Keep the current report-only scan and tombstone/graveyard inspection as the supported surface.
2. Use `docs/OPERATIONS.md` backup/restore guidance as the restore path of record until a dedicated archive state exists.
3. If product needs automatic removal, open an owner decision issue referencing this document and answer the questions in the "Owner decision required" section before any code work begins.
4. Do not add destructive scheduled behavior as a side effect of other features.

## Related surfaces

- Manual retire/remove: `kernel/controlplane/roster.go`, `cmd/agt/agent.go`.
- Notify-only scan: `cmd/agezt` `runScheduledGraveyardScan`, `cadence.SystemTaskGraveyardScan`.
- Tombstone/graveyard views: `agt agent tombstone`, `agt agent graveyard`.
- Backup/restore: `docs/OPERATIONS.md`, `cmd/agt/backup.go`.
- Removal guardrails: `NEXT.md` graveyard section.
