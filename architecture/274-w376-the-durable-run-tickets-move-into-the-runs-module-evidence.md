# W3.76a — the durable run tickets move into the runs module

## What the slice is

The **ninth** slice of the runtime's dissolution, and the **first** slice of the
**run engine** program (W3.76) — the one family the target architecture wants in
`modules/runs`. It is also the first slice that opens the `kernel/modules/runs`
tree, and the first one that moves a family **with its store** into a module.

The **durable run ticket** is the daemon's answer to "the daemon died and the
work died with it". A ticket is written when a root run starts, its
conversation snapshot is refreshed at every safe iteration boundary, and it is
deleted on a clean finish. If the daemon dies with the ticket still on disk,
the boot-time resumer re-dispatches the run with the conversation it had
accumulated.

The family was split in half across two packages:

- `kernel/resume` (**328** non-blank production lines) held the **rows**: the
  `Ticket`, its `Kind`/`Status` constants, the `Store` (one JSON file per run
  plus a quarantine subdirectory), the atomic write, the size cap and the
  attempt counter.
- `kernel/runtime/resume.go` (**211** non-blank lines) held the **rules**: which
  runs carry a ticket at all (the **scope** rule), which of those may be
  resumed (the **resumability** rule), and whether a finished run's ticket
  survives (the **keep/delete** rule) — plus the daemon's own doors: the
  snapshot checkpoint, the shutdown notice, the event publication.

## What the slice found

The candidate was measured before it was chosen. The whole run engine is
**3,535** non-blank lines (the loop **1,415**, the delegation **881**, the
prompt **578**, the run context **562**, the loop config **99**) — the largest
and the most entangled family the dissolution has left. The resume tickets are
the part of it that is **whole**: a store, a lifecycle and three rules, with
nothing else in the daemon owning any of it.

The measurement of this family:

- the daemon's copies of the ticket rules: **3** (the scope rule, the
  resumability rule, the keep/delete rule);
- the daemon's own stores: **1** — and it is the family's own: the durable
  ticket **is** the run's own durable state.

W3.73 stated a rule about the platform: **a rule that owns no store is the
platform's**. This family is the counterpart, and it is the same statement seen
from the other side: **a rule that owns a store is a MODULE's rule**. The
target architecture says it in one line — the runs module owns the **run
registry, resume tickets, steering/interventions** — and the ticket is that
module's data.

So the whole family moved, in one piece: the rows, the store and the rules, to
`kernel/modules/runs/resume`.

## What changed

### The module keeps the rows and the rules

```
kernel/modules/runs/resume/
  doc.go            (36)   why the ticket is the runs module's
  resume.go         (95)   Kind/Status consts, DefaultSnapshotMaxBytes, Ticket, Store, Open, Dir
  resume_ops.go    (182)   Put/Snapshot/Get/List/Delete/MarkSuspendedAll/IncrementAttempt/Quarantine
  resume_helpers.go (25)   safeName, path, writeAtomic
  rules.go         (120)   the three rules
  ticket.go        (111)   the Setting, the Facts, Build
```

The three rules, with the names the daemon asks:

```go
// kernel/modules/runs/resume/rules.go
func CarriesTicket(parentCorrelation string) bool                 // the scope rule
func TerminalOutcome(runErr error, suspending bool) Disposition   // the keep/delete rule
func JudgeResumability(facts DispatchFacts) Resumability           // the resumability rule
```

The ticket's capture, with the verdict the module owns:

```go
// kernel/modules/runs/resume/ticket.go
type Setting struct { Set bool; SourceProfile string }
func (s Setting) Reconstructible(agentSlug string) bool

func Build(f Facts) *Ticket
```

`Build` assembles the ticket **and** asks `JudgeResumability` for the verdict,
so the flag on the row and the rule that judges it cannot drift apart.

### The daemon keeps the doors

```go
// kernel/runtime/resume.go
func (k *Kernel) resumeActive(ctx context.Context) bool {
	return k.resume != nil && k.cfg.ResumeEnabled &&
		resume.CarriesTicket(wakeContextFromCtx(ctx).ParentCorrelation)
}

func (k *Kernel) buildResumeTicket(ctx context.Context, corr, intent, kind string, assureBudget int) *resume.Ticket {
	facts := resume.Facts{ /* what the daemon resolved */ }
	return resume.Build(facts)
}

func (k *Kernel) finalizeResumeTicket(corr string, runErr error) {
	if resume.TerminalOutcome(runErr, k.suspending.Load()) == resume.DispositionKeep {
		return // interrupted by shutdown → keep for resume
	}
	// … delete the ticket
}
```

