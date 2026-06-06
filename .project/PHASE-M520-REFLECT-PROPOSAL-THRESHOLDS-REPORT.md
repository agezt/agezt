# M520 — Mutation testing reflect: pin the proposal-rule inclusive thresholds

## Context
Thirtieth package in the mutation pass: `kernel/reflect` (the meta-cognition loop —
folds the journal into Observations, applies world-model decay, derives advisory
Proposals). Run with `GOMAXPROCS=3` (CPU-capped). go-mutesting score 0.590, 36 survivors;
working tree restored clean.

## The genuine gap (closed)
`proposals` derives three advisory recalibrations, each gated by an inclusive threshold:

```
if o.BriefsSent >= briefVol { … pulse … }
if o.ApprovalsDenied-o.ApprovalsGranted >= denyExcess { … autonomy … }
if o.TasksFailed > 0 && o.TasksFailed*2 >= o.TasksStarted && o.TasksStarted > 0 { … tasks … }
```

`TestProposalsFireAtThresholds` fires all three but only **clear of** the edge: the
autonomy rule with a denied-granted excess of 3 against `DenyExcess 2`, and the tasks rule
at 75% failure (3 of 4). `TestProposalsSilentBelowThreshold` is far below. So only the
brief rule's `>=` is pinned (its test happens to use `briefs == BriefVolume`); the autonomy
and tasks rules' `>=` survived `→ >` (confirmed by hand-applied negative control against
the existing suite):
- `ApprovalsDenied-ApprovalsGranted >= denyExcess → >` — survived (a deny excess *exactly*
  at the configured limit would stop proposing a review).
- `TasksFailed*2 >= TasksStarted → >` — survived (a run batch at *exactly* 50% failure —
  the intended trigger point — would no longer be flagged).

## Fix
Added `TestProposals_ExactThresholds` (calls `e.proposals` directly with crafted
`Observations`): the autonomy rule fires at `denied-granted == denyExcess` and is silent at
`denyExcess-1`; the tasks rule fires at exactly 50% (`failed*2 == started`) and is silent
just under.

## Negative control (manual, CPU-capped)
`denied-granted >= denyExcess → >` and `failed*2 >= started → >` each FAIL under the new
test. Restored byte-for-byte (`git diff --ignore-all-space` on reflect.go empty); passes
again.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — thirty packages (M490–M520)
…planner, ulid, artifact, reflect — plus the controlplane primary-token auth gate verified
solid. Same recurring closeable class: a multi-rule threshold tested only with inputs that
sit clear of the inclusive `>=` edge.
