# W2.42d typed reflection

The two reflection commands now run on the shared dispatcher in the new
`kernel/app/reflection` package. They are the path behind `agt reflect`.

- `reflect_run` triggers one reflection pass. A pass folds the journal into
  observations, applies world-model decay, derives advisory proposals and
  journals its report under the pass's correlation, so the decay is
  explainable via `agt why`.
- `reflect_show` reads the latest report back.

`Service` works over an `Engine` port and a correlation minter, bound to the
primary kernel's reflection engine and `ulid.New`. `Operations` declares two
primary-only, primary-tenancy specs with unknown input allowed and no Web UI
routes, as before. The trigger is audited.

Removed with the move:

- `reflect.go`, which held both handlers and the JSON round-trip report view;
- their two registrations in `registerCognitionCommands`.

The control plane no longer imports `kernel/reflect` directly, so the archcheck
allowlist drops that adapter-bypass edge.

## Preserved behavior

- **Run:**
  - Each pass runs under a fresh `reflect-<ulid>` correlation.
  - The pass is offline and deterministic. Once admitted it runs to completion
    on a background context, independent of the caller's.
  - The result is the report's fields beside `correlation_id`, not nested.
  - An engine error passes through.
- **Show:**
  - Before any pass, the result is exactly `{"found": false}`.
  - After a pass, it is `found` with the latest report.
- Any `tenant` argument is ignored, and tenant tokens are refused.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before any pass or read.

## Runnable comparison and regression evidence

The harness runs the pre-slice handlers on one kernel and the registered
operations on another. 72 steps repeated twenty times cover two fixtures: a
fresh journal, and prior activity across four event kinds. Each fixture runs
six steps under primary, wrong and tenant tokens, in normal and canceled
contexts: reads before, between and after two passes, with ignored, tenant and
secret arguments.

The comparison checks two things:

- responses are equal, apart from minted correlations and times;
- the journals written during the run are equal, under the same mask: the pass
  reports, the decay and the operation audits.

The harness also asserts:

- the first read's exact `found: false`;
- that each pass is journaled under the correlation it returned;
- that the latest report reads back;
- that the trigger is audited.

The exceptions are 12 canceled primary-token steps. These return the admission
error and journal nothing.

Permanent tests cover:

- **Run:**
  - the minted correlation and its prefix;
  - independence from a canceled caller context;
  - the flattened report with every field kept;
  - a passed-through engine error.
- **Show:** the exact not-found form, the found form, and that a read never runs
  a pass.
- **Operations:** the specs and input and output schemas.
- **Binding:** through the native adapter, each pass mints a distinct
  correlation and journals under it in the primary kernel, the latest report
  reads back, and the registry flags hold.

The existing reflect suite passes unchanged through the typed path.

Twelve independent mutations fail tests.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
only.
