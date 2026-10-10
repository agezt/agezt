# W2.43 typed file group

The File Manager's mutations now run on the shared dispatcher in
`kernel/app/files`, as does the snapshot restore behind `/api/rollback/apply`.
That package already owned their governed tool adapters. The four operations
are `file_mkdir`, `file_rename`, `file_delete` and `file_restore`. The Web UI's
file handlers still call them by name.

## What changed

**Typed outputs.** `Apply` and `ApplyRestore` now return typed outputs instead
of maps.

- `MutationOutput` carries `ok` and the path, or for a rename the source and
  target. Each is an optional pointer, so a key the legacy map lacked stays
  absent.
- `RestoreOutput` carries the checkpoint, `applied`, the reason when it was not
  applied, and the restore's result when it was.

**Service.** `Service` is bound per invocation to four things: the kernel as
the governed tool runner, the operation's audit correlation, the request's id,
and the daemon's rollback catalog path.

- `Mutate` rebuilds the arguments the legacy code read, so non-text paths and
  non-boolean flags still read as empty and false.
- `Restore` reads only `id`, and only when it is text.
- The operation handlers report any unclassified failure as
  `file_unavailable`, as the native handlers did.

**Shared adapter.** Two additions:

- If a typed operation's error classifies itself through `ErrorCode()`, the
  adapter writes the code to `error_code` beside the message. The dispatcher's
  audit join preserves the classification.
- It passes the transport request's id to the binding. The binding runs the
  governed tool with that id as the call id, under the correlation the router
  minted for the audited operation. The File Manager's audit arc asserts this
  identity.

**Specs.** Four audited, primary-only, primary-tenancy specs with unknown input
allowed and no shared Web UI route. The console workspace is daemon-global, so
tenant tokens are refused, as before.

**Removed:** `files.go`, `files_restore.go` and `registerFileCommands`.

## Preserved behavior

- Each mutation is admitted through policy and the mandatory tool audit before
  any root creation or path resolution:
  - `op.invoked`, `policy.decision`, `tool.invoked`, `tool.result`, then
    `op.completed`;
  - or `op.failed` after a denial;
  - all under one correlation and one call id.
- Mkdir and rename need `file.write`; delete needs `file.delete`. A restore
  needs `file.write`, or `file.delete` for an absent snapshot.
- Every error message and domain code is unchanged. The codes are
  `file_invalid_path`, `file_not_found`, `file_symlink`, `file_io`,
  `file_denied`, `file_unavailable` and `file_restore_mark_failed`.
- A restore reads only the daemon's catalog and never snapshot data sent by the
  caller.
  - A repeat returns `applied: false` with `already applied`.
  - A success stamps `applied_ms`.
  - The checkpoint keeps its declared member order: the adapter passes the
    struct through, since its generic codec would sort nested keys.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before any audit or filesystem effect.

## Runnable comparison and regression evidence

The harness runs the pre-slice handlers, over a temporary copy of the
pre-slice `app/files` code, on one kernel and the registered operations on
another. Each side has its own workspace and rollback catalog. 672 steps
repeated twenty times cover four fixtures under primary, wrong and tenant
tokens, in normal and canceled contexts:

- default allow;
- `file.write` and `file.delete` denied by policy;
- the kernel's built-in tool invoker;
- an unreadable catalog.

Each run executes twenty-eight stateful steps:

- **mkdir:** with and without parents, an existing directory, a string flag,
  an escaping path, a non-text path, no arguments;
- **rename:** a success, a missing source, a missing or escaping target,
  non-text arguments;
- **delete:** a success, a repeat, non-empty directories with and without a
  real recursive flag, an absolute path;
- **restore:** no id, a non-text id, an unknown id, a non-file checkpoint, a
  padded id with caller snapshot data, a repeat, an absent snapshot, an
  already-applied checkpoint, a broken snapshot;
- **tenant-named:** a call naming a tenant.

The results:

- Responses, error codes included, are byte-exact.
- Journals are equal: the audit arc, policy decisions, tool events and call
  ids. Two values are normalized: the time-decayed failure weight, and
  `applied_ms` in the catalog.
- Workspace trees and catalogs are equal after each run.
- Refused callers change nothing.
- The 112 canceled primary-token steps return the admission error. They
  journal nothing and leave the workspace and catalog untouched.

Permanent tests cover:

- the lenient argument decoding and the call identity, against a fake governed
  runner;
- each failure classification;
- the workspace adapter's mkdir, rename, delete, not-found and missing-target
  paths, and the rename output's wire shape;
- restore validation, success, repeat and member order, and an unreadable
  catalog;
- the unavailable fallback;
- the four specs;
- a native binding test that checks:
  - the server's catalog;
  - the request id as the call id under the operation's correlation;
  - the raw checkpoint member order;
  - the registry flags.

The existing suites pass unchanged through the typed path:

- the client error-code suite, in every client mode;
- the daemon-catalog restore suite;
- the Web UI's file and rollback suites, including the audit arc.

Twenty independent mutations fail tests. One mutant failed to build; it was
rewritten and the run resumed.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels,
workspaces and catalogs only.
