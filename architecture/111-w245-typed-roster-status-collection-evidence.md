# W2.45a typed roster status collection

The roster status **collection** now lives in the application layer, in
`kernel/app/roster` beside the status projection it feeds. The control plane
keeps only the host's data access.

`kernel/controlplane/roster_status.go` (371 lines) is gone.

## What moved

The `agent_list` status row is a projection of many sources: the journal, the
run fold, the reaper scan, the repair summaries, the board's escalations, the
schedules and the standing orders. The projection (`RenderStatus`, the 81
status fields) was already in the application layer; the **collection** — the
part that actually walks the journal and decides what each source means — was
still native.

This slice moves the journal-derived half of the collection:

- **the single journal walk** that dispatches every event into every
  accumulator: the live run statuses, the last activity per agent, the last
  autonomy runbook, the mailbox wakes, the policy denials and the routing and
  retry pressure;
- **the live wake context** and the **live event summary** (the phase, detail,
  tool and iteration a running run is at);
- **the routing-profile match** (whether a model-chain fallback belongs to the
  profile, by task type or by its chain);
- **the model chain** (`AgentModelChain`), which the run resolution now shares
  instead of keeping its own copy.

The wake views (schedules and standing orders), the escalation views (the
board's open help requests), the repair summaries and the reaper mapping stay
in the control plane for now: they read host stores, and they are the next
slice.

## The application owns what a row means

The host hands the collection folded run rows; the application decides which
runs are active and how a live status reads:

```go
type RunRow struct {
    CorrelationID, Intent, Model, Agent string
    SpentMicrocents, StartedUnixMS     int64
    ParentCorrelation                   string
    Completed, Failed, Abandoned        bool
}

func (r RunRow) Status() string // completed > failed > abandoned > running
```

The status precedence used to live in the control plane's `runEntryStatus`.
It now lives beside the rows it describes, and the control plane's copy is
gone — so the roster status, the run reads and the run operation cannot
disagree about what a terminal run is.

## The wire shape is unchanged

The status fields, their names, their types and their presence rules are
untouched: the application already owned the projection, and the collection
still fills the same snapshot. The existing end-to-end suite — the routing
pressure, the retry pressure, the policy denials, the autonomy runbooks, the
last activity summary, the mailbox wakes and the live run count — now exercises
the application layer's collection and passes unchanged.

## The single journal walk is preserved

The collection walks the journal **once**, dispatching every event into every
accumulator. That was already true before the move, and it matters: eleven
separate walks of a busy journal reliably tripped the control-plane
connection's read deadline, because each walk is a callback-driven O(n) pass
with a JSON unmarshal and map lookups per event. The move keeps the one walk
and the same "latest wins" semantics per accumulator.

The `cooldown` capture that the native file carried only to silence a lint is
gone — it was never read.

## Runnable evidence

Permanent tests cover, at the application layer:

- the live run status: the run count, the correlation, the intent, the model,
  the spend, the start time and the phase;
- the rows that are NOT live runs: a completed, a failed and an abandoned row,
  an unknown agent and an ad-hoc run with no agent;
- the last activity: the latest event wins, and two events in the same
  millisecond are settled by the walk order;
- the autonomy runbook: a schedule wake (its phase, source and correlation)
  and a delegation (its phase, source, delegator and the CHILD's correlation);
- the policy denial: the count, the tool, the reason, the capability, the hard
  flag and the timestamp — and that an ALLOWED decision is not a denial;
- the routing pressure: the count, the failed and next model, the reason, the
  cut and the scope — and that a profile matches by its task type;
- the retry pressure: the count, the reason, the next attempt and the maximum
  attempts;
- the mailbox wake: the message row and its correlation — and that a
  non-mailbox standing wake is not a mailbox wake;
- the live wake context: the wake source, the reason and the schedule;
- an empty roster, which has no status at all;
- the model chain: the primary in front, the blanks and the duplicates
  dropped.

The model chain, the routing-profile match and the status precedence are
covered directly.

At the control plane, the existing roster suite — which publishes the events
to a real kernel's bus and reads the status back over the wire — passes
unchanged, and now exercises the application layer's collection.

The mutations fail tests: the live run that is not running, the live run of an
unknown agent, the dropped run count, the wrong phase, the last activity that
is not the latest, the runbook without its phase, the runbook without its
source, the delegation that names the parent's correlation, the allowed policy
decision counted as a denial, the dropped denial count, the ignored routing
cut, the inverted routing scope, the dropped routing count, the routing
pressure that ignores the task type, the dropped retry count, the standing
wake counted as a mailbox wake and the model chain that keeps a duplicate.
One mutation is a recorded equivalent: dropping the "the run has an agent"
check is equivalent, because an empty agent slug is not a legal roster
profile, so the unknown-agent check already skips it.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (268 packages, 129 allowlisted import violations and 13
allowlisted call sites) and `tools/structure-md`.

## Not done here

The wake views, the escalation views, the repair summaries, the reaper
mapping and the snapshot assembly stay in the control plane: they read host
stores, and they are the next slice of the same domain.

The cloned-kernel harness of the earlier slices — the pre-slice handler and the
registered operation on two kernels, compared step by step — is not part of
this slice, as in W2.44b through W2.44f. The roster's own end-to-end suite is
the parity proof here: it publishes to a real journal and reads the status
back over the wire.
