# W2.30d typed agent_graveyard

`agent_graveyard` is now a shared application operation. `approster.GraveyardService`
owns the retired-agent report over the native roster list and an injectable clock;
`GraveyardOperations` declares one primary-only, read-only spec with unknown input
allowed and no HTTP metadata (the command has no Web UI route; `agt agent graveyard`
uses the socket). The native `handleAgentGraveyard` handler, its manual
`commandSpec` row and the now-unused `plInt64` helper are removed. The native wire
entry is `AppOwned`, `ReadOnly`, not tenant-allowed or routed. Tombstone and impact
stay native because they share the removal-cascade subsystems with `agent_remove`.

## Preserved behavior

`older_than_days` keeps the legacy leniency: a JSON number is used as-is, a string
is trimmed and parsed with its parse error ignored but its value kept (so `"1e400"`
is +Inf), and any other value, null or absence means no filter. A positive window
excludes agents retired after `now - days`; the boundary instant is kept and agents
with no retirement time are always listed. `age_days` and the reported
`older_than_days` truncate toward zero, rows keep roster order within equal
retirement times (stable oldest-first sort), and an empty report is `[]`, not null.
`GraveyardOutput`/`GraveyardRow` type every field (`age_days`, `kind`, `name`,
`retired_ms`, `retired_reason`, `slug`, `system`, plus `count`). The intended wire
change is shared already-canceled admission, which now rejects before reading the
roster.

## Runnable comparison and regression evidence

The native harness runs the pre-slice handler (reconstructed from full-path
snapshots, with its `plInt64` helper) against the registered operation on the same
initialized kernels. 468 cases repeated twenty times cover three kernel states
(empty; live, retired custom and retired managed agents with HTML-escaped names;
plus a third retired agent with an empty reason), normal and canceled contexts,
primary, wrong and tenant tokens, thirteen argument shapes (numbers, numeric and
padded strings, garbage strings, negatives, booleans, null, arrays, overflow strings,
1e300, unknown keys) and two reads. 390 complete raw socket responses are
byte-equal; the 78 differences are exactly the primary-token canceled admissions.
Journal head/hash and provider call counts stay unchanged.

Permanent tests pin the codec table, sub-day/boundary/fractional/negative windows,
unknown retirement age, per-field rows including system and managed kinds, the empty
array shape, stable order across 64 interleaved ties, spec metadata and output
schema typing, non-primary and canceled admission, and the native wire entry.
Twenty-two independent mutations across the codec, filter, ages, rows, ordering,
empty shape, spec and native registration each fail a test; sources are restored
byte-for-byte. The first whole-package race run failed only on the raw-argument-cast
ratchet: `roster_tombstone.go` fell from one raw `req.Args[...].(T)` cast to zero,
so its baseline was lowered to 0 as the ratchet requires, and the full gate run was
repeated from the start on the changed inputs. The official structure generator then
showed the new files' license header attached to `package roster` (Go read it as the
package doc); a blank line now detaches it in the new graveyard files and the two
W2.30c list files, the generated structure is unchanged, and the format/build/vet/
roster/native-registry/ratchet/structure/deadcode subset was rerun. Fixtures use isolated temporary
kernels and do not mutate owner profiles, wake agents, run a provider or send
channel messages.
