# W2.44e typed plan execution

The plan execution now runs on the shared dispatcher, in `kernel/app/runs`
beside the plan store. The operator, the Web UI and the CLI reach it through
`plan` and the Web UI through `/api/plan/run`.

The native handler — `kernel/controlplane/server_handlers_plan.go` — is gone.

## The first typed operation that really streams

The panel, the Conductor, the research harness and the plan authoring are
typed LIVE operations that never stream: they declare kernel event frames, as
the shared adapter requires, but their emitters are never called. The plan
execution is the first one that **actually streams**: the caller sees
`plan.started` and every `node.*` event in order, and the terminal answer
carries the plan id and the node outputs.

It keeps the native `StreamEvents` mode, so its context is not tied to the
client connection — the plan keeps running when the client goes away, as
before. The cancel-on-disconnect behaviour belongs to `run`, which still has
its native handler.

## The wire shape is unchanged

The legacy answer was a `map[string]any` with two keys — the plan id and the
node outputs — and the app-owned path re-encodes the typed output through the
same object envelope, so the member order is the same as before. The node
outputs are a map, and an empty one answers as `{}`, not `null`.

## Preserved behavior

- **The plan argument:** required, and a JSON string. A missing argument is
  refused with `args.plan_json required (JSON string)`, an argument that is
  present but is not a string — including an explicit `null` — with
  `args.plan_json must be a JSON string`.
- **A broken plan:** a plan that is not JSON is refused with the
  `plan_json parse:` prefix and the decoder's message.
- **An empty plan:** a plan without nodes is refused with `plan has no
  nodes`.
- **An unknown node:** refused with the node's id and kind, as
  `node "a": unknown kind "nope" (want loop|gate)`.
- **The plan id:** named after the request that submitted it, exactly as
  before: `plan-` plus the request id with a leading `q` removed, and an
  empty name when the request has no id.
- **The events:** the plan's own frames, subscribed before the plan starts so
  `plan.started` is never missed, and the in-flight frames are drained before
  the terminal answer.
- **The failure:** a plan that fails is the operation's failure.

## The request id moved to the app layer

The plan names itself after the request that submitted it, so the operation
needs the request id. The context key that carried it lived in the control
plane; it now lives in `kernel/app` (`app.WithRequestID` and `app.RequestID`),
and the control plane's own key is gone. The file operations, which already
read it as the governed tool call's id, read it from the same place.

## The archcheck ratchet moved again

`kernel/controlplane/planner.go` was the last control-plane file that imported
`kernel/planner`; `kernel/controlplane/server_handlers_plan.go` was the last
one that imported `kernel/scheduler`. With both gone, the allowlist tightens
again: the gate now reports 129 allowlisted import violations and 13
allowlisted call sites, down from 130 and 13.

## Runnable evidence

Permanent tests cover:

- the refused plan: the missing argument, the wrong type, the explicit null;
- the broken plan and the empty plan;
- the unknown node kind;
- the streamed events, the subscription pattern, the plan id and the terminal
  answer with the node outputs;
- the plan id derivation from the request id;
- the plan failure;
- the spec: the name, the route, the primary-only scope, the streaming mode
  and the declared event frames.

At the control plane, the plan runs on the primary kernel, streams its events
as event frames, answers with the plan id and the node outputs, refuses the
missing plan, and is registered as an app-owned, primary-only, streaming
operation.

Twelve independent mutations fail tests: the unrequired plan, the accepted
null, the dropped parse prefix, the dropped empty-plan check, the accepted
unknown kind, the unemitted events, the dropped node outputs, the plan id that
is not the request id, the plan id that loses the request prefix, the wrong
subscription pattern, the swallowed plan failure and the nil node outputs.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (268 packages, 129 allowlisted import violations and 13
allowlisted call sites) and `tools/structure-md`.

## Not done here

The cloned-kernel harness of the earlier slices — the pre-slice handler and the
registered operation on two kernels, compared step by step — is not part of
this slice, as in W2.44b, W2.44c and W2.44d. The `run` command, the last
native handler of the streaming family, is not part of it either.
