# W3.39g — the live kernel leaves the registry

## The last row a plugin could not name

After W3.39f, **every field of every row of the registry was a contract row or
a contract port, with exactly one exception**: `KernelDeps.K *runtime.Kernel`.

That one field was the whole remaining leak, and it was a leak of a specific
kind: the plugin never imported `kernel/runtime` — the **field's type** was the
dependency. `plugins/builtintools`'s only L3 import was `kernel/toolreg`; the
kernel itself was reached only through the type of a row the registry carried.

So this slice removes the field — and with it the last type-level dependency
the first-party tools had on the composition root's runtime.

## The three uses it served

`d.K` was used in exactly three places. Each needed a different shape, and
each got one.

### 1. The forge's runner — a port, and a row that had to move with it

```go
// before
ft.Bind(d.K, d.Forge)
```

The tool_forge tool's port is **four verbs** — draft, update, test, ask the
operator to promote — and the fourth one named a **platform** type:

```go
RequestToolPromotion(ctx context.Context, corr, ref string) (toolforgeapi.ScriptTool, approval.Decision, string, error)
```

`approval.Decision` lives in `kernel/approval` (L2). The contract layer may not
import it (archcheck's principle P4), so **the row had to move with the port**:

- new contract package `kernel/contract/approvalapi` — `Decision`, its four
  values and `IsTerminal()`;
- `kernel/approval` keeps `type Decision = approvalapi.Decision` and the four
  constants as aliases, so **all 92 usages across 30 files keep compiling
  unchanged**;
- new port `toolforgeapi.Runner` (the four verbs), and
  `forgetool.Kernel = toolforgeapi.Runner` — every caller and every fake in the
  tool's tests is unchanged.

**The kernel satisfies the new port with no adapter at all.** Its own methods
already speak the contract's rows (`toolforge.ScriptTool` is an alias of
`toolforgeapi.ScriptTool`; `approval.Decision` is now an alias of
`approvalapi.Decision`), so the two are the *same types* — the port is
satisfied by identity, not by a translation.

### 2. The Verifier's code executor — a row that already existed

```go
// before
d.K.SetConductorExec(ce)
```

The executor is **one verb** (`RunScript`) and pure. The contract already had
it — `toolregapi.ScriptRunner`, added in W3.38 for the pre-Open phase — with
the **exact** signature. So the runtime's own `CodeExecutor` became an
**alias** of it:

```go
type CodeExecutor = toolregapi.ScriptRunner
```

and the kernel's setter signature is now the contract's row, for free. The
registry's row is a **plain closure**:

```go
ConductorExec func(x toolregapi.ScriptRunner)
```

— a function value names no type, so the row stays in the contract without the
contract naming the kernel (the same shape as the `AgentLookup` closure of
W3.39d). The daemon supplies `func(x toolregapi.ScriptRunner) { k.SetConductorExec(x) }`.

### 3. The artifact bytes — a closure, and no new type at all

```go
// before
smt.Bind(d.ChannelSendMedia, targets, func(ref string) ([]byte, error) {
	if k != nil {
		if a := k.Artifacts(); a != nil {
			return a.Get(ref)
		}
	}
	return nil, fmt.Errorf("artifact store unavailable")
})
```

The resolver was **already a closure** — so this needed no port and no new
contract package. The registry's row is the closure itself:

```go
ArtifactBytes func(ref string) ([]byte, error)
```

and the daemon builds it over the live blob store (the daemon may name the
artifact module). The blob store re-verifies the hash on the way out, so a
tampered blob is refused rather than attached to a message — and that check
travelled with the row.

## The typed-nil rule, made explicit

The code_exec tool hands **itself** to the row. A nil `*codeexec.Tool` handed
to an interface parameter is a **non-nil interface** — the Verifier would
believe it can execute code and then call into a tool that is not there. So
the hand-over is a named function with the check in it:

```go
func installExecutor(ce *codeexec.Tool, d toolreg.KernelDeps) {
	if ce == nil || d.ConductorExec == nil {
		return
	}
	d.ConductorExec(ce)
}
```

## The ratchet did NOT move — for the seventh time

**78/13**, unchanged, and that is the point of the slice, not an omission.
The registry's **rows** are still in `kernel/toolreg` (L3), and the plugin
still imports it; the last `plugin-reach` edge falls only when the registry
itself moves down to the contract layer.

