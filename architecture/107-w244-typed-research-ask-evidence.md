# W2.44c typed research ask

The deep-research harness now runs on the shared dispatcher in
`kernel/app/research`. The operator, the Web UI and the CLI reach it through
`research_ask` and the Web UI's `/api/research/ask` route.

The native handler — `kernel/controlplane/research.go` — is gone.

## The third typed LIVE operation

`research_ask` was `StreamLive` because the harness is model-heavy: a
disconnected client cannot receive the report, so the run is cancelled instead
of being spent into a closed connection. The typed operation keeps the LIVE
metadata and declares kernel event frames, as the shared adapter requires for
any stream. Its emitter is never called.

It is primary-only and audited: the audit arc is the shared one, `op.invoked`
before the harness runs and `op.completed` after it answers.

## The wire shape is unchanged

The legacy answer was a `map[string]any` with ten keys, and the app-owned path
re-encodes the typed output through the same object envelope, so the member
order is the same as before. Two row shapes needed care:

- a source carries its id, url, title, rank and hash — and no text. The
  runtime's source type has a text field; the report cites the source, the
  harness keeps the body, so the typed row leaves the text out.
- a claim always carries its text, source ids, verdict and note, even when the
  note is empty. The runtime's claim type omits an empty note, so the typed
  row keeps it.

The empty lists stay empty lists: an empty report answers with `[]` for its
sources and its claims, not `null`, so the Web UI can iterate them.

## Preserved behavior

- **Question:** trimmed text. A blank one is refused with
  `args.question required` before the harness is called.
- **Caps:** `max_sub_questions`, `max_sources` and `max_verify_claims` accept a
  number (truncated) or a string of decimal digits, and read as zero otherwise.
  The harness applies its own defaults and caps.
- **Verification:** the adversarial claim-verification pass is on unless an
  explicit boolean `false` turns it off. A string, a number or a missing
  argument leaves it on, as before.
- **Correlation:** a client-supplied id is kept when it is a short plain token
  (`[A-Za-z0-9_-]`, at most 80 characters) and is otherwise replaced by a
  minted one.

## The lenient readers moved to the app layer

The panel, the Conductor and the harness all read their arguments the same
lenient way: a flag that accepts a JSON boolean or the strings `true` and `1`,
a count that accepts a number or a digit string, and trimmed text. The readers
and the correlation sanitizer now live once, in `kernel/app/lenient.go`, and
the three packages call them from there.

That closes the last of the duplication: `kernel/controlplane/args_lenient.go`
is gone, and with it the `dlInt` helper that only `research_ask` used. The
`stringArg` helper in `kernel/controlplane/send.go` goes with it — `research_ask`
was its last caller. The dead-code gate is green.

## Runnable evidence

Permanent tests cover:

- the refused question, before the harness is called;
- the report mapping: the sources carry no text, the claim keeps its empty
  note, the empty lists stay lists;
- the lenient caps and the verification flag, including the string `false` that
  does not turn the pass off;
- the client correlation that is kept and the odd one that is minted;
- the harness failure that becomes the operation's failure;
- the spec: the name, the route, the primary-only scope, the LIVE stream and
  the declared event frames;
- the shared readers, in the app layer.

At the control plane, the harness runs on the primary kernel, the answer keeps
the ten legacy keys, the operation is audited with one `op.invoked` and one
`op.completed`, and the empty question is a clean error.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (268 packages placed, unchanged at 131 allowlisted import
violations and 13 allowlisted call sites) and `tools/structure-md`.

## Not done here

The cloned-kernel harness of the earlier slices — the pre-slice handler and the
registered operation on two kernels, compared step by step — is not part of
this slice, as in W2.44b.
