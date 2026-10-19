# W2.44b typed council ask and conductor ask

The Council of Elders' panel and the Conductor now run on the shared dispatcher
in `kernel/app/council`, beside the panel membership and the role preview. The
Web UI reaches the panel through `/api/council/ask`; the operator reaches the
Conductor through `agt conductor_ask`.

The two native handlers — `kernel/controlplane/council.go` and
`kernel/controlplane/conductor.go` — are gone. `kernel/controlplane/research.go`
keeps its native handler and borrows the moved correlation sanitizer.

## The second typed LIVE operation pair

`council_ask` and `conductor_ask` were `StreamLive` so that a disconnected
client cancels the model calls instead of spending every seat into a closed
connection. They never sent an event frame: the Web UI follows the run on the
bus under the correlation it chose.

The typed operations keep the LIVE metadata and declare kernel event frames, as
the shared adapter requires for any stream. Their emitters are never called.

Both are primary-only and audited. The audit arc is the shared one:
`op.invoked` before the panel runs and `op.completed` after it answers.

## The wire shape is unchanged

The legacy answers were `map[string]any`, so the wire was alphabetically
ordered. The app-owned path re-encodes the typed output through the same object
envelope, so the member order is the same as before — the typed structs do not
have to imitate the map order, and they do not.

The row shapes keep their legacy key sets:

- a panel opinion always carries its seat, model, round and text, and carries
  its error only when there is one;
- a panel member carries its seat and model;
- a conductor step always carries its round, role and model, and carries its
  text, verdict, reason, error and exec only when they are there;
- a conductor exec row always carries its four fields, even when the language
  and the output are empty.

`council_ask` keeps its Web UI route. `conductor_ask` has none — the Conductor's
nav cluster was removed in the Day-28 topology audit, so the operation declares
no HTTP hint, as `conductor_roles` already did.

## Preserved behavior

- **Question and task:** both are trimmed text. A blank one is refused with
  `args.question required` and `args.task required` before the engine is
  called.
- **Rounds:** a number is truncated, a string of decimal digits is read, and
  anything else is zero. The engine applies its own default.
- **Plan:** a boolean, or the strings `true` and `1`, turns the instruction
  tailoring on.
- **Correlation:** a client-supplied id is kept when it is a short plain token
  (`[A-Za-z0-9_-]`, at most 80 characters) and is otherwise replaced by a
  minted one. The sanitizer moved to `council.SanitizeCorrelation`, and the
  still-native `research_ask` uses it.
- **The panel:** the deliberation, the members, the consensus, the dissent, the
  date and the brief come from the engine, unchanged.
- **The Conductor:** the roles, the plan, the rounds, the transcript and the
  answer come from the engine, unchanged.

## The lenient boolean reader moved with the Conductor

`dlBool` in `kernel/controlplane/args_lenient.go` was only used by the
Conductor's `plan` argument. The typed operation reads the flag itself, so the
helper is deleted. The count reader stays, because `research_ask` is still
native and still uses it.

This closes the dead-code gate: before the slice, `tools/deadcodecheck` failed
with `kernel/controlplane/args_lenient.go:10: unreachable func: dlBool`.

## Runnable evidence

Permanent tests cover:

- the correlation sanitizer: trimming, the length cap, the refused characters;
- the panel: the refused question, the mapped deliberation, the members, the
  opinions, the lenient rounds, the minted correlation, the engine failure;
- the Conductor: the refused task, the mapped transcript, the roles, the exec
  row, the lenient plan flag;
- the specs: the names, the HTTP route, the primary-only scope, the LIVE stream
  and the declared event frames;
- the native binding: the panel and the Conductor run on the primary kernel, the
  answer keeps the nine and the eight legacy keys, the operation is audited
  with one `op.invoked` and one `op.completed`, and the empty question is a
  clean error.

Fifteen independent mutations fail tests: a fixed minted correlation, a widened
id cap, an accepted id character, an untrimmed question, untruncated rounds, a
plan flag that ignores `1`, a dropped opinion error, a dropped member model, a
dropped opinion seat, dropped roles, dropped max rounds, a dropped plan flag, a
dropped exec row, a dropped answer and dropped members. The mutation run found
one real gap — the opinion row's seat and model were not asserted — and the
test now asserts the whole row.

The suites pass twenty times over: `kernel/app/council` and the council,
conductor and chat control-plane tests. The full `kernel/controlplane` package
passes unchanged, including the existing `TestConductor_RolesAndAskViaControlPlane`
end-to-end test, which now runs through the typed path.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (unchanged at 131 allowlisted import violations and 13
allowlisted call sites) and `tools/structure-md` (regenerated).

## Not done here

The cloned-kernel harness of the earlier slices — the pre-slice handler and the
registered operation on two kernels, compared step by step — is not part of this
slice. The permanent tests above, the mutation run and the existing end-to-end
control-plane test cover the same ground for this pair, but the byte-level
side-by-side comparison the earlier evidence documents carry is not reproduced
here.