What this slice changes is that the registry's rows are now **all**
contract-shaped — **with no exception left**. The last move (W3.39h) is now a
single, mechanical step: the rows and the registry's own functions
(`Register`, `Names`, `BuildAll`, `Set`, ...) move to the contract layer, the
plugin imports `kernel/contract/toolregapi` instead of `kernel/toolreg`, and
the edge falls — **ratchet 78 → 77**.

The only thing standing in that move is the registry's own reach for the
daemon's configuration (`Set.ApplyPreOpen(cfg *runtime.Config)`), and even
that has a home: the adapter moves to the composition root, which may name
`*runtime.Config` — and `runtime.Config.ScriptRunner` is **already** the
contract's runner.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `kernel/contract/approvalapi/approvalapi_test.go` | the verdict's vocabulary is **closed** (four values, all terminal, nothing else) and the row is a plain string |
| `kernel/contract/toolforgeapi/runner_test.go` | the forge runner port carries **exactly four** verbs, and every type it names is the contract's row (the script tool, the verdict) |
| `cmd/agezt/kernel_row_ports_test.go` | the **nil-port rule** on the forge runner (checked on the concrete type); the daemon supplies the port and it carries the **live** implementation (a draft made through it appears in the daemon's own store); the conductor-exec row is supplied and its **effect** is observable (the kernel reports that it can execute code only after the row is called); the artifact row hands back the stored bytes **verbatim**; both closures survive a missing kernel |
| `plugins/builtintools/kernel_row_ports_test.go` | the forge spec with no runner leaves the tool reporting that the forge is unavailable; the executor row receives the tool and **refuses a nil one**; the send_media row resolves the bytes to the channel, and an unwired row says the store is unavailable and delivers **nothing** |

Two **existing** tests were re-pointed at the new rows
(`forge_ports_test.go`, `config_ports_test.go`, `introspect_ports_test.go`):
they carried the live kernel in the row they measured; the kernel now arrives
as the port that serves it — what they measure and what they assert are
**unchanged**.

### Mutation run

**15 mutations: 14 caught, 0 survived, 0 skipped, 1 build error** — and the one
non-catch is an honest fact about the slice, not a gap. (A **sixteenth**
mutation, measured locally against the collateral fix below, brings the run to
**16 / 15 caught**.)

| # | Mutation | Caught by |
|---|---|---|
| 1 | the forge spec never binds the runner | the forge test |
| 2 | the send_media row resolves to nothing | the plugin's row test |
| 3 | the send_media row says the wrong thing when unwired | the plugin's row test |
| 4 | the executor row accepts a nil tool | the typed-nil test |
| 5 | the executor is never installed | the plugin's row test |
| 6 | the daemon does not supply the forge runner | the daemon's test |
| 7 | the forge runner port stops being nil-safe | the nil-port test |
| 8 | the daemon does not supply the conductor-exec row | the daemon's test |
| 9 | the conductor-exec row installs nothing | the daemon's test (the kernel's `CodeExecAvailable`) |
| 10 | the artifact row hands back nothing | the daemon's wire test |
| 11 | the forge runner port grows a fifth verb | the port's shape test |
| 12 | the verdict row is never terminal | the vocabulary test |
| 13 | **gate**: the contract names the platform again | **archcheck** (principle P4) |
| 14 | the verdict vocabulary loses a value | the vocabulary test (the first run **skipped** it — the anchor did not match the gofmt-aligned constant — and it was re-run) |
| 15 | the platform stops speaking the contract row | the **compiler** — a build error, not a measured failure: the alias's identity is enforced by the type system, not by a test |
| 16 | the rename retry is removed (the collateral fix, below) | the skill package's own test — caught **locally**, on round 2 of 3 (the race is probabilistic) |

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 311 packages placed; 78 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — unchanged, as designed |
| `go run ./tools/deadcodecheck` | OK — 28 public SDK findings and 1 cross-package test seam allowlisted |
| `staticcheck ./...` | clean |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** (two Windows-only flakes below were found by this run and fixed) |

## A Windows race the full test run found (collateral)

The slice's own full test run dropped `TestSeedBuiltins_ReportsTheQuarantinedStatus`
— and the cause was **not** in the slice. Seeding a built-in skill bundle
removes the old bundle and renames the staged one over it:

```go
if err := os.RemoveAll(dst); err != nil { ... }
if err := os.Rename(tmp, dst); err != nil { ... }
```

