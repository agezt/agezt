# W2.52 typed run fold

The run **fold** now lives in the application layer, in
`kernel/app/runs/collect.go` beside the run service that reads it. The
control plane keeps only the host's data access.

Two native files shrink to almost nothing:
`kernel/controlplane/runs.go` (the fold and the row type are gone) and
`kernel/controlplane/runs_extract.go` (the payload extractors are gone).

A third file leaves with them: `kernel/journal/runs.go` held a second copy of
the folded row (`RunEntry` and `RunEntryStatus`) that no reader had used
since the control plane grew its own fold. With the application owning the
single folded row, the journal's copy is dead weight and is gone.

## What moved

The typed `runs` operations were already in the application layer
(`kernel/app/runs`): they own the list, the stats, the show and the terminal
answer. What they did not own was the **fold** behind the rows — the single
journal walk that turns a run's events into a row.

`kernel/app/runs/collect.go` owns it now:

```go
func Collect(journal func(func(*event.Event) error) error) (map[string]Run, error)
func RunRows(rows map[string]Run) []approster.RunRow
```

with the payload extractors that feed it: the intent, the agent, the tool,
the iters, the cost, the model, the answer excerpt, the spawn link and the
failure reason.

The native `runEntry` row type is gone: the fold produces the application's
own `Run` row, so the native code no longer kept a second copy of the same
fields and converted them on the way out.

## The three consumers

The fold had three readers, and all three now ask the application:

- the **run service** (`runReads`) — the fold feeds it directly;
- the **roster status** — `RunRows` converts the folded runs into the status
  collection's run rows (the conversion lives in `kernel/app/runs`, which
  already depends on `kernel/app/roster`);
- the **schedule firings** and the **tenant activity** — the host adapters
  read the folded runs' fields into their own app row types.

## The semantics are unchanged

The fold is moved verbatim:

- one forward walk, no early stop: the limit is applied post-sort, because
  "last N runs" requires knowing all runs first (the journal is ordered by
  seq, not by run start time, and one run's events interleave with others
  under concurrency);
- `task.received` starts the row (the intent, the agent, the start time, the
  start sequence and the `starting` phase);
- `task.completed` completes it (the iters and the answer excerpt);
- `task.failed` fails it (the classified reason and the failure time);
- `task.abandoned` abandons it;
- `subagent.spawned` links the **child** to its parent, creating the child's
  row if the spawn is seen first;
- `budget.consumed` folds the spend into the **existing** row only — a
  budget event for an unknown correlation must not conjure a phantom run that
  would then count as "running" in the stats — and records the model
  first-wins, because a later provider fallback does not change the model the
  run was routed to;
- the activity phase is last-wins forward (`thinking`, `using tool`,
  `observing tool`, `continuing`) and, like the spend, only for a run that
  already exists.

The answer excerpt is still a single line: the whitespace runs collapse to
single spaces, the result is trimmed and truncated to 80 runes with an
ellipsis.

## The wire shape is unchanged

The run rows, their fields, the list, the stats and the show are untouched:
the application already owned the presentation, and the fold still answers
the same questions with the same rules. The existing end-to-end suite — which
drives real runs over a real kernel and reads the rows back — passes
unchanged.

## Runnable evidence

Permanent tests cover, at the application layer:

- the received event: the intent, the agent, the start time, the start
  sequence, the phase and that the run is running;
- the completed event: the completion, the iters and the answer excerpt;
- a completion without a reception: the half that is known is still recorded;
- the failed event: the failure, the classified reason and the status;
- the abandoned event: the abandonment and the status;
- the spawn: the child's parent link, and a spawn without a parent that links
  nothing;
- the spend: the costs sum, and the model is first-wins;
- a budget event for an unknown correlation and one without a correlation:
  no phantom run;
- the phase: the last phase wins, the tool in flight is recorded, and each of
  the four phases (`thinking`, `using tool`, `observing tool`, `continuing`);
- a lifecycle event for an unknown correlation: no phantom run;
- the extractors: the empty payload, the malformed payload and the extracted
  value for each of them;
- the answer excerpt: the collapsed line, the blank answer and the bounded
  excerpt;
- the run rows: the conversion into the roster status' row type.

At the control plane, the existing runs, schedule, tenant and roster suites
pass unchanged.

The mutations fail tests: the dropped intent, the dropped agent, the dropped
start time, the dropped start sequence, the dropped starting phase, the
dropped completion, the dropped iters, the dropped answer preview, the
dropped failure, the dropped reason, the dropped abandonment, the dropped
parent link, the parent link not required, the dropped cost, the last-wins
model, the conjured run on a spend, the dropped thinking phase, the dropped
tool phase, the dropped observing phase, the dropped continuing phase, the
dropped tool name, the conjured run on a lifecycle event, the uncollapsed
answer, the unbounded answer, the dropped iters count, the dropped cost, the
dropped parent link in the row conversion and the dropped model in the row
conversion. 28 mutations, 28 caught.

The run was honest about the order: the first pass caught 21 of 26 and left
five open — the dropped start sequence, the parent link not required, the
dropped thinking phase and the dropped tool phase escaped because the tests
did not assert them, and the dropped tool name was skipped because its
anchor did not match the source. The tests were strengthened (the start
sequence, the spawn without a parent, and each of the four phases) and the
run was repeated: 7 mutations, 7 caught. No equivalent mutation is recorded
for this slice.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (268 packages, 129 allowlisted import violations and 13
allowlisted call sites) and `tools/structure-md`.

## Not done here

The run views' cursor handling already lives in `kernel/journal` (shared by
every journal-backed list endpoint). What remains native in the run domain
is the host's own binding — the journal read — and the shared cursor code.

The cloned-kernel harness of the earlier slices — the pre-slice handler and the
registered operation on two kernels, compared step by step — is not part of
this slice, as in W2.44b through W2.51. The runs' own end-to-end suite is the
parity proof here: it drives real runs over a real kernel and reads the rows
back.
