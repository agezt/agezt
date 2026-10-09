# W2.40a typed budget

The two budget commands now run on the shared dispatcher in the new
`kernel/app/budget` package. They are the first of the provider-config group:

- `budget` returns the governor's spend snapshot for the current UTC day, the
  view behind `agt budget` and the console's budget panel;
- `budget_set` adjusts the global daily ceiling at runtime (M607) and returns
  the post-set snapshot in the same shape.

`Service` works over a `Governor` port with two methods, `Snapshot` and
`SetDailyCeiling`. The control plane binds it to the primary kernel's provider
when that provider is a `*governor.Governor`, converting the snapshot field by
field, and leaves it unbound otherwise. `Operations` declares two primary-only,
primary-tenancy specs with unknown input allowed, matching the native
registration:

- `budget`, read-only, on `GET /api/budget`;
- `budget_set`, audited, with no Web UI route, as before.

Removed with the move:

- `budget.go`, which held both handlers, `budgetResult` and `numArg`;
- their two registrations in `registerProviderConfigCommands`.

## Preserved behavior

- **Snapshot shape:** `utc_date`, `spent_mc`, `ceiling_mc`, `per_task` and
  `strict_pricing`.
  - `per_task` holds one row per configured per-task cap, with `task_type`,
    `spent_mc` and `ceiling_mc`, sorted by task type.
  - Without per-task caps it is an empty array, never null.
- **No governor:** both commands refuse with their own unchanged error text.
  For `budget_set` this refusal comes before any argument check.
- **`ceiling_mc`:**
  - Absent is "required". A present null, boolean, object or array is an
    "unexpected type" error naming the decoded type.
  - A number must be whole. A fraction, or a value that does not survive the
    int64 round trip such as `1e300`, is refused with the value printed.
  - A string is trimmed and parsed as a base-10 int64. Anything else, including
    `"4.0"`, an empty string and an out-of-range number, is refused with the
    quoted string.
  - Every error text is unchanged.
- **Setting:** a negative ceiling is passed through and the governor clamps it
  to 0 (unlimited). The governor still publishes its ceiling-set event. The
  returned snapshot is read after the set.
- Any `tenant` argument is ignored, and tenant tokens are refused.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before any read, audit or ceiling change.

## Runnable comparison and regression evidence

Each run opens a fresh kernel. The governed fixtures use a governor built over
a priced mock provider. It completes once with a `code` task type and once
without, so both the total spend and a per-task row are non-zero. Its bus is
wired to the kernel, so ceiling-set events reach the journal.

The harness runs the pre-slice handlers on one kernel and the registered
operations on another. 504 steps repeated twenty times cover three fixtures:

- no governor;
- a governor with real spend;
- a governor with per-task caps, including a zero cap, and strict pricing.

Each fixture runs 28 steps under primary, wrong and tenant tokens, in normal and
canceled contexts. The steps include:

- reads, with ignored and tenant arguments;
- every refused `ceiling_mc` form;
- sets through numbers, padded strings, negatives, whole floats, `2^53+1`, zero
  and a secret-bearing argument, each followed by a read.

The comparison checks two things:

- responses are byte-exact, with no masking;
- both journals, grouped by correlation, are equal: the operation audits and
  the governor's ceiling-set events.

The harness also asserts:

- the not-a-governor errors, the required and type errors, and the set values
  read back;
- the snapshot spend of 1,800,000 microcents and the sorted per-task rows with
  strict pricing;
- eight ceiling-set events per governed primary run.

The exceptions are 84 canceled primary-token steps. These return the admission
error and leave no journal record.

Permanent tests cover:

- **Get:** the refusal, the empty per-task array, row sorting and every field,
  and that a read never sets.
- **Set:**
  - the refusal order;
  - every refused argument with its exact error;
  - that a refusal never sets;
  - accepted numbers and strings;
  - the post-set snapshot and the governor's clamp.
- **Operations:** the specs and output schemas.
- **Binding:** both operations through the native adapter against a real
  governor, with exact JSON including per-task spend and strict pricing, the
  clamp, and the registry flags.

The existing budget suites pass unchanged through the typed path.

Thirty-two independent mutations fail tests. They cover:

- the sort, the empty array and each snapshot field;
- both guard texts and the guard order;
- the required check and every error form;
- the trim and the base;
- skipping the set, or reading the snapshot before it;
- the spec flags, the route and authorization;
- each field of the governor binding.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
only.
