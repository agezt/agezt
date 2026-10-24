# W3.39h — the registry itself moves down to the contract layer

## The last plugin-reach edge

After W3.39g the registry's **rows** were all contract-shaped, with no
exception left. What still stood between the built-in tools and the contract
layer was the **registry itself**: `kernel/toolreg` (L3, module "runs"), which
the plugin imported for the `Spec` type, the dependency rows and `Register`.

So the last `plugin-reach` edge was structural, not a row:

```
plugin-reach plugins/builtintools -> kernel/toolreg
```

It was the **only** such entry left in the allowlist, and it is now **gone**:

```
OK: 310 packages placed; 77 allowlisted import violations and 13 allowlisted
call sites remain (ratchet).
```

**78 → 77.** The first slice in the whole W3.39 series that moved the
ratchet down, and it moves it by exactly the one edge it names.

## Why the whole package had to move, and not just the rows

A half-move would have left the package split across two layers for no
reason: `Spec`, `BuildDeps`, `KernelDeps`, `LateDeps` and `Built` are the
registry's vocabulary, and `Register`, `Names`, `BuildAll` and `Set` are the
only way to use it. The plugin names **both** — it declares a `Spec` and it
registers it — so both halves have to be where the plugin may name them.

The move was therefore: **the registry's rows and the registry's own
functions, together**, down to the contract layer.

## Three names had to change with it

The registry is now a **contract** package, and archcheck's principle P4 says
a contract may import the standard library and other contract packages —
nothing else. It had exactly three names outside that:

| Before | After | Why |
|---|---|---|
| `channel.Attachment` | `channelapi.Attachment` | `kernel/channel` is not under `kernel/contract/`; the channel package keeps its name as an **alias** of the contract's, so this is one row with one spelling |
| `runtime.PluginInfo` | `types.PluginInfo` | `kernel/runtime` is a module (L3); the runtime keeps its name as an **alias** of the contract's row |
| `ApplyPreOpen(cfg *runtime.Config)` | `ApplyPreOpen(cfg Config)` | the hook answers the contract's **port**; the adapter that answers the port over the daemon's configuration moved to the composition root |

The first two were free: both were already **aliases**, so the change is a
change of *spelling*, not of type. The third was the real work, and it is
where the slice's shape decided itself.

## The adapter moved to where the configuration lives

The registry used to wrap the daemon's configuration in a `runtimeConfig`
adapter — it could, because the registry was the one place that knew both the
contract and the daemon. Now it cannot: it is a contract package, and it may
not name `*runtime.Config` at all.

So the adapter travelled **down to the composition root**, which is where the
configuration lives anyway:

```go
// cmd/agezt/tooldeps.go
type runtimeConfigPort struct{ cfg *runtime.Config }

func (p runtimeConfigPort) SetScriptRunner(r toolregapi.ScriptRunner) {
	if p.cfg == nil {
		return
	}
	p.cfg.ScriptRunner = r
}
```

This is not a workaround — it is the shape the architecture wanted all along.
The registry was a **contract** pretending to be an implementation; the
adapter was the seam where it could not keep the pretence.

## The plugin's test stopped reaching for the daemon's configuration

The built-in tools' own test used to hand the pre-Open phase a live
`*runtime.Config`:

```go
var cfg runtime.Config
set.ApplyPreOpen(&cfg)
```

That is a plugin reaching for the daemon's configuration type — the very
thing the port exists to prevent, one layer down. The test now speaks the
**port** with a fake of it:

```go
cfg := &fakePreOpenConfig{}
set.ApplyPreOpen(cfg)
if cfg.runner != nil { /* ... */ }
```

One spelling of the wire: the adapter that answers the port over the daemon's
configuration lives in **one** place (the composition root), and a second copy
inside the plugin's test would have been a second spelling of it.

## A gap the slice found in itself: the call site nobody measured

The daemon's pre-Open call sat in the middle of `runDaemon` — a hundred-line
function that no test calls. So the wiring "the daemon hands the phase a port
built over **its own** configuration" was **true and unmeasured**: a change to
`ApplyPreOpen(nil)` would have compiled, booted, and left the script-tool
forge (M794) permanently unavailable, with no error anywhere.

The fix is the same one W3.39g used for the kernel's rows — a **named
function** at the composition root:

```go
func applyPreOpen(set *toolregapi.Set, cfg *kernelruntime.Config) {
	if set == nil {
		return
	}
	set.ApplyPreOpen(runtimeConfigPort{cfg: cfg})
}
```

Now the call site is a thing a test can hold. The daemon's test registers a
probe spec, builds the real tool set, runs the phase and asserts that the
port it handed over is wired to **that** configuration.

## The gate files move with it

