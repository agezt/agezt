# W2.47 typed operator resolve journal reads

The operator resolution's **journal reads** now live in the application layer,
in `kernel/app/roster/resolve_journal.go` beside the resolve service that
reads them. The control plane keeps only the host's data access.

Two native files are gone:

- `kernel/controlplane/roster_wake_helpers.go` (107 lines)
- `kernel/controlplane/roster_helpers.go` (the payload accessors — the last
  readers were the helpers above)

## What moved

The typed `agent_resolve` operation was already in the application layer
(`kernel/app/roster/resolve.go`): it validates the agent, decides the
resolution, applies it and journals the outcome. Two of its ports were still
answered by native code that walked the journal itself:

- **`ExhaustedChain`** — the latest model chain the doctor found exhausted for
  an agent, a task type and an incident lineage. The operator's force-chain
  resolution must not repeat it;
- **`ForceGeneration`** — the latest applied force-chain generation, which the
  operator's next resolution supersedes.

Both now live in the application layer, over a journal port:

```go
type ResolveJournal func(func(*event.Event) error) error

func LatestOperatorForceGeneration(journal ResolveJournal, slug, taskType string) int
func LatestExhaustedRoutingChain(journal ResolveJournal, slug string, lineage IncidentLineage, taskType string) []string
```

The incident-lineage matching moves with them: which payload belongs to the
incident the operator is resolving (any of the three lineage ids among the
payload's incident ids, case-insensitively).

## The lineage type is now the application's

The native code carried its own `operatorWakeLineage` — a private transport
type with three unexported fields, converted at every call site. The
application layer already had `IncidentLineage` (the operator wake and repair
services use it), so the native type is gone: the launch adapters and the
resolve ports speak `approster.IncidentLineage` end to end, and there is one
conversion fewer in the codebase.

The `hasAny` helper (does this lineage name an incident at all?) is now a
method on the application's lineage type, where the matching that uses it
lives.

## The semantics are unchanged

The reads are moved verbatim:

- the force generation accepts only `doctor.auto_repair` and `agent.resolve`
  info events, only a `force_chain` resolution, only the `resolution_applied`
  and `completed` phases and only the named task type, and takes the highest
  `routing_force_generation`;
- the exhausted chain accepts only `doctor.auto_repair` info events with the
  `routing_force_exhausted_detected` phase, the named agent (case-insensitively)
  and the named task type, matches the incident lineage, and takes the latest
  event by sequence number.

## The dead code the move left behind

`kernel/controlplane/roster_helpers.go` held `plString`, `plStrings` and
`intNumber` — the payload accessors. The two moved helpers were their last
readers, so the file is gone, and with it the control plane's last dependency
on the payload-accessor pattern (the application layer has its own, with the
same semantics).

## The wire shape is unchanged

The resolve operation's answers, its journal arc and its refusals are
untouched: the application already owned the decision, and the two ports
answer the same questions with the same rules. The existing end-to-end suite
— which drives the real operation over a real kernel — passes unchanged.

## Runnable evidence

Permanent tests cover, at the application layer:

- the force generation: the latest applied one wins across two subjects
  (`doctor.auto_repair` and `agent.resolve`);
- the force generation's filters: another agent, another task type, another
  resolution kind, a queued phase and an unrelated phase are all ignored;
- the force generation's empty inputs: no journal, no agent and no task type
  all answer zero — with a journal full of matching events, so the guards
  cannot pass by accident;
- the exhausted chain: the latest event for the incident wins, and the lineage
  matches on the incident id, the root incident id and the parent incident id,
  case-insensitively;
- the exhausted chain's filters: another incident, another agent, another
  phase, another task type and a lineage with no incident at all are ignored;
- the exhausted chain's empty inputs;
- the lineage's `hasAny`: each of the three ids counts, blanks do not.

At the control plane, the existing roster suite — which drives the real
operation over a real kernel and reads the journal back — passes unchanged.

The mutations fail tests: the doctor subject ignored, the resolution kind
ignored, the phase ignored, the task type ignored, the latest generation not
winning, the phase ignored in the chain read, the task type ignored in the
chain read, the agent ignored in the chain read, the lineage ignored, the
oldest event winning, the model chain dropped, the blank incident counting, the
case-sensitive lineage match, the parent incident ignored and `hasAny`
ignoring the parent incident. 18 mutations: 15 caught, and three recorded
equivalents — the empty-agent guard, the outer empty-lineage guard and the
inner empty-lineage guard are redundant with the filters below them (an empty
slug matches no event, an empty lineage id matches no payload), so removing
them changes no behavior.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (268 packages, 129 allowlisted import violations and 13
allowlisted call sites) and `tools/structure-md`.

## Not done here

The roster domain's remaining native bodies are the teardown
(`roster_teardown.go`, `roster_teardown_state.go`), the impact cascade
(`roster_cascade.go`) and the lifecycle helpers. They are the next slices of
the same domain.

The cloned-kernel harness of the earlier slices — the pre-slice handler and the
registered operation on two kernels, compared step by step — is not part of
this slice, as in W2.44b through W2.46. The roster's own end-to-end suite is
the parity proof here: it drives the real operation over a real kernel and
reads the journal back.
