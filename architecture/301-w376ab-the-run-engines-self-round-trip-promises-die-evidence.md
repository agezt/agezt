# W3.76ab — the run engine's self-round-trip promises die

**Slice:** W3.76ab (the thirty-sixth of the runtime's dissolution)
**Base:** `050ca522` (the W3.76aa merge)
**Date:** 2026-11-04

---

## 1. The family

The **run engine** is the runs module's: `kernel/modules/runs/runner` holds the
run's body — the loop, the post-run distill and forge, the shadow judgement,
the completion verification and the vision sidecar. The daemon **holds** the
engine (the host builds it while opening itself), and that is right: the host
needs the engine, and the host may name the module.

What the host must not do is **answer for the engine a second time**.

## 2. The round trip

The daemon carried six **wrappers** over the module's own body:

| the daemon's wrapper | the module's operation |
|---|---|
| `DescribeImages` | `Runner.DescribeImages` |
| `MaybeDistill` | `Runner.MaybeDistill` |
| `MaybeForge` | `Runner.MaybeForge` |
| `MaybeShadowEval` | `Runner.MaybeShadowEval` |
| `VerifyCompletion` | `Runner.VerifyCompletion` |
| `PublishHeuristicBypass` | `Runner.PublishHeuristicBypass` |

Each one was a forwarding copy: `k.MaybeDistill(…)` was `k.runner.MaybeDistill(…)`
and nothing else. **Nobody outside the daemon called them** — the daemon's own
image admission called one, and the daemon's own workboard wiring called
another.

What made them a *copy* rather than a door was the direction of the calls.
The run engine asked the **host** for them through its `KernelAPI` port:

```go
r.k.MaybeDistill(runCtx, corr, intent, answer)   // the engine asks the host
    → k.MaybeDistill(ctx, corr, intent, answer)  // the host answers
        → k.runner.MaybeDistill(ctx, corr, intent, answer)  // the host asks the engine back
```

**A port entry that is a round trip is a promise the host never had to keep.**
The module has the body; the module calls it.

## 3. What the port lost

Eight entries — the six above plus `RunWith`, `RunWithRetry` and
`CompleteAgentLifecycle`, which the engine reached the same way (through
`RunAssured` and the retry loop):

| the port entry | who asked the host for it | why it was a round trip |
|---|---|---|
| `MaybeDistill`, `MaybeForge`, `MaybeShadowEval` | the engine's own post-run step | the host answered with the engine |
| `PublishHeuristicBypass` | the engine's own publish step | the host answered with the engine |
| `RunWith`, `RunWithRetry` | `RunAssured` and the retry loop | the host answered with the engine |
| `CompleteAgentLifecycle` | the engine's own close step | the host answered with the engine |
| `VerifyCompletion` | nobody in the engine (it called its own) | a promise nobody opened |

The run engine's host port carries **37 entries** now (it carried 45). What
stayed is what the **host** owns: the run's setup and cleanup, the prompt, the
loop configuration, the resume handshake, the retry policy and the host's own
state.

## 4. What the daemon keeps

The daemon still **names** the engine where it needs it:

- the **image admission** asks the module for the vision sidecar
  (`k.runner.DescribeImages(…)`) — the host's own door for every image ingress
  adapter, and the module's operation behind it;
- the **workboard's proof** asks the module for the verification
  (`k.runner.VerifyCompletion(…)`) — the workboard is another module, so it
  asks the host, and the host names the run engine.

**Naming the module is the host's business; answering for it twice is not.**

And the host's own doors stayed, because they are how the callers above
*start* and *close* a run: `Run`, `RunWith`, `RunWithRetry`, `RunAssured` and
`CompleteAgentLifecycle` (the cadence runner calls the last one).

One guard moved with the body: the daemon's `RunWith` rejected an empty
correlation id, and the engine's own `RunWith` now does too — the check
belongs with the implementation once the host's copy is gone.

## 5. The measurement

| what | before | after |
|---|---|---|
| the daemon's public verbs | 135 | **129** |
| the daemon's production lines | 5,648 | **5,622** |
| the daemon's production files | 55 | **55** |
| the app packages that import `kernel/runtime` | 40 | **40** |
| the run engine's host port (`KernelAPI`) | 45 entries | **37 entries** |
| the run engine's package | 910 | **917** |

**Six verbs off the daemon's surface and eight promises off the module's
port** — the port is the W3 exit criterion's real weight, and it is the first
time a slice took a **port** down rather than a verb.

The daemon's **lines went down** (5,648 → 5,622): the wrappers' bodies and
the account of them left together. The engine's package grew by seven — the
guard that moved into it, and the comments that say where the calls go.

## 6. The ratchets — and the probe

**The daemon's side** (`kernel/runtime/runner_engine_ratchet_test.go`, new):
the daemon holds **no** verb per run engine operation, and the daemon's
lifecycle file does not grow a wrapper for one.

**The module's side** (`kernel/modules/runs/runner/selfcall_ratchet_test.go`,
new): the port carries **none** of the run engine's own operations (the check
looks at the **declaration** — a tab-indented name at the start of a line — so
the file's own history stays out of the ratchet), and the engine's production
files do not ask the host for them.

The daemon's own surface ceiling followed the floor again: **135 → 129** (220
→ 201 → 194 → 179 → 173 → 166 → 164 → 156 → 152 → 148 → 143 → 135 → 129), and
the six retired names joined the retired-verb list.

**The probe** put the daemon's `MaybeDistill` wrapper back, put the port entry
back and had the engine ask the host for it again: the retired-verb ratchet,
the surface ceiling (130 > 129), the daemon's two run-engine ratchets and the
module's two port ratchets all went red at once — **five** ratchets in one
shot (six red assertions). The probe was reverted and the suites went green
again.

## 7. The coverage moved with the subject

The daemon's **vision** tests now go through the host's own door —
`AdmitImages` — instead of the wrapper that is gone: the sidecar's routing to
the vision model, and the rejection with no vision model (the correlated
`capability.rejected` record and the clear error).

## 8. Verification

- the compiler (`go build ./...`, `go vet ./...`) and `gofmt`;
- the daemon's tests, the run engine's tests, the applications' tests, the
  control plane's tests, the composition root's tests;
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 9. What is left

The run engine's own operations are the module's, the port carries only the
host's capabilities, and the daemon answers for none of them.

The daemon holds **129** public verbs and **5,622** production lines; **forty**
application packages still name it; the run engine's host port still carries
**37** entries. The next block is the **host's own run doors** — `Run`,
`RunWith`, `RunWithRetry`, `RunAssured`, `CompleteAgentLifecycle` — which the
callers above use to start and close a run: that is the seam where the
daemon's public surface and the run engine's port meet, and it is the largest
thing left in the daemon's run family.

The **standing package** itself still lives at `kernel/standing`, and its
eventual home is the **triggers module** (`kernel/modules/triggers`) — the
architecture gate already places both `kernel/cadence` and `kernel/standing`
in that module, so the family's boundary is drawn; the module's directory does
not exist yet, and the move is a path move (four production importers), not a
lifecycle one.