The daemon reads the run's context, hands the facts over and publishes the
events. It no longer holds a copy of any of the three rules.

## The rule

**The host keeps the doors; the module keeps the rows and the rules that judge
them.**

W3.73–W3.75 moved rules that own **no** store into the platform. This one
moves a family that owns **its own** store into a module — and the difference
is not cosmetic: a rule with a store has a **lifecycle** (a ticket is written,
refreshed, suspended, quarantined and deleted), and a lifecycle needs an owner.

## What the slice cost

**Ten import sites** and nothing else:

- `kernel/runtime/compose.go` (the store's opening), `kernel/runtime/runtime_kernel.go`
  (the store's field);
- `kernel/runtime/runexec/{runner,runner_lifecycle,runner_retry}.go`;
- `cmd/agezt/main_standing_resume.go` (the boot resumer) and
  `cmd/agezt/resume_profile_test.go`;
- `kernel/runtime/{resume_lifecycle_test,resume_profile_internal_test}.go` and
  `kernel/controlplane/run_resume_profile_test.go`.

No new face, no new contract row, no new test setup, no change to the
`runexec` kernel API. The move was a **move**: the daemon's file shrank by the
rules and grew by the calls.

## What did NOT move, and why

- **The store's FILE opening stayed in the kernel's composition root**
  (`kernel/runtime/compose.go` opens `<BaseDir>/resume`). The kernel already
  opens its journal, its state store and its config center from the same base
  directory, and the ticket store is the **run engine's own** durable state —
  the daemon never makes a decision about it, it only asks the kernel for it
  (`k.ResumeStore()`) at boot. The roster and the forge are the other shape:
  those are **services** with a host-facing port, so the composition root
  opens their files and the kernel holds a contract port (W3.56, W3.72). The
  ticket store is not that, and moving it would have bought a contract row and
  six test wirings for nothing.
- **The ticket format did not change.** The same JSON, the same fields, the
  same file names, the same quarantine layout — a ticket written by the old
  daemon is read by the new one.
- **The resumer's behaviour did not change.** The boot resumer still reads the
  suspended tickets, quarantines the poison ones, re-dispatches the rest with
  the conversation it saved, and still bumps the durable attempt counter
  **before** the re-dispatch (the crash-loop guard).
- **The run context did not change.** `WithResumeSeed`, `WithResumeOwned` and
  `ResumeFinalize` keep their names and their signatures; the `runexec`
  kernel API (`ClaimResumeTicket`, `FinalizeResumeTicket`,
  `ResumeOwnedKindFromCtx`, `ResumeSeedFromCtx`) is untouched.
- **The `runctx` keys did not move.** The run context is a separate family
  (W3.76's next slices).

## The contract row

**None.** The ticket's wake provenance uses the **existing** row
`agentapi.WakeContext` — the module flattens it into the row's flat fields, so
the module never names the runtime's own type.

## The tests

**12 new permanent tests** (7 in the module, 5 in the daemon), on top of the
**15 store tests** that moved with the package:

- `kernel/modules/runs/resume/rules_test.go` — **4 tests**: the scope rule is
  the root run's, the termination rule keeps a ticket **only** for a shutdown
  interruption, a setting's reconstructibility follows its source, and the
  resumability rule resumes a profile run but not an overridden one.
- `kernel/modules/runs/resume/ticket_test.go` — **3 tests**: the ticket
  captures the run's dispatch context, a run that never had a trust ceiling
  records none, and the verdict on the ticket is the module's.
- `kernel/runtime/resume_internal_test.go` — **5 tests**: the **ratchet** (the
  daemon's resume hook must not grow the ticket rules back), the scope rule is
  the module's, the scope rule needs the feature and the store, the ticket
  captures the governance ceilings, and the keep/delete rule is the module's.

The eight-scenario profile table (`kernel/runtime/resume_profile_internal_test.go`)
still passes unchanged: a profile-sourced default is reconstructible, a
per-run override is not, a retargeted identity is not, and an unnamed profile
is not.

### The test the mutations had to sharpen

None this time — every mutation was caught by the test that watches the
behaviour it breaks. The calibration below is the record of that.

## The mutations

**37 mutations, 37 caught**, in two independent passes with **byte-identical**
logs.

SHA-256: `1DCBE55A3200EFAC6EDD76CA31BA942EF3D723B59862079A1A12317FDD5DFFBB`

The calibration reported **one compile error**, and it was the mutation's
fault: the mutation that made the termination rule always keep replaced the
whole `if` condition, which left `context` and `errors` imported and unused —
Go refused to compile. It now mutates the rule's **fallback** instead, so both
imports stay alive and the mutation still changes the behaviour.

- **M01**, **M02** the scope rule is inverted, or it carries every run.
- **M03**, **M04**, **M05**, **M06** the termination rule drops the timeout
  branch, ignores the suspending flag, always keeps, or never keeps.
- **M07**, **M08**, **M09**, **M10** the resumability rule ignores the tool
  allowlist, the system override or the model override, or never resumes.
- **M11**, **M12** an unset setting is treated as set, or an empty profile
  source is accepted.
- **M13**–**M21** the ticket's capture: the assure budget, the agent slug, the
  cost cap, the trust ceiling, the run timeout, the wake source, the wake
  subject, the status, or the agent the verdict is judged against.
- **M22**–**M28** the store: the default status, the snapshot's conversation,
  the attempt counter, the suspended status, the quarantine's directory, the
  size cap, the file name's path syntax.
- **M29**–**M37** the daemon's doors: it bypasses the scope rule, ignores the
  suspending flag, hides the system or model source, hides the tool override,
  or loses the trust ceiling, the run timeout, the cost cap or the agent slug.

## The measurement that moved

| | before | after |
|---|---|---|
| the daemon's copies of the ticket rules | **3** | **0** |
| the module's lines (the family) | **0** | **569** |
| the daemon's `resume.go` (lines) | **211** | **219** |
| `kernel/resume` (the old package) | **328** production lines | **gone** |
| `kernel/runtime` (upper level, lines) | **8,687** | **8,695** |
| `kernel/modules` (recursive, lines) | **25,266** | **25,844** |
| the kernel's verbs | **242** | **242** |
| the import sites that changed | — | **10** |
| the contract's new rows | — | **0** |
| the test setups that had to be wired | — | **0** |

The daemon's copies of the ticket rules fell from **3** to **0** — and a
**ratchet test** holds them there.

## The measurement that did NOT move

The daemon's `resume.go` grew by **8** lines (211 → 219), and the runtime
grew with it (**8,687 → 8,695**).

That is the same shape W3.74 found and W3.75 explained: **moving a rule moves
the CONDITIONS, not the lines.** The daemon's file did not lose the rules — it
lost the rules' **bodies** and kept the **wiring** that feeds them: the facts
the daemon resolves (`agentSlugFromCtx`, `maxCostFromCtx`,
`trustCeilingFromCtx`, `runTimeoutFromCtx`, the two run settings, the tool
allowlist), the call into the module, and the `resumeSettingFromCtx` helper
that maps a resolved value and its source profile onto the module's `Setting`.
That wiring was always there — it was *inside* the conditions.

The line counts are counted the way the previous slices counted them:
**non-blank** lines of the package's production files (`git ls-files`, upper
level for the runtime, recursive for the modules).

## What is left

The run engine's dissolution stands at **242 verbs** in **8,695 lines**.

This slice took the **tickets**. What remains of the run engine for `modules/runs`:

- the **delegation** (**881** non-blank lines) — `prepareSubAgent` (279),
  `executeSubAgent` (89), `subAgentInjectedSystem` (87) and the tree's rails;
- the **prompt** (**578**) — `buildRunPrompt` (170) and the run's context
  assembly;
- the **loop** (**1,415**) — the agent loop and `buildLoopConfig` (99);
- the **run context** (**562**) — the 22 context keys and their accessors.

`kernel/runtime/runexec` is already ported (`KernelAPI`, 126 non-blank lines) and
imports no runtime package, so it moves cleanly once the loop does.

The self-repair module's edge into the runtime is still in the archcheck
allowlist — it is the other program's work, and it waits there.