On Windows a scanner (or an indexer) that still holds a handle on one of the
old bundle's files makes that delete only **PENDING**: the directory is still
there, and `MoveFileEx` refuses to replace it — *"Access is denied"*. The
handle is released in milliseconds, so the delete succeeds a moment later and
the rename never gets its chance.

`internal/atomicfile` already carries this exact hazard and its exact remedy
(remove-then-rename, retried). A bundle is a **DIRECTORY**, so it cannot go
through that helper — the fix had to live in the bundle's own IO:

```go
func renameBundle(oldPath, newPath string) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 20 * time.Millisecond)
		}
		if err = os.Rename(oldPath, newPath); err == nil {
			return nil
		}
		if runtime.GOOS != "windows" || !os.IsPermission(err) {
			return err
		}
	}
	return err
}
```

The retry is bounded, **Windows-only**, and only for a permission failure —
every other error returns at once, so the helper cannot turn a real fault into
a slow one. It is a **fix**, not a gate: no CI job runs on Windows, so this
cannot be measured by CI, and the mutation below was measured locally.

### The mutation (measured locally)

| # | Mutation | Caught by |
|---|---|---|
| 16 | the rename retry is removed (5 attempts → 1) | the skill package's own test — **caught on round 2 of 3** |

The catch is **probabilistic**, and the honest record says so: round 1 (8 runs)
**survived**, round 2 (8 runs) failed with

```
--- FAIL: TestSeedBuiltins_ReportsTheQuarantinedStatus (0.21s)
    SeedBuiltins: seed browseruse: skill: commit bundle: rename
    ...\bundles\browser-use.tmp ...\bundles\browser-use: Access is denied.
```

The race needs a handle held at the right moment, so no fixed run count
guarantees it. What the measurement shows is that **the retry is load-bearing**:
without it the failure reproduces, and with it (`-count=8`) the package is
green. The mutation was **not** weakened to make it deterministic — a
deterministic reproduction would need a handle held deliberately, which is a
Windows-only harness this project does not carry.

The fix is in the slice because the slice's own test run surfaced it; it is
**not** part of the slice's architecture claim.

### The second flake: a timeout test that measured the machine

The same full test run also dropped `kernel/warden`'s
`TestRun_TimeoutKillsAndFlagsTimedOut`, with an exact message:

```
warden_test.go:164: Run: warden: start "cmd": context deadline exceeded
```

The root cause is in the **budget**, not in the kill path. The deadline the
warden installs does not only pay for the run — it also pays for PATH
resolution and process creation, because `os/exec` checks the context
*before* it creates anything:

```go
// os/exec/exec.go — Cmd.Start
if c.ctx != nil {
	select {
	case <-c.ctx.Done():
		return c.ctx.Err()
	default:
	}
}
```

So on a loaded machine the 200 ms budget could be **gone before the process
existed**, and `Start` refused to create it. The engine's answer is honest —
nothing ran, so it is a start failure and not a timeout — but the test was
measuring the machine's process-creation latency, not the property it exists
to measure ("a run that exceeds its deadline is killed and flagged").

The fix repeats the run with a budget that covers the start, and makes the
wall-clock bound **relative** to the budget, for the same reason the bound
already existed: a bound that tight measures the scheduler, not the warden.

**It weakens nothing.** The run that is measured still has to start, still has
to run past its deadline, still has to be killed, still has to report
`TimedOut=true`, and still has to publish the `warden.limit_exceeded` event.
A broken kill path fails **both** attempts — it never returns at all and
trips the package timeout instead.

| | before | after |
|---|---|---|
| 200 runs under load (a full `go test ./...` in parallel) | **11+ failures** (`warden: start "cmd": context deadline exceeded`, up to 3.88 s each) | **0 failures** (44.2 s for all 200) |
| the full `go test ./...` running **concurrently** with that loop | dropped the warden package | **all green** |

A third flake was observed under that same artificial double load —
`toolbox`'s `TestProbeVersion` (its 3 s version-probe budget ran out under
the extreme load) — and is **recorded, not changed**: it did not reproduce in
any normal run, it is Windows-only, and no CI job runs on Windows.

## What is deliberately left

The registry itself — its rows and its functions — still lives in
`kernel/toolreg` (L3). That is the last move, and it is a move, not a
redesign: the rows are already contract-shaped, and the only thing the
registry names that the contract may not is the daemon's configuration, whose
adapter belongs to the composition root anyway.
