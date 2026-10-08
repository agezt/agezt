# W2.30e agent activity summary move

The per-event activity text — one human line for each journal event attributable to
an agent — moved unchanged from the native control plane into `kernel/app/roster`.
This is a move-before-rewrite foundation for binding `agent_activity` as a typed
operation; no command, wire shape or registration changes in this slice.

## What moved

- `agentActivitySummary` (`agent_activity_summary.go`, 438 lines) becomes
  `approster.ActivitySummary`; the native file is deleted.
- Its sole-consumer helpers move with it: `agentRetryPolicySummary`,
  `pausedTriggerSummary`, `removalCleanupSummary` (the whole
  `roster_activity_text_misc.go`, deleted), plus `joinActivityParts` and
  `wakeRunbookActivitySuffix` from `roster_escalation_rows.go`.
- `isMailboxWakeSubject` becomes `approster.IsMailboxWakeSubject`, now shared by the
  summary and the native status collector.
- `activity_payload.go` holds application copies of the six payload accessors the
  text needs (`plString`, `plInt`, `plStrings`, `truncate`, `firstNonEmpty`,
  `intNumber`), so the application owns its decoding without importing the control
  plane. The native originals stay for their other native callers.

The native activity handler and the single-pass status journal collector now call
the application functions. Bodies were extracted from full-path snapshots with only
the two renames applied.

## Evidence

A temporary harness restores the pre-move native functions under their original
names next to the moved ones and compares `(text, ok)` for 2,000,000 seeded
generated events: 13 event kinds, the code's own subject literals, 52 payload keys,
values drawn from its 243 string literals and its `case`/`==` selector literals,
long and HTML-like strings, counts, booleans, string lists, runbook and trigger
payload maps, slug-matching and foreign agents, and in-run versus foreign
correlations. 558,599 events are attributable; every result is identical, as is
`IsMailboxWakeSubject` for every subject. Under this corpus the moved
`ActivitySummary` reaches 97.7% of its statements and every moved helper 100%.

A permanent golden test pins one event per summary branch family (38 cases,
including non-attributable events) and the mailbox-subject rule. Twenty-one
independent mutations across branch scopes, labels, joins, thresholds, fallbacks
and the payload accessors each fail the permanent tests; sources are restored
byte-for-byte. Source, race and full repository gates pass on unchanged inputs; the
first gate run stopped only at the whole-tree gofmt check (line endings in the new
golden test), which was reformatted and the format, diff, vet and package tests rerun.
Fixtures do not touch the owner's home, wake agents, run a provider or send
channel messages.
