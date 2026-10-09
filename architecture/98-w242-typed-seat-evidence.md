# W2.42b typed seats

The three seat commands now run on the shared dispatcher in the new
`kernel/app/seats` package. Seats are the execution profiles a workboard task
can be dispatched under: the seeded built-ins plus operator-defined custom
seats. Each pins an isolation profile, a model chain and an optional tool
allowlist.

- `seat_list` lists every seat with the count.
- `seat_create` adds a custom seat.
- `seat_delete` removes a custom seat.

`Service` works over a `Store` port bound to the primary kernel's seat store,
which keeps its own validation and normalisation. `Operations` declares three
primary-only, primary-tenancy specs with unknown input allowed and no Web UI
routes, as before. Both edits are audited.

Removed with the move:

- `seat.go`, which held the three handlers and the JSON round-trip view;
- `workboard_dispatch_args.go`, whose list reader had no other caller;
- their three registrations in `registerCognitionCommands`.

The control plane no longer imports `kernel/seat` directly, so the archcheck
allowlist drops that adapter-bypass edge.

## Preserved behavior

- **List:** the built-ins and then the custom seats, always an array, with the
  count.
- **Create:**
  - `id`, `name`, `description` and `execution_profile` are trimmed strings;
    any other value reads as empty.
  - `model_chain` and `tools` take an array, keeping its trimmed non-blank
    strings, or a non-blank string split on commas as given; anything else is
    none.
  - `restrict_tools`, when present, must be a boolean. Null included, anything
    else is refused before the store sees the seat.
  - The store lower-cases the id and profile, fills the name, cleans the lists
    and turns on tool restriction when tools are listed. Its id, built-in,
    profile and duplicate errors keep their texts.
- **Delete:**
  - `id` is a trimmed string; blank or absent is "seat_delete requires id".
  - The trimmed id is echoed as given.
  - The store's not-found and built-in refusals keep their texts.
- Any `tenant` argument is ignored, and tenant tokens are refused.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before any read, audit or change.

## Runnable comparison and regression evidence

The harness runs the pre-slice handlers on one kernel and the registered
operations on another. 132 steps cover 22 steps under primary, wrong and
tenant tokens, in normal and canceled contexts, repeated twenty times. The
steps include:

- every refused creation, including a mistyped id, a built-in id, a remote
  profile and mistyped or null `restrict_tools` both before and alongside an
  invalid id;
- a padded, HTML-bearing seat with string and array lists;
- a pure-reasoning seat, a seat with array and comma lists, and a duplicate;
- every refused deletion, then a padded deletion and its repeat;
- listings between the edits.

The comparison checks three things:

- responses are byte-exact, with no masking;
- the seat stores are equal;
- both journals, grouped by correlation, are equal: the operation audits.

The harness also asserts every error, the normalised seats and the final store.
The exceptions are 22 canceled primary-token steps. These return the admission
error and leave the store at its built-ins and the journal unchanged.

Permanent tests cover:

- both readers;
- listing, including an empty store;
- creation with every refusal and the normalised results;
- deletion with every refusal and the echo;
- the specs and input and output schemas.

These tests use a real store. A native binding test drives the operations
against the primary kernel's store and checks the registry flags.

Twenty-one independent mutations fail tests. The control-plane binding is a
one-line pass-through to the kernel's store, covered by the binding test.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
only.
