# W3.76ah — the run's ticket lifecycle moves into the runs module

**Slice:** W3.76ah (the forty-second of the runtime's dissolution)
**Base:** `4f9f3246` (the W3.76ag merge)
**Date:** 2026-11-12

---

## 1. The family

A durable ticket is the run's own state: one small file per in-flight root
run, so the work survives a restart.

The **rules** that judge it moved into the runs module in W3.76a
(`kernel/modules/runs/resume`): the **scope** rule (a root run carries a
ticket), the **resumability** rule (a per-run override cannot be
reconstructed) and the **keep/delete** rule (a run interrupted by a restart
keeps its ticket). The rows and the store moved with them.

What stayed in the daemon were the **operations** over them:

```go
func (k *Kernel) ClaimResumeTicket(ctx context.Context, corr, intent, kind string, assureBudget int) (context.Context, bool) {
	return k.claimResumeTicket(ctx, corr, intent, kind, assureBudget)
}

func (k *Kernel) FinalizeResumeTicket(corr string, runErr error) {
	k.finalizeResumeTicket(corr, runErr)
}
```

plus a third door, the per-iteration snapshot:

```go
func (k *Kernel) ResumeCheckpointFn(corr string) func(int, []llm.Message) {
	return k.resumeCheckpointFn(corr)
}
```

And the **run engine** — which lives *inside* the runs module, next door to
the tickets themselves — asked the **host** for all three through its
`KernelAPI` port:

```go
runCtx, ownedHere = r.k.ClaimResumeTicket(runCtx, corr, intent, resume.KindRun, 0)
resumeCheckpoint = r.k.ResumeCheckpointFn(corr)
r.k.FinalizeResumeTicket(corr, err)
```

**A port entry that is a round trip inside one module is a promise the host
never had to keep.** The module has the body; the module calls it.

This one is the plainest case of the whole programme: the daemon's bodies
worked on the **module's own store** (`k.resume.Put`, `k.resume.Snapshot`,
`k.resume.Delete`) and applied the **module's own rules**
(`resume.CarriesTicket`, `resume.TerminalOutcome`, `resume.Build`). The host
was a forwarding desk for the module's own object.

## 2. What moved

The ticket's **lifecycle** joined the ticket: `kernel/modules/runs/resume/service.go`
— a `Service` that owns

- `Claim` — the ownership dance (a root run owns a ticket unless an outer
  frame already owns one);
- `CheckpointFn` — the per-iteration snapshot the loop refreshes;
- `Finalize` — the keep/delete rule applied to a run's terminal outcome;
- `SuspendAll` — the shutdown transition (every live ticket becomes a resume
  candidate);
- `Store` — the handle the host's boot-time resumer enumerates.

The dependencies are **narrow ports**, never the host's implementations:

| port | what it is |
|---|---|
| `Store` | the module's own durable ticket store — not a port, the module's object |
| `Enabled` | the host's config: is resume on? (read per run — the operator may turn it off while the daemon runs) |
| `Publish` | the host's bus: the lifecycle's anomalies travel on it |
| `Suspending` | the module's own registry latch: is the daemon going down? |

The daemon hands the module its store, its config switch, its bus and the
suspend latch, and keeps **no** second handle of the store: the daemon's
`ResumeStore()` now asks the module.

## 3. What the daemon keeps

**Two** duties over the tickets, and only two — both the **host's** boot and
shutdown protocol, and both asking the module:

- the **boot-time resumer** (`cmd/agezt/main_standing_resume.go`), which
  enumerates the suspended tickets, quarantines the poisoned ones and
  re-dispatches the rest. It asks the module for the store
  (`k.ResumeStore()`) and for the finalization (`k.ResumeFinalize`);
- the **shutdown protocol** (`k.Suspend`), which latches the registry's
  suspend latch, broadcasts the notice to the live runs and asks the module
  for the suspend transition (`k.tickets.SuspendAll()`).

The daemon's five private bodies are gone with the verbs: `resumeActive`,
`claimResumeTicket`, `buildResumeTicket`, `resumeSetting`,
`finalizeResumeTicket`, `resumeCheckpointFn` and `publishResumeAnomaly`.

