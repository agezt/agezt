# W2.42c typed council membership

The Council of Elders' default membership (M839) now runs on the shared
dispatcher in the new `kernel/app/council` package. The membership decides which
models speak when the multi-model panel is convened without an explicit panel.

- `council_members` returns the default panel.
- `council_set` replaces it.

Convening the panel, `council_ask`, streams its deliberation and stays native
with the streaming cognition commands.

`Service` works over ports bound to the primary kernel and the server's base
directory:

- the kernel's default panel, read as it reports it;
- the panel's live setter;
- the config store;
- one catalog snapshot for the unknown-model report.

`Operations` declares two primary-only, primary-tenancy specs with unknown input
allowed, on their Web UI routes. The edit is audited.

Removed with the move:

- the two handlers and the member wire type, from `council.go`. The streaming
  ask and `sanitizeCorr` stay there.
- their two registrations in `registerCognitionCommands`.

## Preserved behavior

- **Members:** the panel as the kernel reports it, always an array, with the
  count.
- **Set:**
  - `members` is required and must be an array.
  - Each entry must be an object with a non-empty string model; the first
    failing entry is named by index.
  - Seats and models are trimmed after that check, so a blank model is kept as
    an empty one.
  - The entries are sorted by seat. Their models are persisted comma-joined as
    `AGEZT_COUNCIL_MEMBERS`, or removed when there are none. Load and save
    failures keep their texts.
  - The panel is applied live only after a successful save. A blank seat is
    named `Elder N` by its sorted position.
  - Models the catalog does not know are reported sorted and deduplicated, and
    are never refused.
- Any `tenant` argument is ignored, and tenant tokens are refused.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before any read, audit or change.

## Runnable comparison and regression evidence

The harness runs the pre-slice handlers on one kernel and the registered
operations on another. Each kernel has a catalog that knows one model. 288
steps repeated twenty times cover three fixtures:

- no panel;
- a configured panel with a duplicate seat;
- an unreadable config store.

Each fixture runs 16 steps under primary, wrong and tenant tokens, in normal and
canceled contexts. The steps include:

- every refused form;
- a mixed panel with padded, HTML-bearing, mistyped and blank seats and a blank
  model;
- a duplicate-seat panel with a secret argument;
- a clearing edit, each followed by a read.

The comparison checks four things:

- responses are byte-exact, apart from the per-run root in config-load errors;
- the config stores are equal;
- the live panel is equal;
- both journals, grouped by correlation, are equal: the operation audits.

The harness also asserts the errors, the sorted and named panel and the unknown
models. The exceptions are 48 canceled primary-token steps. These return the
admission error and leave no journal record or config write.

Permanent tests cover:

- **Members:** the empty and populated forms.
- **Set:**
  - every refusal, which never opens the store;
  - load and save failures, and that a failed save never applies;
  - the persisted order, the live naming, the unknown models, clearing, and a
    panel of known models.
- **Operations:** the specs and input and output schemas.
- **Binding:** through the native adapter, the edit persists under the server's
  directory and applies to the primary kernel's panel, the catalog decides
  unknown models, and the registry flags hold.

Twenty-eight independent mutations fail tests.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
only.
