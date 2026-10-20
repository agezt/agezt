# W2.44f typed run

The `run` command now runs on the shared dispatcher, in `kernel/app/runs` beside
the run reads and the plan execution. The operator, the CLI and the Web UI
reach it through `run`.

The native handler — `kernel/controlplane/server_handle_run.go` (596 lines) —
is gone, together with the remote-agezt helpers that lived beside it
(`kernel/controlplane/server_handle_run_remote.go`).

This is the last native command of the streaming family. `whoami` stays native
by design: it echoes the transport principal and must not route the tenant it
names.

## One typed output for two answers

The legacy `run` answered with two different shapes: a plain run answered with
`answer`, `correlation_id` and — when the journal fold knew the run — `agent`,
`iters`, `spent_mc` and `model`; a dry run answered with the resolved plan
(`model`, `model_source`, `tools`, `warnings`, …). A single typed output
reproduces both, because every optional field is a pointer: a plain run always
carries an answer (even an empty one) while a dry run never does, and the plan
always carries a model (even an empty one) while a plain run only reports the
model the run actually billed.

`TestBuildRunPlan_WireShape` pins the plan's key set exactly — the plan's keys
are never absent and the run's answer keys never leak into it — and
`TestRunExecuteAnswersWithTheRun` pins the plain run's keys.

## The wire shape is unchanged

The legacy answers were `map[string]any`; the app-owned path re-encodes the
typed output through the same object envelope, so the member order is the same
as before. The strict argument contract is preserved verbatim: a mistyped flag
is a usage error, never a silent default.

The readers moved with the command: `argBool`, `argInt64`, `argStringList` and
`parseCapList` now live in `kernel/app/runs/run_args.go` and read raw JSON
arguments, and the control plane's copies are gone. `argTruthy` moved with them.

## Preserved behavior

- **The intent:** required. A missing, blank or non-string intent is refused
  with `args.intent required`.
- **The strict argument contract:** `args.<name> must be a string` / `must be a
  number` / `must be a boolean` / `must be an array`, and
  `args.<name>[i] must be a string`. A JSON `null` is a type error, not an
  absent argument — the dry run's `null` plan was the precedent, and it is
  preserved here.
- **The dangerous mistypes:** a `dry_run` sent as the string `"true"` is a
  usage error, not a silent `false` that would EXECUTE the run the operator
  meant to preview; a `tools` sent as a bare string is a usage error, not a
  silent empty list that would scope the run to no tools.
- **The agent binding:** an unknown, retired, paused or managed sub-agent is
  refused; the agent's whole profile binds the run (soul, model, cost ceiling,
  tool allow/deny, trust ceiling, memory scope), and its fallbacks head the
  model chain behind the resolved primary.
- **The vision gate:** the images are admitted before the run, and the admitted
  intent and images are the ones the run uses.
- **The execution profile:** the same profiles (ssh, k8s, modal, daytona,
  remote-agezt, warden) with the same refusals for an inactive backend, the
  same tool requirement and the same routable-profile list. An agent's own
  execution profile applies to a direct run too.
- **The dry run:** the plan's key set, its sources (`daemon default` /
  `per-run (--model)`, `per-run (--system)`, the timeout and cost-cap
  formatting) and its warnings (unknown model, a model that does not advertise
  tool-use while tools are enabled, a small context window, strict pricing
  refusing an unpriced model, a cap that cannot bind) are unchanged.
- **The remote-agezt delegation:** the same lifecycle events, the same peer
  metadata footer and the same peer-event mirroring — the mirroring itself
  stays in the control plane, because the host owns the peer registry, and
  reaches the run through a port.
- **The cancel-on-disconnect:** the run still cancels itself when the operator's
  client goes away. The host now supplies the connection's disconnect signal
  through a port; the read deadline is cleared first, so a long run is never
  mistaken for a disconnect.
- **The terminal answer:** the run's own answer and correlation, enriched with
  the run's iters, spend and model when the journal fold knows the run — and
  bare, never wrong, when it does not.

## The archcheck ratchet

`kernel/controlplane/execution_inventory.go` was the last control-plane file
that built the execution-profile inventory; the builder and `toolNames` moved
to `kernel/app/execprofile/inventory.go`, shared by the profile reads and the
run's unroutable-profile error. The gate now reports 129 allowlisted import
violations and 13 allowlisted call sites.

## Runnable evidence

Permanent tests cover:

- the operation's spec: the name, the streaming mode, the audited scope, the
  tenant routing, the lenient input and the declared output and emission
  schemas;
- the refused arguments: every mistyped flag, the missing intent, the unknown,
  retired, paused and managed agent, the bad timeout, the peer without the
  remote profile;
- the agent binding: the agent's model, the explicit override, the agent's
  execution profile;
- the dry run: the plan's key set, the tool modes, the sources, the cost cap,
  the warnings, the strict-pricing warning, the model override, the tool scope;
- the plain run: the answer, the correlation, the enrichment and the bare
  answer when the fold is unknown;
- the readers: the string, number, boolean, list, truthy and capability
  contracts;
- the pricing probe: a priced model binds a cap, a known-free model does not.

At the control plane, the run is registered as an app-owned, audited,
tenant-routed, streaming operation; the dry run answers with the plan and
streams no event; a plain run answers with the run's answer and correlation;
the missing intent and the two dangerous mistypes are refused.

The mutations fail tests: the unrequired intent, the accepted managed
sub-agent, the ignored agent model, the accepted non-positive timeout, the
accepted peer without the remote profile, the ignored model override, the
dropped model context, the accepted mistyped boolean, the kept blank list
entry, the ignored tool scope, the ignored agent execution profile, the
mislabelled tool mode, the dropped capability fields, the dropped
strict-pricing warning, the dropped dropped-tool list, the absent tool list, the
broken pricing probe, the ignored dry-run branch and the dropped run
accounting.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (268 packages, 129 allowlisted import violations and 13
allowlisted call sites) and `tools/structure-md`.

## Not done here

The cloned-kernel harness of the earlier slices — the pre-slice handler and the
registered operation on two kernels, compared step by step — is not part of this
slice, as in W2.44b, W2.44c, W2.44d and W2.44e. The run's own end-to-end
behavior is covered by the control plane's existing run suite (the model
override, the system override, the agent profile, the vision gate, the per-run
cost cap, the resume eligibility), which now exercises the typed operation
instead of the native handler.

The remote-agezt branch is covered by the control plane's existing remote
mirror tests, not by a new end-to-end peer fixture: the delegation needs a
registered `remote_run` tool and a live peer, which this slice does not build.
