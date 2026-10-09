# W2.42f typed chat suggestions

The chat surface's next-prompt chips, `chat_suggestions`, now run on the shared
dispatcher in the new `kernel/app/chat` package. The Web UI asks for them after
each chat turn through `/api/suggestions`. Summarizing a chat, `chat_summarize`,
streams a model call and stays native with the streaming cognition commands.

The suggestion catalog, the memory ranking and phrasing, the snippet trimming
and the ID dedupe moved verbatim, as did their unit tests.

`Service.Suggestions` reads one port: the primary kernel's active memory. A
missing manager leaves the port nil and a read error is ignored; both give no
memory-derived chips, as before. `Operations` declares one read-only,
primary-only, primary-tenancy spec on `/api/suggestions`, with unknown input
allowed.

Removed with the move:

- `chatsuggestions.go`;
- its registration in `registerCognitionCommands`;
- its entry in the raw `req.Args` cast ratchet.

## Preserved behavior

- Memory chips lead, at most three. Only preferences, summaries, facts and
  relations with a subject count. They are ranked by confidence, then by
  recency, and deduped by subject regardless of case. Each is phrased by type,
  with the content collapsed to one line and cut at 140 runes with an ellipsis.
- The tool-context catalog fills the rest, deduped by ID, five in all.
  - With no tools, the first four catalog entries.
  - File-editing, shell, web and git tools pick their sets, matched regardless
    of case, at most four.
  - Unrecognized tools get the generic four.
- A present `session_id` must be a string; anything else, `null` included, is
  refused with `args.session_id must be a string`.
- `tools` is comma-joined text, as the Web UI's read proxy forwards it, or an
  array. Entries are trimmed and blanks dropped. Non-string entries and any
  other type are ignored.
- Each chip keeps its declared member order (`id`, `label`, `prompt`,
  `category`, then `icon` when set). The typed adapter's generic object codec
  sorts nested object keys, so it passes the chip structs through, as it
  already does for other legacy embedded structs.
- Any `tenant` argument is ignored, and tenant tokens are refused.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before any read.

## Runnable comparison and regression evidence

The harness runs the pre-slice handler on one kernel and the registered
operation on another. 432 steps repeated twenty times cover four memory
fixtures under primary, wrong and tenant tokens, in normal and canceled
contexts:

- no memory;
- noise only: an observation and a blank subject;
- one summary;
- a mix of every type with a duplicate subject in another case, an
  HTML-bearing subject, and a long multi-byte, multi-line content.

Each fixture runs eighteen inputs. They cover:

- the session id as a string, a number, `null`, a boolean and an array;
- tools as text with spacing, blanks and case, an array with non-strings,
  blanks and padding, a number, an object, `null`, empty text and an empty
  array;
- unknown tools and every tool group;
- extra and `tenant` arguments.

Responses are byte-exact, the journals are equal, and no read journals
anything. The ranking, the dedupe, the ellipsis and the session check are
asserted. The exceptions are 72 canceled primary-token steps, which return the
admission error.

The first parity run caught the member-order drift described above before the
pass-through was added.

Permanent tests cover:

- the moved helper tests;
- the codec, the memory port and its read error, both caps and the wire order;
- the spec and its schemas;
- a native binding test that reads the raw response line, so the member order
  is pinned, from the primary kernel's memory, checks the session id error and
  the registry flags.

The existing integration suites pass unchanged through the typed path.

Fourteen independent mutations fail tests.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
only.
