# W2.39b typed disk statistics

`disk_stats` now runs on the shared dispatcher in `kernel/app/storage`, beside
`storage_stats`. It reports the append-only journal's on-disk size and the free
space on the filesystem holding the daemon's home. `agt disk` and the doctor's
disk check read it.

`Service.Disk` uses the same ports as the storage inventory:

- the filesystem's `Usage` walk, which reads the journal directory under the
  home;
- the injected `DiskFreeFunc` probe on the home.

The free-space projection is now one `freeSpace` helper, shared by both
operations. The operation is primary-only and primary-tenancy, read-only and
unaudited, with unknown input allowed and no route. These are the defaults
`storage_stats` already declares, and they match the native registration.

Removed with the move:

- `disk.go`, which held `handleDiskStats` and its own `dirSize` walk;
- its registration.

The journal size reported by `journal_stats` now comes from the same `dirUsage`
walk. That walk sums the same regular-file sizes and skips the same errors.

## Preserved behavior

- The response is
  `{"base_dir":…,"journal_bytes":…,"disk_available":…}`, plus
  `disk_free_bytes`, `disk_total_bytes` and `disk_free_pct` only when the
  probe succeeds on a non-zero filesystem.
- The percentage is `free / total * 100`. A probe that reports zero free still
  shows its zero fields, and byte counts keep full 64-bit precision.
- With no probe, a probe error, or a zero-sized filesystem, the disk fields are
  absent and `disk_available` is `false`. A missing or unreadable journal counts
  as empty. Disk stats never fail.
- Any `tenant` argument is ignored, and tenant tokens are refused. The home is
  the primary kernel's.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before any read.

## Runnable comparison and regression evidence

The harness clones a fixture kernel with a populated journal. It runs the
pre-slice handler on one copy and the registered operation on another, under
six probes:

- none;
- an error;
- a zero total;
- 1 of 3 bytes;
- zero free of 2^53+1;
- 25 of 100 GiB.

144 steps repeated twenty times send an empty request, a named tenant,
unknown and secret-named arguments, and no arguments under primary, wrong and
tenant tokens, in normal and canceled contexts.

Responses are byte-exact. The only masked value is the home path, which differs
per clone; the harness checks first that each side reports its own home.
Neither journal changes. The harness asserts each probe's fields and a non-zero
journal size. The exceptions are 24 canceled primary-token steps. These return
the admission error.

Permanent tests cover:

- the journal directory's usage;
- the probe path;
- every probe mode with exact wire bytes, including present zero fields and a
  2^53+1 total;
- a missing journal;
- both storage specs and the disk output schema.

The native storage and artifact spec test now pins the six storage and
artifact operations, with `disk_stats` among the unaudited reads that stay
available while the journal is closed. The existing `disk_stats` suites and the
`agt disk` and doctor tests pass unchanged.

Fourteen independent mutations fail tests. They cover the journal path, the
reported home, each probe guard, the percentage, the field assignment in both
operations, the read-only flag, the registration and the journal-stats size.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
only.
