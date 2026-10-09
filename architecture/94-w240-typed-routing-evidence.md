# W2.40b typed routing and chains

The four model-routing commands now run on the shared dispatcher in the new
`kernel/app/routing` package:

- `routing_get` returns the governor's per-task model fallback chains (M703),
  the task types the Routing view seeds rows from, and each task's model-chain
  fallback activity from the journal (M706);
- `routing_set` replaces the per-task chains;
- `chains_get` returns the named `@name` chains (M963), the default chain and
  what references each chain (M964);
- `chains_set` replaces the named chains and the default.

`Service` works over ports bound to the primary kernel:

- governor reads and writes of both registries, asserted by method so any
  provider carrying one is used, exactly as before;
- the roster's agent models and fallbacks;
- the journal;
- the server's config store;
- one catalog snapshot for the unknown-model report.

`Operations` declares four primary-only, primary-tenancy specs with unknown
input allowed, on the Web UI routes: `GET /api/routing`, `POST
/api/routing/set`, `GET /api/chains` and `POST /api/chains/set`. Both edits are
audited.

Removed with the move:

- `routing.go`, `chains.go` and `config_helpers.go`, which held the four
  handlers, the activity fold, the usage scan, the chain codec, the task-type
  list and `stringSliceMapToAny`;
- their four registrations in `registerProviderConfigCommands`.

## Preserved behavior

- **`routing_get`:**
  - Without a governor, chains are an empty object.
  - Chains are always an object of arrays.
  - Activity folds `provider.fallback` events with the `model-chain` scope by
    task type, with no task as `(unknown)`, in journal order.
  - Each task keeps its count and the newest failed model, next model and time.
    Its last non-empty reason is cut to 160 bytes on a rune boundary.
  - Provider-scope fallbacks and undecodable payloads are skipped.
- **`chains_get`:**
  - Without a governor, chains are an empty object and the default is empty.
  - Usage scans every agent's model and fallbacks and every per-task chain for
    `@name` references, ignoring a bare `@`.
  - A defined chain gets its sorted, deduplicated agents and task types and
    whether it is the default. The default is listed even when nothing
    references it.
  - Names referenced but not defined are listed, sorted, under `__dangling__`.
- **Both edits:**
  - `chains` is required. A present non-object is "chains must be an object
    {task: [models]}", and a non-array entry is refused by name.
  - Keys and models are trimmed. Blank keys, non-string and blank models, and
    keys left empty are dropped.
  - The env spec is written sorted to the server's config store, or removed
    when empty; load and save failures keep their texts.
  - Edits apply live when the provider carries the registry, and still report
    `applied: live` when it does not.
  - Models the catalog does not know are reported sorted and deduplicated,
    never refused.
- **`chains_set`** also requires slug names and refuses a chain that references
  another chain. `default` is a trimmed string, any other value reads as none,
  and it must name a defined chain. Both `AGEZT_FALLBACK_CHAINS` and
  `AGEZT_DEFAULT_CHAIN` are written or removed.
- Every error text is unchanged. Any `tenant` argument is ignored, and tenant
  tokens are refused.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before any read, audit or config write.

## Runnable comparison and regression evidence

The harness runs the pre-slice handlers on one kernel and the registered
operations on another. Each run opens a fresh kernel with a catalog that knows
one model. 558 steps repeated twenty times cover three fixtures:

- no governor;
- a governor with named chains, a default, per-task chains, three agents
  referencing defined, undefined and bare chains, and five fallback events
  including provider-scope, mistyped and long multibyte ones;
- a governor over an unreadable config store.

Each fixture runs 31 steps under primary, wrong and tenant tokens, in normal and
canceled contexts. The steps include:

- reads before and after every successful edit;
- every refused argument form;
- edits with padded keys and models, mixed entries, secrets, a non-string
  default and clearing edits.

The comparison checks three things:

- responses are byte-exact, apart from the per-run root in config-load errors
  and fallback times;
- the config stores are equal;
- both journals, grouped by correlation, are equal, apart from the per-run
  root: the operation audits.

The harness also asserts the usage maps, activity, persisted specs, unknown
models and errors. The exceptions are 93 canceled primary-token steps. These
return the admission error and leave no journal record or config write.

Permanent tests cover:

- **Routing:** the empty form, and the fold with every skipped and kept event
  shape.
- **Codec:** every refused shape, trimming and dropping, and sorted encoding.
- **Edits:**
  - every refusal, which never opens the store;
  - load and save failures, and that a failed save never applies;
  - the persisted set or remove calls;
  - live apply, the unknown-model report and the ungoverned path.
- **Chains:** usage over defined, undefined, bare, default and unreferenced
  chains.
- **Operations:** the specs and input and output schemas.
- **Binding:** all four operations through the native adapter. Edits persist to
  the server's config store and reach a real governor, the catalog decides
  unknown models, usage reads the roster, and the registry flags hold.

The existing chains-usage, routing-activity and chains-set suites pass
unchanged through the typed path.

Fifty-seven independent mutations fail tests. They cover:

- the wire shapes and every activity rule;
- each codec rule and the sorted encoding;
- persistence, live apply and the unknown-model report;
- each usage rule and each chain check;
- the spec flags and route;
- each binding port.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
only.
