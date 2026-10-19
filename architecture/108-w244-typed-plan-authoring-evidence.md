# W2.44d typed plan generate and plan refine

The plan authoring surface now runs on the shared dispatcher, in
`kernel/app/runs` beside the plan store. The operator, the Web UI and the CLI
reach it through `plan_generate` and `plan_refine`, and the Web UI through
`/api/plan/generate` and `/api/plan/refine`.

The native handlers — `kernel/controlplane/planner.go` — are gone.

## The fourth and fifth typed LIVE operations

Both commands were `StreamLive`: the plan is a model call, and a disconnected
client cannot receive it, so generation is cancelled instead of being spent
into a closed connection. The typed operations keep the LIVE metadata and
declare kernel event frames, as the shared adapter requires for any stream.
Their emitters are never called.

Both are primary-only and audited: the audit arc is the shared one.

## The wire shape is unchanged

The legacy answers were `map[string]any` with two keys — the plan JSON and the
node count — and the app-owned path re-encodes the typed output through the
same object envelope, so the member order is the same as before.

## Preserved behavior

- **Required arguments:** the intent, the plan and the feedback are required.
  A missing argument and a blank one are refused with `args.<name> required`;
  an argument that is present but is not a string is refused with
  `args.<name> must be a string`. An explicit `null` is a wrong type, not a
  missing argument, as before.
- **The model override:** the caller's own string reaches the planner
  unchanged, and an empty one falls back to the provider's default.
- **The intent:** it reaches the planner as it was given, untrimmed, as before.
- **The provider:** a daemon without a provider answers "daemon has no
  provider configured", the operator's answer rather than a planner error.
- **A broken plan:** a plan that is not JSON is refused with the `plan_json:`
  prefix and the decoder's message.

## The plan JSON is a string

The plan arrives as a JSON string, not as an embedded object: the CLI forwards
the JSON the planner returned, and the Web UI posts the same string. The typed
operation reads it as a string and parses it, exactly as the native handler
did.

## The archcheck ratchet moved

`kernel/controlplane/planner.go` was the last control-plane file that imported
`kernel/planner` directly. With it gone, that edge no longer occurs, and the
allowlist entry that tracked it was tightened: the gate now reports 130
allowlisted import violations and 13 allowlisted call sites, down from 131 and
13.

## Runnable evidence

Permanent tests cover:

- the refused intent, the refused plan and the refused feedback, each with its
  own message;
- the wrong-type refusal, including the explicit `null` that is a type error
  and not a missing argument;
- the model override, which must be a string and reaches the planner as given;
- the intent, which reaches the planner untrimmed;
- the daemon without a provider;
- the planner failure, which becomes the operation's failure;
- the broken plan, refused with the `plan_json:` prefix;
- the answers: the plan JSON and the node count, generated and revised;
- the specs: the names, the two routes, the primary-only scope, the LIVE
  stream and the declared event frames.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck` (the
now-unused `requiredArgString` leaves the control plane with the plan
handlers), `tools/archcheck` (268 packages, 130 allowlisted import violations
and 13 allowlisted call sites) and `tools/structure-md`.

The existing end-to-end control-plane tests carry the migration: they call
`plan_generate` through the client and now run through the typed operation.
They still see the `plan_json` and the `node_count`, the refused intent and
the planner's own validation failure.

## Not done here

The cloned-kernel harness of the earlier slices — the pre-slice handler and the
registered operation on two kernels, compared step by step — is not part of
this slice, as in W2.44b and W2.44c.
