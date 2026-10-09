# W2.39c typed redaction check

`redact_test` (M104) now runs on the shared dispatcher in the new
`kernel/app/redaction` package. It lets an operator confirm that the live
secret redactor catches a candidate before such a value could reach the
permanent journal. `agt redact` and the console's policy page call it.

`Service` works over two ports:

- the bus's live redactor, read at call time, and nil when redaction is off;
- the built-in pattern categories (`redact.MatchedCategories`).

`Operations` declares one primary-only, primary-tenancy spec, read-only and
unaudited, with unknown input allowed, on `POST /api/redact/test`. Because the
operation is read-only, no operation audit ever records the candidate. The
route matches the Web UI write route, so the candidate never travels in a URL.

Removed with the move:

- `redact_test_cmd.go`, which held `handleRedactTest` and its `toAnySlice`
  helper;
- its registration.

## Preserved behavior

- `text` is strict: it may be absent (empty), but a present `null` or
  non-string is `args.text must be a string`. The candidate passes untrimmed.
- With a live redactor, `redacted` is the scrubbed candidate. Without one,
  `enabled` is false and the candidate comes back unchanged.
- `categories` names the built-in patterns the raw candidate matches. It is
  always an array and is reported even with redaction off.
- `would_redact` means the redactor changed the candidate. `literal_hit` means
  a change no built-in pattern explains, that is, a configured literal; which
  literal is never said.
- Only the redacted form is returned.
- Any `tenant` argument is ignored, and tenant tokens are refused. The redactor
  is the primary bus's.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before the candidate is examined.

## Runnable comparison and regression evidence

The harness clones a fixture kernel and runs the pre-slice handler on one copy
and the registered operation on another. Each copy runs under one of three
redactor states:

- none;
- the built-in patterns;
- the built-in patterns plus a configured literal.

The secret-shaped candidates (a bearer token, a connection-string password and
the literal) are assembled at run time.

216 steps repeated twenty times cover twelve candidates under primary, wrong
and tenant tokens, in normal and canceled contexts:

- plain prose;
- each secret shape alone and combined;
- a padded literal;
- absent, empty, numeric, `null` and list candidates;
- an HTML-bearing literal with a named tenant and secret-named arguments.

Responses are byte-exact, and neither journal changes. The harness asserts:

- the disabled report;
- the bearer and connection-string categories;
- the literal hit;
- the strict-string error;
- that a scrubbed bearer never comes back.

The exceptions are 36 canceled primary-token steps. These return the admission
error.

Permanent tests cover:

- every non-string candidate, rejected before any categorization;
- the untrimmed candidate;
- a pattern hit, a literal hit and plain text;
- redaction off;
- an absent candidate's exact wire form;
- the spec and output schema.

Native tests cover the registry flags. A native round trip installs a redactor
after the daemon starts, then checks that the check uses it, never returns the
literal and never journals the candidate. The existing controlplane redact
suite passes unchanged.

Fourteen independent mutations fail tests. They cover:

- the strict codec and the untrimmed candidate;
- the redact call;
- categorizing the raw (not the redacted) candidate, and the array shape;
- the would-redact, enabled, literal-hit and redacted-echo fields;
- the route, the read-only flag and the provider guard;
- the native registration and category binding.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
only.
