# M133 — `agt changelog`: the system timeline (SPEC-08 §4.2)

## Why
This is a **specced** feature (SPEC-08 §4.2 "System changelog"), not invented
polish. The spec calls for a filtered projection of the journal as a
human-readable, **tamper-evident** timeline of what actually changed about *this*
system — skill promoted/quarantined/reverted, policy updates, core/plugin updates,
export/import/restore — rendered by `agt changelog --system`, with `agt why`
working on any entry. It was unbuilt.

The value over what exists: `journal tail` shows *every* raw event (llm.request,
tool.invoked, routing.decision — overwhelmingly routine noise). An operator asking
"what materially changed about my system, and when?" had no curated answer.
Because it rides on the hash-chained journal, the answer is *provable* — a static
CHANGELOG.md can't say "this change really happened, when, and what caused it".

## What
- **`CmdChangelog` + `agt changelog [N] [--since <dur>] [--json]`** — folds the
  journal to a curated allowlist of material-change event kinds, each mapped to a
  stable human label, newest-first, carrying its **full event id** so
  `agt why <id>` proves and explains it. `--system` is accepted as a spec-
  compatible alias (the default view *is* the system timeline).
- **Curated kinds** (only those that actually exist today): halt, resume,
  policy.changed, skill.created/promoted/quarantined/reverted, reflection.completed,
  catalog.synced / sync_failed / discovery_completed / discovery_failed,
  pulse.paused/resumed. Plugin/migration/core-update entries from the spec's list
  are added when those features land and emit events — membership is a single map,
  so extending it is one line.
- A light `changelogDetail` probes a few common payload keys (summary/name/skill_id/
  change/…) so entries read meaningfully without brittle per-kind decoding.
- Read-only; tenant-routed via `kernelFor(tenantOf(req))` for a future `--tenant`.

## Files
- `kernel/controlplane/protocol.go` — `CmdChangelog`; `server.go` dispatch.
- `kernel/controlplane/changelog.go` (new) — `handleChangelog`, `changelogKinds`,
  `changelogDetail`.
- `cmd/agt/changelog.go` (new) — `cmdChangelog`; dispatch in `main.go`.
- `kernel/controlplane/changelog_test.go` (new) — material kinds folded, routine
  `task.received` filtered out, newest-first, correct label/detail, full event id.

## Live proof (offline mock)
After a routine run + a halt + a resume:
```
$ agt changelog
  2026-06-02 10:40:14  system resumed              (01KT3MD98P0FS764N004GJA1YF)
  2026-06-02 10:40:14  system HALTED               (01KT3MD95XSCCMX237AD3SEV5R)

(journal held 17 raw events; changelog curated the material 2 — the run's
 llm/tool/routing events are correctly excluded.)
```
Each entry's full event id feeds `agt why <id>`. Policy/skill folding is covered
by the unit test (publishes policy.changed + skill.promoted, asserts labels,
detail, ordering, and that task.received never appears).

## Verification
- 55 packages `ok`, **FAIL 0**; **1426 tests**.
- `go vet` clean, `GOOS=linux go build ./...` ok, `go.mod` / `go.sum` unchanged.
- gofmt: all new files clean under LF.
