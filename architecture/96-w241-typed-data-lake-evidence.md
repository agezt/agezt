# W2.41a typed data lake

The seven data lake commands now run on the shared dispatcher in the new
`kernel/app/data` package. They are the operator's window onto the personal
data lake (M836): the Web UI Data view and `agt data` browse and lightly edit
the structured collections agents build with the db tool (M834/M835).

- `data_collections` lists every collection with its record count.
- `data_records` queries one collection.
- `data_insert`, `data_update` and `data_delete` edit records.
- `data_create_collection` and `data_drop_collection` add and remove
  collections.

`Service` works over a `Lake` port bound to the primary kernel's data lake, and
refuses every operation when the kernel has none. `Operations` declares two
read-only and five audited specs, all primary-only and primary-tenancy with
unknown input allowed. The two views and the three record edits are on their
Web UI routes; the collection edits have none, as before.

Removed with the move:

- `datalake.go`, which held the seven handlers and their row and schema
  helpers;
- `registerDatalakeCommands`;
- `failMsg` in `respond.go`, whose only callers were these handlers.

The control plane no longer imports `kernel/datalake` directly, so the archcheck
allowlist drops that adapter-bypass edge and the ratchet keeps it from coming
back.

`dlBool` and `dlInt`, which the council, conductor and research commands still
use, move unchanged to `args_lenient.go`.

## Preserved behavior

- **No lake:** every command refuses with "data lake unavailable" before any
  argument check.
- **Arguments:**
  - `collection`, `id` and `name` are strict: absent or blank is "required", a
    present non-string (null included) is "must be a string". They pass
    untrimmed.
  - `search` and `sort` are trimmed strings; any other value reads as none.
  - `desc` is true for a JSON true or the strings `"true"` and `"1"`.
  - `limit` and `offset` take a number, truncated, or a string of decimal
    digits. Any other value reads as zero.
  - `record` and the collection definition are optional objects: absent and
    null read as none, any other value is "must be an object".
- **Collections:**
  - Every schema field is always present, and the field list is always an
    array.
  - A definition reads non-string values as empty and skips non-object field
    entries; a missing name is "collection.name required".
  - Created collections report a count of zero.
- **Records:**
  - Every record field and its provenance are always present. Operator edits
    are attributed to `operator`.
  - A query returns the collection, its schema counted by the page, the count
    and the records, an empty array when none match.
- **Errors:** a missing collection or record is "no such collection or record:"
  with the collection, or the collection and id for updates and deletes. An
  existing collection, an invalid name, a system collection and a missing
  collection on drop keep their texts.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before any read, audit or change.

## Runnable comparison and regression evidence

The harness runs the pre-slice handlers on one kernel and the registered
operations on another. 276 steps cover 46 steps under primary, wrong and
tenant tokens, in normal and canceled contexts, repeated twenty times. Later
steps refer to the records each run inserted, by that run's own ids. The steps
include:

- every refused collection definition;
- a full definition with a mistyped icon, a skipped field entry and a field with
  a mistyped type, and a duplicate;
- inserts with nested, list, HTML-bearing and secret-bearing values and a null
  record;
- queries with every argument form;
- updates, including a null field and a null patch;
- repeated deletes and drops.

The comparison checks two things:

- responses are equal, apart from the per-run record ids and times;
- both journals, grouped by correlation, are equal under the same mask: the
  operation audits.

The harness also asserts every error, the created collection's shape, the
inserted fields, the counts, the null-patch removal and the delete and drop
effects. The exceptions are 46 canceled primary-token steps. These return the
admission error and leave no journal record or insert.

Permanent tests on a real lake cover:

- the unavailable refusal for every operation;
- every argument reader;
- collection creation, its errors, and the always-present fields;
- record insert, update and delete with their errors;
- search, empty results, limit, offset, both sort directions against the
  creation-time default, and provenance copying;
- drops, including a seeded system collection;
- the specs and input and output schemas.

A native binding test drives the operations against the primary kernel's lake
and checks the registry flags.

Forty-four independent mutations fail tests. Five first survived and closed
real gaps:

- copying `updated_by` from `created_by`, because every fixture had equal
  provenance;
- dropping the sort field, because the creation-time default matched the
  fixture's text order;
- dropping the limit, because the paging check also applied an offset;
- a null record list for an empty query, because no test read an empty result;
- the system-collection refusal text, because a fresh lake has no system
  collection until the built-ins are seeded.

Each now has a test.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
only.