| File | Change | Why |
|---|---|---|
| `tools/archcheck/allowlist.txt` | the `plugin-reach plugins/builtintools -> kernel/toolreg` line **deleted** | the check fails when a listed edge no longer occurs — the stale entry had to go |
| `tools/archcheck/layers.json` | the `kernel/toolreg` L3 rule **deleted** | the directory is gone; a stale rule would match nothing |
| `tools/deadcodecheck/main.go` | the seam key `kernel/toolreg/toolreg.go\|Names` → `kernel/contract/toolregapi/registry.go\|Names` | the allowlist is keyed by **file\|symbol** on purpose, so the file's move is a key change |
| `tools/deadcodecheck/main_test.go` | the fixture strings moved with the key | the full test run caught the stale ones (below) |

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `kernel/contract/toolregapi/registry_shape_test.go` → `TestTheRegistryRowsAreTheContractsRows` | the rows are the **contract's** rows, checked on the TYPE and not on an assignment: `Spec.PreOpen` takes the contract's `Config` port and the contract's tool, `Built.Infos` is `[]types.PluginInfo`, `LateDeps.ChannelSendMedia` takes `[]channelapi.Attachment` — and **no row grows a field** (`Spec` has 7, `Built` has 6) |
| `kernel/contract/toolregapi/registry_shape_test.go` → `TestApplyPreOpenHandsThePortToTheHook` | the phase hands the hook **the very port it was given** — not a wrapper of its own. This is the behaviour the move changed: if the registry wrapped the port again, the installation would land in a copy and the forge would keep reporting itself unavailable, with no error anywhere |
| `cmd/agezt/preopen_port_test.go` → `TestThePreOpenPortInstallsTheRunner` | the daemon's adapter really installs the runner into the daemon's configuration — the effect M794 depends on |
| `cmd/agezt/preopen_port_test.go` → `TestThePreOpenPortRefusesAMissingConfiguration` | a configuration that is not there is **refused**, not dereferenced (the panic would land in the pre-Open phase, before the kernel opens) |
| `cmd/agezt/preopen_port_test.go` → `TestApplyPreOpenHandsTheHookTheDaemonsPort` | the **call site**: the phase reaches a hook, and the port it handed over is wired to *that* configuration — the gap the slice found in itself |

The **moved** tests came with the package: `registry_test.go` (the registry's
own behaviour), `close_test.go` (what `Build` started is released), and
`toolregapi_test.go` (the pre-Open port's shape). The plugin's own
`ratchet_test.go` still pins the boot tool surface through `Names()` — and
`Names()` moved with the package, so the deadcodecheck seam followed it.

### The mutation run

**14 mutations: 14 caught, 0 survived, 0 skipped, 0 build errors.**

| # | Mutation | Caught by |
|---|---|---|
| 1 | the contract names something outside the contract layer again (`kernel/warden`) | **archcheck** — `impure-contract kernel/contract/toolregapi -> kernel/warden` (P4) |
| 2 | the plugin names a module again (`kernel/runtime`) | **archcheck** — `plugin-reach plugins/builtintools -> kernel/runtime` |
| 3 | the stale `plugin-reach` allowlist entry comes back | **archcheck** — a listed edge that no longer occurs |
| 4 | the deadcodecheck seam key goes stale | **deadcodecheck** — `kernel\contract\toolregapi\registry.go:302:6: unreachable func: Names` |
| 5 | the phase hands the hook a port of its own | the contract's port test |
| 6 | the phase stops calling the hook at all | the contract's port test and the moved registry test |
| 7 | the daemon's adapter installs nothing | the daemon's adapter test and the call-site test |
| 8 | the daemon's adapter drops the nil guard | the daemon's adapter test (**panic**: nil pointer dereference) |
| 9 | the daemon hands over a port over nothing | the daemon's call-site test |
| 10 | a row grows a field nobody names | the shape test |
| 11 | the registered names stop being ordered | the plugin's boot ratchet |
| 12 | `Register` stops registering | the moved registry test |
| 13 | the plugin manifest reports nothing | the moved registry test |
| 14 | `Close` stops closing | the moved close test |

**Four of the fourteen are gate-measured**, and that is the point of the
slice: the claim "the plugin no longer reaches a module" is not something a
test can see — it is what archcheck counts.

### The honest record of the run

The first run of the harness was **invalid** and was thrown away: the
mutations were declared as nested PowerShell arrays, and PowerShell
*unrolls* a nested array into its parent — so `$p[0]` was the first
**character** of the anchor, and ten mutations were applied to the wrong
place. The second run had a different fault: the restore of
`plugins/builtintools/inject.go` failed on Windows (*"The requested operation
cannot be performed on a file with a user-mapped section open"* — the Go build
had the file memory-mapped), so mutation 2 stayed in the file and mutation 3
was measured against the **wrong cause**. Both were redone: the harness now
declares its pairs as objects and retries the restore, and mutations 2 and 3
were re-run and caught by the right reason each:

```
M02 CAUGHT :: - plugin-reach plugins/builtintools -> kernel/runtime
M03 CAUGHT :: - plugin-reach plugins/builtintools -> kernel/toolreg
```

A mutation that was measured against the wrong cause is not a caught mutation.
It is a measurement that happened to pass.

### The collateral the full test run surfaced

The full test run dropped `tools/deadcodecheck`'s own tests: their fixture
strings still named `kernel/toolreg/toolreg.go`. The seam allowlist is keyed
by file, the key moved, and the fixtures — which mirror the real allowlist —
had to move with it. Fixed in the same slice: the gate and its fixtures now
name one path.

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 310 packages placed; 77 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — **down from 78** |
| `go run ./tools/deadcodecheck` | OK — 28 public SDK findings and 1 cross-package test seam allowlisted |
| `staticcheck ./...` | clean |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** |

### The reach is P4-pure, and that was checked before it was built

Every contract package the registry names was checked for purity **before**
the move: all 23 of them import the standard library and other contract
packages only. So the move could not buy its edge by pulling a module in
through a contract package — the indirect leak that archcheck's direct-import
rule cannot see was measured, not assumed.

## What is deliberately left

Nothing of the registry. The package is in the contract layer, it names
nothing outside it, the plugin reaches it through the contract, and the last
`plugin-reach` edge is gone.

What the slice leaves is the next one: **W3's exit condition** is that
`kernel/runtime` and `kernel/controlplane` no longer exist and no type has
more than 60 methods. The registry was the last thing standing between the
built-in tools and the contract layer; what stands between the daemon and the
contract layer now is the kernel itself.
