# W3.76o — the run engine asks the agents module for the roster

**Slice:** W3.76o (the twenty-third of the runtime's dissolution)
**Base:** `8caa91d2` (the W3.76n merge)
**Date:** 2026-11-02

---

## 1. The family

W3.76n moved the roster's **operations** into the agents module. This slice
closes the loop that move opened: the **run engine** was still asking the
**host** for the roster's writes.

The run engine writes the roster's rows. When a run completes under a named
agent, the engine decides what happens to that agent — it retires an agent
whose run was its last (`RetireOnComplete`), and it advances the agent's
**cycle counter** for a cycling agent (`LifecycleCycle`). Both are writes to
the roster's rows.

After W3.76n those writes are the agents module's operations, so the chain
became:

> the run engine → the host → the agents module → the store

and the middle hop bought nothing: the run engine lives in the **runs** module
and may import **another module's api package** — that is the whole point of
the api layer (archcheck's P5 allows it, and the code already does it for the
runs module's own packages).

## 2. What moved

The run engine's port (`runner.KernelAPI`) carried five roster entries. Their
fates differ, and the difference is the measurement:

| the port entry | who opened it | the fate |
|---|---|---|
| `UpdateProfile` | the run engine (1) | **the module's** — the engine asks the module |
| `SetProfileRetired` | the run engine (2) | **the module's** — the engine asks the module |
| `Roster()` | **nobody** in the runs module | **gone** — a port entry that is never opened is a promise the host does not have to keep |
| `RosterList()` | **nobody** in the runs module | **gone** — the same |
| `RosterGet` | the run engine (1) | **stays** — a read over the contract's port is the host's own state |

So the run engine now **holds the agents module's roster service**
(`agentsapi.RosterService`, injected by the composition root — the same
service the daemon holds) and asks *it* for the writes:

```go
func (r *Runner) retireAgent(slug, reason string) {
	if r.roster == nil {
		return
	}
	_, _ = r.roster.SetRetired(slug, true, reason)
}
```

A host that wired no roster service leaves the run engine **quiet**, not
broken: the run finished either way, and there is no roster to write.

The two dead entries deserve their own word. `Roster()` and `RosterList()`
were in the port because the port once mirrored the daemon's whole surface.
Nobody in the runs module ever opened them — and the daemon's own public
`Roster()`/`RosterList()` verbs stay, because the **application layer** and
the agent gateway do call them. The port entry and the public verb are two
different things: a port entry is a promise the host makes to *this* module.

## 3. The measurement

| measure | before | after |
|---|---:|---:|
| the run engine's port (`runner.KernelAPI`) | 64 | **60** |
| the daemon's public verbs | 203 | **203** |
| the daemon's production lines (non-blank) | 6,156 | **6,162** |
| the runner package's production lines (non-blank) | 883 | **931** |
| the agents module (production lines) | 4,291 | **4,291** |

The daemon's public surface did not move: the daemon still holds
`UpdateProfile` and `SetProfileRetired` as **its own** verbs (the application
layer's roster ports and the self-repair module still call them), and what
changed is that the run engine no longer *needs* them through the port. The
runner package grew by 48 lines — the roster field, the two small helpers and
the comments that say why.

## 4. The ratchet — and the probe

The run engine's own port-shape ratchet
(`kernel/modules/runs/runner/home_internal_test.go`) now pins the four names'
**absence** alongside the two run-context names it already pinned:

```
--- FAIL: TestTheHostStillImplementsThePort (0.00s)
    home_internal_test.go:130: the KernelAPI port has a UpdateProfile method again — the roster's writes are the agents module's, and nobody in this module opened the store
```

**The probe fired**: putting `UpdateProfile` back into the port made the
ratchet fail, and the file was restored byte-for-byte afterwards.

The first probe attempt did **not** fire, and that is worth recording: it
added the method to a *struct* instead of to the *interface*, so the port's
method set never changed. **A ratchet that watches a shape must be probed
against that shape** — a probe on a neighbour proves nothing.

## 5. Verification

- **the compiler**: `go build ./...` and `go vet ./...` clean;
- **the run engine's own tests**
  (`kernel/modules/runs/runner/runner_roster_test.go`, 3 tests): the engine
  asks the module to retire the agent, the engine is quiet without a service,
  and the engine leaves an unknown agent alone;
- **the daemon's test**
  (`kernel/runtime/roster_service_test.go`): the composition root wires the
  service into the engine, so a completed run retires its agent **through the
  module** (the store shows the agent retired);
- **the port-shape ratchet and its probe** (§4);
- **the full test suite** and **the thirteen gates**.

**No mutation run.** The slice moves two calls from one owner to another and
deletes two port entries; there is no rule whose reversal a mutation could
expose. The evidence is the compiler, the permanent tests, the ratchet and its
probe, and the full suite.

## 6. What is left

The daemon holds **203** public verbs and **6,162** production lines; the run
engine's port carries **60** entries and no roster vocabulary beyond the one
read.

The next door in the same family is the **delegation** package: it has its own
`Host` port (`kernel/modules/runs/delegation/host.go`) and it asks the host
for `RosterGet` — the same read the run engine asks for, through a second
port. And the wider W3 exit is unchanged: the daemon's composition root
(`compose.go`), its `Kernel` (`runtime.go`), the accessors family and the
control plane (`kernel/controlplane`, 7,570 lines).

Evidence: `architecture/288-w376o-the-run-engine-asks-the-agents-module-for-the-roster-evidence.md`