## 4. The port

The run engine's host port lost **three** entries — `ClaimResumeTicket`,
`FinalizeResumeTicket` and `ResumeCheckpointFn` — and carries **27** now (it
carried **30**). What stayed is what the **host** owns.

The engine holds the module's service and asks it:

```go
runCtx, ownedHere = r.tickets.Claim(runCtx, corr, intent, resume.KindRun, 0)
resumeCheckpoint = r.tickets.CheckpointFn(corr)
r.tickets.Finalize(corr, err)
```

A host that wired none leaves the engine quiet — it says the lifecycle is
missing **before** the run is registered, so a run whose ticket it cannot own
leaves no live-run state behind.

## 5. The measurement

| what | before | after |
|---|---|---|
| the daemon's public verbs | 114 | **111** |
| the daemon's production lines | 5,372 | **5,277** |
| the daemon's production files | 54 | **54** |
| the app packages that import `kernel/runtime` | 40 | **40** |
| the run engine's host port (`KernelAPI`) | 30 entries | **27 entries** |
| the run engine's package | 997 | **1,025** |
| the resume module's package | 7 files / 635 lines | **8 files / 849 lines** |

**Three verbs off the daemon's surface and 95 lines with them.** The run
engine's package grew by 28 lines (the service's handle, the refusal, the
calls and the comments that say where they go), and the resume package grew
by 214 — the 131 lines that moved out of the daemon, plus the service's own
ports, doc comments and the guards that make a nil port safe.

## 6. The ratchets — and the probe

**The daemon's side** (`kernel/runtime/resume_ratchet_test.go`, new): the
daemon holds **no** verb per ticket operation, its production files hold
**no** ticket-lifecycle body (`claimResumeTicket`, `buildResumeTicket`,
`finalizeResumeTicket`, `resumeCheckpointFn`, `resumeActive`) and **no** copy
of the module's rules (`resume.CarriesTicket`, `resume.TerminalOutcome`,
`resume.Build`, `resume.Facts`) — and it asks the module for the two duties
it keeps.

**The module's side** (`kernel/modules/runs/runner/runner_resume_ratchet_test.go`,
new): the port carries **no** ticket entry, the engine **holds** the module's
service and **calls** it (`r.tickets.Claim`, `r.tickets.CheckpointFn`,
`r.tickets.Finalize`), and a host that wired none leaves the engine quiet.

The daemon's own surface ceiling followed the floor again: **114 → 111** (220
→ 201 → 194 → 179 → 173 → 166 → 164 → 156 → 152 → 148 → 143 → 135 → 129 →
125 → 121 → 118 → 116 → 114 → 111), and the three retired names joined the
retired-verb list.

**The probe** put the world back before this slice — the daemon's three
verbs with their **real** bodies, the three entries back into the engine's
port and the engine asking the host again: the retired-verb ratchet, the
surface ceiling (114 > 111), the daemon's four ticket ratchets, the module's
three ticket ratchets and the two pre-existing port expectations all went
red at once — **eleven** ratchets in one shot, and nothing else broke. The
probe was reverted and the suites went green again.

## 7. The guard

The three verbs were not in any control-plane family by name, so no guard
list changed: the daemon's surface is pinned by its own retired-verb list and
its ceiling, and the port's shape by the run engine's own ratchets.

Two pre-existing expectations moved with the port — the run engine's
`home_internal_test.go` (the port's shape) and its `runctx_ratchet_test.go`
(the host's own state) — and the daemon's `runctx_ratchet_test.go` learned
that the run's **timeout** read left the daemon with the ticket's capture:
the module's service reads it now.

## 8. Verification

- the compiler (`go build ./...`, `go vet ./...`) and `gofmt`;
- the daemon's tests, the run engine's tests, the resume module's tests, the
  applications' tests, the control plane's tests and the command's tests;
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 9. What is left

The daemon holds **111** public verbs and **5,277** production lines; **forty**
application packages still name it; the run engine's host port carries **27**
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
