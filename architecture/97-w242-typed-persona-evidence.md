# W2.42a typed persona and prompts

Four owner-editing commands now run on the shared dispatcher in the new
`kernel/app/persona` package. They are the first of the cognition group.

- `persona_get` returns the daemon default identity (M710): the fallback system
  instructions for runs not bound to a roster agent.
- `persona_set` replaces that identity.
- `prompts_get` returns the saved chat prompt library (M713) the Chat view
  launches from.
- `prompts_set` replaces the library.

`Service` works over ports bound to the primary kernel and the server's base
directory:

- the kernel's live identity and its setter;
- the config store;
- the bytes of `chat_prompts.json`, written with owner-only permissions.

`Operations` declares four primary-only, primary-tenancy specs with unknown
input allowed, on their Web UI routes. Both edits are audited.

Removed with the move:

- `persona.go` and `prompts.go`, which held the four handlers and the library
  helpers;
- their four registrations in `registerCognitionCommands`.

## Preserved behavior

- **Identity:**
  - The identity is returned in full, with whether one is set.
  - `system` is required, and a present non-string, null included, is refused.
  - The text is kept exactly as given and persisted as `AGEZT_SYSTEM_PROMPT`.
    Only the empty string removes it; blank text is kept.
  - It is applied live only after the store saves.
  - The result reports the byte length.
  - Load and save failures keep their texts.
- **Library:**
  - A missing, unreadable or corrupt file reads as an empty list, so it never
    breaks the Chat view.
  - `prompts` is required and must be an array.
  - Non-object entries are skipped. Titles and texts are trimmed, and entries
    missing either are dropped.
  - Fields are cut at 120 and 8000 bytes, and at most 100 entries are kept.
  - The file is indented JSON.
  - Encode and save failures keep their texts.
- Any `tenant` argument is ignored, and tenant tokens are refused.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before any read, audit or write.

## Runnable comparison and regression evidence

The harness runs the pre-slice handlers on one kernel and the registered
operations on another. Each kernel starts with a seeded identity. 378 steps
repeated twenty times cover three fixtures:

- a fresh daemon;
- a corrupt prompt library;
- an unreadable config store, with an existing library.

Each fixture runs 21 steps under primary, wrong and tenant tokens, in normal and
canceled contexts. The steps include:

- every refused identity and library form;
- an HTML-bearing multibyte identity, a blank one and a clearing one, each read
  back;
- a library mixing kept, skipped, incomplete, mistyped and over-long entries,
  then an empty library, each read back.

The comparison checks four things:

- responses are byte-exact, apart from the per-run root in config-load errors;
- the config stores and library files are equal;
- the live identity is equal;
- both journals, grouped by correlation, are equal: the operation audits.

The harness also asserts the errors, the stored and returned text, the byte
length, the caps and the filtering. The exceptions are 63 canceled
primary-token steps. These return the admission error and leave the identity,
store, library and journal unchanged.

Permanent tests cover:

- **Identity:**
  - every refusal, which never opens the store;
  - load and save failures, and that a failed save never applies;
  - the kept text and its byte length;
  - clearing only on the empty string.
- **Library:**
  - missing, corrupt, mistyped and null files;
  - extra fields;
  - every refusal, which never writes;
  - trimming, filtering, both caps, the entry limit, the indented file, an empty
    library and a save failure.
- **Operations:** the specs and input and output schemas.
- **Binding:** through the native adapter, the identity applies to the primary
  kernel and persists, with the library, under the server's directory and not
  the kernel's. Registry flags are checked too.

The existing persona and prompt suites pass unchanged through the typed path.

Thirty-one independent mutations fail tests. A mutation of the library file's
permissions was left out, because Windows cannot observe it; the binding test
checks the permissions where the platform reports them.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
only.
