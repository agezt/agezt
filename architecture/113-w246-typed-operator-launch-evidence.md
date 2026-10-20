# W2.46 typed operator wake and repair launches

The operator **launches** now live in the application layer, in
`kernel/app/roster/launch.go` beside the services they serve. The control
plane keeps only the host's data access.

`kernel/controlplane/roster_repair.go` (64 lines) is gone, and the two native
launch bodies — `runAgentWake` and `runAgentRepair` — leave the control
plane. `kernel/app/roster/launch.go` (140 lines) is new.

## What moved

The typed `agent_wake` and `agent_repair` operations were already in the
application layer (`kernel/app/roster/wake.go` and `repair.go`): they validate
the agent, journal the request and hand the work to a `launch` port. What the
port's implementation did was still native — and that implementation is where
the interesting decisions live:

- **does the run retry?** a profile asks for retries only when it has a retry
  policy with more than one attempt;
- **what cost ceiling does the run carry?** the agent's own ceiling, and no
  ceiling when the profile has none;
- **what does the terminal event say?** the phase, the agent, the reason, the
  answer (truncated) or the error, the autonomy runbook and the whole incident
  lineage;
- **what happens when a governed repair panics?** the panic firewall (WF-001):
  the panic is caught, reported and journaled as a failed repair instead of
  taking the daemon down.

`kernel/app/roster/launch.go` owns all four:

```go
func LaunchOperatorWake(ports WakeLaunchPorts, p core.Profile, corr, intent, reason string, lineage IncidentLineage)
func LaunchOperatorRepair(ports RepairLaunchPorts, p core.Profile, corr, reason string, lineage IncidentLineage)
```

## The application owns the decision, the host owns the data

The two launchers take ports. The host binds them to its own capabilities:

| Port | Host binding |
|---|---|
| `WakeLaunchPorts.Run` | the kernel's `RunWith` / `RunWithRetry` under the decorated run context |
| `WakeLaunchPorts.Publish` | `publishOperatorAction` — the control plane's publisher to the kernel's bus |
| `RepairLaunchPorts.Repair` | `overseertool.NewKernelSource(...).RepairAgent` — the governed repair |
| `RepairLaunchPorts.Publish` | the same publisher |
| `RepairLaunchPorts.OnPanic` | the stderr report of a caught panic |

The retry policy and the cost ceiling reach the host as **decisions**, not as
context: the application resolves them from the profile and hands them to the
host, which executes the run. That keeps the decisions observable (a test can
assert what the host was asked to do) and keeps the runtime's context
mechanics — the agent profile, the operator wake context, the cost ceiling —
in the host, where the kernel lives.

## What stayed native, and why

The control plane keeps `roster_escalation.go`: the operator incident lineage
(`operatorWakeLineage`), the publisher (`publishOperatorAction`) and the two
thin adapters (`launchOperatorWake`, `launchOperatorRepair`) that bind the
host's run and repair capabilities to the application's launchers. Those
adapters are the host's data access, not its decisions.

The repair's governed source still comes from `plugins/tools/overseertool` —
the layering violation the architecture records (the kernel must not import
plugins) is unchanged by this move: the violation is now in one adapter
instead of in the launch body.

## The dead code the move left behind

The native launches were the control plane's last readers of its own
`truncate` helper, so `truncate` leaves `kernel/controlplane/roster_helpers.go`
(the application layer has its own, with the same semantics). The `plInt`
payload helper went with the repair file in W2.45b.

## The wire shape is unchanged

The `agent_wake` and `agent_repair` events keep their subjects, their phases,
their fields and their order: the same `agent.wake` / `agent.repair` subjects,
the same `requested` → `completed`/`failed` arc, the same payload keys
(`phase`, `agent`, `reason`, `answer`/`error`, `autonomy_runbook`, the three
incident ids, and for the repair the applied list, the routing task type and
the two routing model chains).

The existing end-to-end suite — which drives the real operation over a real
kernel and reads the journal back — passes unchanged.

## Runnable evidence

Permanent tests cover, at the application layer:

- the completed wake: the subject, the correlation, the phase, the agent, the
  reason, the answer, the incident lineage and the canonical autonomy runbook;
- the failed wake: the phase and the error, and that it has no answer;
- the retry decision: a profile with a retry policy retries, a profile with a
  one-attempt policy runs plain, and a profile without a policy runs plain;
- the cost ceiling: the profile's ceiling reaches the host, and a profile
  without one runs without a ceiling;
- the truncated answer;
- the completed repair: the subject, the agent, the reason, the applied list,
  the routing task type, the routing model chains, the answer and the lineage;
- the failed repair: the phase and the error;
- the panic firewall: the caught panic is reported to the host, the failed
  event is published with the panic message, the lineage survives and exactly
  one event is published;
- the truncated repair answer.

At the control plane, the existing suite passes unchanged — including the
native parity tests that drive the operator wake and repair over the wire
(`TestAgentWakeNativeLaunchCarriesLineage`,
`TestAgentRepairNativeLaunchCarriesLineage`) and the firewall test
(`TestAgentRepairRunContainsPanics`), which now proves that the native adapter
keeps the application's firewall.

The mutations fail tests: the single attempt that retries, the run that always
retries, the dropped cost ceiling, the completed wake journaled as failed, the
failed wake journaled as completed, the dropped error, the dropped answer, the
untruncated answer, the dropped runbook, the dropped incident lineage, the
wrong subject, the completed repair journaled as failed, the dropped applied
list, the dropped routing chain, the dropped task type, the untruncated repair
answer, the dropped agent slug, the dropped reason, the missing panic
firewall, the unreported panic and the panic event's wrong subject. 24
mutations, 24 caught.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (268 packages, 129 allowlisted import violations and 13
allowlisted call sites) and `tools/structure-md`.

## Not done here

The operator resolve path (`latestOperatorForceGeneration`,
`latestExhaustedRoutingChain` and the incident-lineage matching behind it)
still lives in the control plane, and so do the roster teardown and the
impact cascade. They are the next slices of the same domain.

The cloned-kernel harness of the earlier slices — the pre-slice handler and the
registered operation on two kernels, compared step by step — is not part of
this slice, as in W2.44b through W2.45b. The roster's own end-to-end suite is
the parity proof here: it drives the real operation over a real kernel and
reads the journal back.
