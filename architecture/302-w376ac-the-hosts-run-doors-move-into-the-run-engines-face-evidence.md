# W3.76ac — the host's run doors move into the run engine's face

**Slice:** W3.76ac (the thirty-seventh of the runtime's dissolution)
**Base:** `01fba5d4` (the W3.76ab merge)
**Date:** 2026-11-04

---

## 1. The family

The **run engine** is the runs module's: `kernel/modules/runs/runner` holds the
run's body — the run itself, the run under a retry policy, the run under the
assurance loop and the close of an agent's lifecycle.

W3.76ab took the engine's **wrappers** away (the six copies the daemon kept over
the module's own operations) and left the daemon's **doors**: `Run`, `RunWith`,
`RunWithRetry`, `RunAssured` and `CompleteAgentLifecycle` — the five verbs the
callers above the daemon used to *start* and *close* a run.

Those five were the same second spelling, one slice later. The engine already
answered in exactly those names:

| the daemon's door | the module's operation |
|---|---|
| `Run` | `Runner.RunWith` (+ the host's own correlation id) |
| `RunWith` | `Runner.RunWith` |
| `RunWithRetry` | `Runner.RunWithRetry` |
| `RunAssured` | `Runner.RunAssured` |
| `CompleteAgentLifecycle` | `Runner.CompleteAgentLifecycle` |

`Run` had **no production caller at all** — the tests were its only callers —
and the other four were forwarding copies.

## 2. The face

The daemon now hands over the module's **face** instead:

```go
func (k *Kernel) RunnerService() api.RunnerService { return k.runner }
```

The port lives in the runs module's own api package
(`kernel/modules/runs/api/runner.go`) and carries the engine's four operations
in the engine's own names:

```go
type RunnerService interface {
	RunWith(ctx context.Context, corr, intent string) (string, error)
	RunWithRetry(ctx context.Context, corr, intent string, pol agentapi.RetryPolicy) (string, error)
	RunAssured(ctx context.Context, corr, intent string, maxAttempts int) (string, assureapi.Result, error)
	CompleteAgentLifecycle(ctx context.Context, corr string)
}
```

The engine says it **is** that port (`var _ api.RunnerService = (*Runner)(nil)`),
so the face and the body cannot drift apart.

## 3. What the host keeps

The daemon keeps what is the **host's own**:

- the **correlation id** a run carries (`NewCorrelation`) — the host's identity
  door (W3.76aa's rule: the run's *name* is the host's business; the
  adaptörler subscribe by subject, the engine is asked to run under an id the
  caller already holds);
- the run's own **setup and cleanup**, the resume handshake, the retry helpers.

Every caller above the daemon now asks the **module**:

- `kernel/app/runs/ports.go` — the launch, the retry launch and the assured loop
  ask `k.RunnerService().…`;
- `kernel/runtime/accessors_misc.go` — the loop runner asks the face;
- `kernel/runtime/subagent_doors.go` — the child lifecycle's close asks the face;
- `kernel/modules/agents/selfrepair/selfrepair_wake_agent.go` — the wake asks
  the face;
- `cmd/agezt` — the cadence, the channels, the standing runner and the OpenAI
  adaptör ask the face.

## 4. The measurement

| what | before | after |
|---|---|---|
| the daemon's public verbs | 129 | **125** |
| the daemon's production lines | 5,622 | **5,579** |
| the daemon's production files | 55 | **55** |
| the app packages that import `kernel/runtime` | 40 | **40** |
| the runs module's api package | 2 files / 86 lines | **3 files / 122 lines** |
| the run engine's package | 917 | **922** |
| the run engine's host port (`KernelAPI`) | 37 entries | **37 entries** |

**Five doors off the daemon's surface** (129 → 125): five went, one came — the
face. The daemon's **lines went down** by 43: the doors' bodies and the account
of them left together.

The run engine's **host port did not move** (37 → 37): W3.76ab already took the
engine's own operations out of it, and what stayed is what the **host** owns —
the run's setup and cleanup, the prompt, the loop configuration, the resume
handshake and the host's own state. The engine's package grew by five lines
(the port's evidence and the comment that says where the calls go).

## 5. The guard lists

The control plane's guard reads the kernel's own method set, so a new face is
a new verb that a family must own. `RunnerService` joined the **runs** family,
and `RunAssured` — the daemon's own copy of the assurance loop — left it: the
loop is the module's now, and the daemon answers `RunnerService`.

The runs family keeps `AdmitImages` (the vision gate), `SubjectForRun` (the
run's own subject), `MaxDuration` (the daemon's run ceiling), `LoopRunner` and
`RunPlan` (the plan runner's machinery).

## 6. The ratchets — and the probe

**The daemon's side** (`kernel/runtime/run_door_ratchet_test.go`, new): the
daemon holds **no** verb per run engine operation, it **keeps** the face, its
production files do not declare a door, and its runner file says where the
engine's operations live.

**The module's side** (`kernel/modules/runs/runner/runner_face_ratchet_test.go`,
new): the port carries the engine's four operations **and nothing else**, and
the engine carries the port's evidence.

**The application's side** (`kernel/app/runs/binding_ratchet_test.go`, new): the
runs application asks the **face** for a run, not the daemon — and the check
looks at the **shape**, because the verb names are the same on both sides.

The daemon's own surface ceiling followed the floor again: **129 → 125** (220
→ 201 → 194 → 179 → 173 → 166 → 164 → 156 → 152 → 148 → 143 → 135 → 129 → 125),
and the five retired names joined the retired-verb list.

**The probe** put the daemon's `RunWith` back, had the application call the
daemon's door directly, put an operation the engine does not own into the port
and took the port's evidence away: the retired-verb ratchet, the surface
ceiling (126 > 125), the daemon's two run-door ratchets, the application's
binding ratchet and the module's two face ratchets all went red at once —
**seven ratchets in one shot**. The probe was reverted and the suites went
green again.

## 7. The coverage moved with the subject

The daemon's `Run` door is gone, and its **tests** were the only callers. The
two test packages that used it — `runtime_test` and `controlplane_test` — now
mint the correlation id at the host's own door and run through the face
(`runForTest`), and every other test asks the face directly. The guarantee did
not change: the correlation id is still the **host's**, and the run is still the
**module's**.

## 8. Verification

- the compiler (`go build ./...`, `go vet ./...`) and `gofmt`;
- the daemon's tests, the run engine's tests, the applications' tests, the
  control plane's tests, the composition root's tests;
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 9. What is left

The run engine's operations are the module's, the module's **face** is what the
daemon hands over, and the daemon answers for none of them.

The daemon holds **125** public verbs and **5,579** production lines; **forty**
application packages still name it; the run engine's host port still carries
**37** entries.

What is left in the daemon's run family is the run's **own machinery** rather
than its verbs: `runtime_lifecycle.go`'s fourteen living verbs, the plan runner,
the resume handshake and the prompt doors — the surface a run touches on its way
through the host.

The **standing package** itself still lives at `kernel/standing`, and its
eventual home is the **triggers module** (`kernel/modules/triggers`) — the
architecture gate already places both `kernel/cadence` and `kernel/standing` in
that module, so the family's boundary is drawn; the module's directory does not
exist yet, and the move is a path move (four production importers), not a
lifecycle one.
