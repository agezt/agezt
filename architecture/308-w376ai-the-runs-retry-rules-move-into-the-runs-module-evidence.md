# W3.76ai — the run's retry rules move into the runs module

**Slice:** W3.76ai (the forty-third of the runtime's dissolution)
**Base:** `ad47323f` (the W3.76ah merge)
**Date:** 2026-11-12

---

## 1. The family

A failed run is **classified**, the policy decides whether **another attempt**
is allowed, and the backoff says **how long** to wait:

```go
retry.Reason(err, halted)            // "halted" | "canceled" | "timeout" | "error"
retry.Retryable(reason, pol.RetryOn) // is another attempt allowed?
retry.Delay(pol, attempt)            // how long to wait before it
```

Those three rules moved into the runs module in **W3.76f**
(`kernel/modules/runs/retry`): the delegation engine needed them and could not
reach the daemon's unexported copies, so the package was created for them.

What stayed in the daemon was a **forwarding desk**:

```go
// kernel/runtime/runner_helpers.go
func retryReason(err error) string             { return retry.Reason(err, ErrHalted) }
func agentRetryable(reason string, retryOn []string) bool { return retry.Retryable(reason, retryOn) }
func retryDelay(pol agentapi.RetryPolicy, attempt int) time.Duration { return retry.Delay(pol, attempt) }
```

behind three public verbs (`RetryReason`, `AgentRetryable`, `RetryDelay`) —
and the **run engine**, which lives *inside* the runs module, asked the
**host** for all three through its `KernelAPI` port:

```go
reason := r.k.RetryReason(err)
if attempt >= max || !r.k.AgentRetryable(reason, pol.RetryOn) { ... }
delay := r.k.RetryDelay(pol, attempt)
```

**A port entry that is a round trip inside one module is a promise the host
never had to keep.** The module has the body; the module calls it.

This one is the purest case of the whole programme: the daemon's "copy" of the
rules was a copy of **nothing** — its file held three one-line calls into the
package next door, plus a test file that repeated the module's own tests.

## 2. What moved

Nothing moved **into** the module: the rules were already there. What moved
was the **engine's side of the seam** — the engine calls the module now:

```go
reason := retry.Reason(err, ErrHalted)
if attempt >= max || !retry.Retryable(reason, pol.RetryOn) { ... }
delay := retry.Delay(pol, attempt)
```

exactly the way the **delegation engine** has always called them
(`kernel/modules/runs/delegation/exec.go`).

The halt sentinel is the module's own: `state.ErrHalted` is the origin,
`runner.ErrHalted` is the engine's alias and `runtime.ErrHalted` is the
daemon's — one value, three names. So a halted run is classified exactly as
the daemon classifies it.

The daemon's `runner_helpers.go` is gone, and its test file with it. The F1
overflow regression (exponential growth must never wrap `time.Duration`'s
int64 backing to a negative value) moved to the module's own test — next to
the rule it pins.

## 3. The port

The run engine's host port lost **three** entries — `RetryReason`,
`AgentRetryable` and `RetryDelay` — and carries **24** now (it carried
**27**). What stayed is what the **host** owns.

## 4. The measurement

| what | before | after |
|---|---|---|
| the daemon's public verbs | 111 | **108** |
| the daemon's production lines | 5,277 | **5,246** |
| the daemon's production files | 54 | **53** |
| the app packages that import `kernel/runtime` | 40 | **40** |
| the run engine's host port (`KernelAPI`) | 27 entries | **24 entries** |
| the run engine's package | 1,025 | **1,040** |
| the retry module's package | 1 file / 91 lines | **1 file / 91 lines** |

**Three verbs off the daemon's surface and 31 lines with them**, and a file
gone with them. The retry package is **unchanged** — the rules were already
the module's; this slice removed the desk, not the rules. The run engine's
package grew by 15 lines (the calls and the comments that say where they go).

## 5. The ratchets — and the probe

**The daemon's side** (`kernel/runtime/retry_ratchet_test.go`, new): the
daemon holds **no** verb per retry rule, **no** forwarding wrapper
(`retryReason`, `agentRetryable`, `retryDelay`) and does not even **name** the
module's rules (`retry.Reason`, `retry.Retryable`, `retry.Delay`) — and the
run engine asks them.

**The module's side** (`kernel/modules/runs/runner/runner_retry_ratchet_test.go`,
new): the port carries **no** retry entry, the engine **calls** the module's
rules, and the module's own halt sentinel classifies as halted.

The daemon's own surface ceiling followed the floor again: **111 → 108** (220
→ 201 → 194 → 179 → 173 → 166 → 164 → 156 → 152 → 148 → 143 → 135 → 129 →
125 → 121 → 118 → 116 → 114 → 111 → 108), and the three retired names joined
the retired-verb list.

**The probe** put the world back before this slice — the daemon's three
verbs with their **real** bodies, the three entries back into the engine's
port and the engine asking the host again: the retired-verb ratchet, the
surface ceiling (111 > 108), the daemon's four retry ratchets and the
module's two retry ratchets all went red at once — **eight** ratchets in one
shot, and nothing else broke. The probe was reverted and the suites went
green again.

## 6. The guard

The three verbs were not in any control-plane family by name, so no guard
list changed: the daemon's surface is pinned by its own retired-verb list and
its ceiling, and the port's shape by the run engine's own ratchets.

## 7. Verification

- the compiler (`go build ./...`, `go vet ./...`) and `gofmt`;
- the daemon's tests, the run engine's tests, the retry module's tests, the
  applications' tests, the control plane's tests and the command's tests;
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 8. What is left

The daemon holds **108** public verbs and **5,246** production lines; **forty**
application packages still name it; the run engine's host port carries **24**
entries.

What is left in the daemon is the run's **remaining machinery**: the plan
runner's registration (`RunPlan`), the two doors the run engine still opens
for the host's own state (`ResolveRunModel`, `BuildLoopConfig`) and the
`FoldRunTools` port entry — the daemon's journal walk behind a port the
engine already holds.

The **standing package** itself still lives at `kernel/standing`, and its
eventual home is the **triggers module** (`kernel/modules/triggers`) — the
architecture gate already places both `kernel/cadence` and `kernel/standing`
in that module, so the family's boundary is drawn; the module's directory does
not exist yet, and the move is a path move (four production importers), not a
lifecycle one.
