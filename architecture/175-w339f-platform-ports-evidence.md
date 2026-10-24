# W3.39f — the platform rows become the contract's ports

## The last family that blocked the final move

The registry's rows (`Spec`, `Built`, `BuildDeps`, `KernelDeps`, `LateDeps` and
the registry itself) live in `kernel/toolreg` — a **module** (L3). The plugin
that registers the first-party tools (`plugins/builtintools`) is L6, and a
plugin may reach the contract and the platform, and **nothing else**. That is
the one remaining `plugin-reach` violation in the whole repository:

```
plugin-reach plugins/builtintools -> kernel/toolreg
```

It falls the moment the registry's rows live where a plugin may name them,
and that needs **every field of every row** to be a contract row or a contract
port. Five families stood in the way. Four fell in W3.39b–W3.39e:

| Slice | Family | What moved |
|---|---|---|
| W3.39b | reasoning | `reasoningapi.{Council,Conductor,Research}Runner` |
| W3.39c | workflow + workboard | `workflowapi.Runner`, `workboardapi.Runner` |
| W3.39d | MCP + standing | `mcpapi.Runner`, `standingapi.Host` |
| W3.39e | schedule + skill | `cadenceapi.Store`, `skillapi.Forge`, `skillapi.BundleStore` |
| **W3.39f** | **the platform rows** | **`wardenapi.Engine`, `eventapi.Publisher`, `journalapi.Reader`** |

The fifth family was the hard one, because these three rows did not merely
*name* a platform type — they named one that carries rows the contract layer
is not allowed to name at all.

## What moved

### The warden — `kernel/contract/wardenapi`

The sandbox's four rows (`Profile`, `Limits`, `Spec`, `Result`) moved to the
contract layer **verbatim** — the same field names, the same order, the same
doc comments — and `kernel/warden` keeps every name its own callers use as an
**alias**:

```go
type (
	Profile = wardenapi.Profile
	Limits  = wardenapi.Limits
	Spec    = wardenapi.Spec
	Result  = wardenapi.Result
)
```

So there is one row and one spelling of it. `warden.Profile.IsKnown()` moved
with the type (a method on an aliased type must live in the package that
declares it).

The **port** is deliberately narrower than the platform's engine:

```go
type Engine interface {
	Run(ctx context.Context, spec Spec) (*Result, error)
	EffectiveProfile(p Profile) Profile
}
```

The platform's engine carries a third verb — `SetBus(*bus.Bus)` — and that verb
names the **bus**, a row the contract layer may not import (archcheck's
principle P4: a package under `kernel/contract/` may import the standard
library and other contract packages only). The platform's engine now embeds
the contract's port and keeps `SetBus` for itself; the tools (the shell tool
and the code_exec tool — both plugins) take the contract's port.

### The bus — `kernel/contract/eventapi`

The event's **kind** and the event's **spec** are contract rows now, and the
bus's publish side is a port:

```go
type Kind string
type Spec struct { Subject string; Kind Kind; Actor string; CorrelationID string; CausationID string; Payload any; Tags map[string]string }
type Publisher interface { Publish(spec Spec) error }
```

`kernel/event` keeps both names as aliases:

```go
type Kind = eventapi.Kind
type Spec = eventapi.Spec
```

**This is the part worth pausing on.** Because the platform's spec is an
*alias* of the contract's spec, the bus's own method
`Publish(spec event.Spec) (*event.Event, error)` takes **the contract's row** —
the two are the same type, not two shapes kept in step by hand. The daemon's
adapter therefore translates **nothing**: what the code_exec tool publishes is
exactly what the bus writes. The wire is preserved by construction, not by a
copy that has to be checked.

The kind's **vocabulary** (`event.KindCodeExecuted` and the rest) stays in
`kernel/event`, which owns the event registry and the guard test over it
(`registry_guard_test.go`). Only the *type* moved — and the guard test reads
`kernel/event/kinds.go` exactly as before, so it was not weakened.

That last sentence is a claim, so it was **measured**: an unused kind added to
`kernel/event/kinds.go` is still caught by the guard test (the probe was
reverted immediately after):

```
registry_guard_test.go:119: KindGuardProbeUnused is declared but never emitted
or consumed — delete it, or add it to reservedKinds with a reason
```

Moving the whole vocabulary into the contract would have **silently** broken
it: the guard's declaration pattern (`^\s*(Kind\w+)\s+Kind\s*=`) does not match
alias lines, so the test would have found **zero** declarations and passed
vacuously. Moving only the *type* keeps the vocabulary, the guard and the
guard's input file exactly where they were.

The port answers **one** thing — *did it land?* — where the bus answers two
(the event it wrote, and the error). The durable event carries the hash chain,
and the contract layer may not name a BLAKE3 digest (P4), so the publisher asks
the only question it can ask without it.

### The journal — `kernel/contract/journalapi`

The journal's read side is a port and a **projection**:

```go
type Record struct {
	Kind          eventapi.Kind
	CorrelationID string
	TSUnixMS      int64
	Payload       json.RawMessage
}
type Reader interface { Tail(n int) ([]Record, error) }
```

The projection is four fields, and they are exactly the four the runs tool
folds: which kind of thing happened, which run it belongs to, when, and what
it carried. The durable envelope's own fields — the hash chain, the sequence,
the subject — belong to the journal, not to the reader, and the envelope
cannot be a contract row anyway (its `VerifyHash` needs BLAKE3).

The **adapter** lives at the composition root (`cmd/agezt/tooldeps.go`): the
journal hands back the durable event, the port hands back the projection, and
the copy is field by field — and **measured** (see below).

### The nil-port rule

Both new ports keep the rule the other ports follow, and the check is on the
**concrete** type on purpose — a nil `*bus.Bus` handed to an interface
parameter is already a **non-nil** interface:

```go
func busPort(b *bus.Bus) eventapi.Publisher {
	if b == nil {
		return nil
	}
	return busPublisher{b: b}
}
```

A daemon without a bus hands over **nothing** — the code_exec tool then
publishes no event and says so, instead of binding a publisher that is not
there and panicking on the first run.

## The ratchet did NOT move — for the sixth time

**78/13**, unchanged, and that is the point of the slice, not an omission.
The registry's **rows** are still in `kernel/toolreg` (L3), and the plugin
still imports it. The last `plugin-reach` edge falls only when the registry
itself moves down to the contract layer — W3.39g.

What this slice *does* change is the shape of the blocker. After it, **every
field of every row of the registry is a contract row or a contract port, with
exactly one exception**: `KernelDeps.K` — the live kernel itself. The registry
imports `kernel/channel` and `kernel/runtime` and **no platform package at
all**.

So the list of what W3.39g has to solve is now short and exact:

1. `d.K` is used in three places: the forge tool's runner, the kernel's code
   executor, and the artifact bytes the send_media tool resolves.
2. The registry's own functions (`Register`, `Names`, `BuildAll`, `Set`) move
   to the contract layer with the rows.

## The finding: a rule nobody measured

The mutation run produced a **survivor**: *"the journal read port swallows the
read error"* changed nothing.

It was not a weak mutation but a **real rule with no measurement**. The
adapter's error path (`return nil, err`) had no test walking it — every test
ran a journal that could be read. And the rule matters: a port that answered
"nothing found" for a read that **failed** would tell the operator that the
daemon has no past runs, when in truth the history could not be read at all.
That is the difference between *"there is nothing"* and *"I could not look"*,
and it is the whole point of the port.

The fix was **a test, not a weaker mutation**: the daemon's test now puts a
segment the journal cannot decode into its directory and asserts that the port
**reports** the failure. The tool side of the same rule was already measured
(`coverage_more_test.go` binds a history that fails and asserts the tool says
`read journal`, not `no runs`).

The second mutation that did not measure was re-authored: the first version of
*"the runs tool no longer marks a finished run as completed"* replaced the
case's constant with one the switch already had, and the result was a
**compile error** — a duplicate case — not a measured failure. A compile error
is not a catch. The mutation was re-written to point at a kind the switch does
not carry (`task.abandoned`), and the runs test then caught it: the finished
run stayed `running`.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `kernel/contract/wardenapi/wardenapi_test.go` | the port carries **exactly two** verbs (Run, EffectiveProfile) and not the bus-naming third; the four profile constants and nothing else; the three rows keep their fields, in order |
| `kernel/contract/eventapi/eventapi_test.go` | the publish port carries **exactly one** verb and answers an error; the spec keeps its seven fields; the kind is a string row |
| `kernel/contract/journalapi/journalapi_test.go` | the read port carries **exactly one** verb and answers `([]Record, error)`; the record keeps its four fields |
| `cmd/agezt/platform_ports_test.go` | the **nil-port rule** on both new ports (checked on the concrete type); the **publish wire** (subject, kind, actor, correlation, causation, tags, payload — field by field, through the journal); the **read wire** (kind, correlation, timestamp, payload); a refused publish is **reported**; a failed read is **reported**; the daemon supplies both ports and they are wired to the **same live daemon** |
| `plugins/builtintools/platform_ports_test.go` | the shell spec carries the daemon's warden engine **through the port** — unchanged |
| `plugins/tools/codeexec/publish_ports_test.go` | the tool publishes the code.executed event with its subject, kind, actor and payload; with no port it publishes **nothing** and does not crash |
| `plugins/tools/runstool/*_test.go` (re-pointed) | the fold now reads the contract's record — what they measure and what they assert are **unchanged** |

### Mutation run

**18 mutations, 18 caught** — 0 survived, 0 skipped, 0 build errors.

| # | Mutation | Caught by |
|---|---|---|
| 1 | the journal read port drops the correlation | the read wire |
| 2 | the journal read port drops the timestamp | the read wire |
| 3 | the journal read port drops the payload | the read wire |
| 4 | the journal read port rewrites the kind | the read wire |
| 5 | the journal read port swallows the read error | the failed-read test (added for this) |
| 6 | the publish port stops being nil-safe | the nil-port test |
| 7 | the journal read port stops being nil-safe | the nil-port test |
| 8 | the publish port swallows the write error | the refused-publish test |
| 9 | the publish port is not supplied to the registry | the daemon's wiring test |
| 10 | the journal read port is not supplied to the registry | the daemon's wiring test |
| 11 | code_exec publishes the wrong kind | the tool's publish test |
| 12 | code_exec publishes no payload | the tool's publish test |
| 13 | code_exec publishes without checking the port | the tool's no-port test (panic) |
| 14 | the shell spec drops the warden engine | the plugin's warden test |
| 15 | the warden port grows a third verb | the port's shape test |
| 16 | the journal read port grows a second verb | the port's shape test |
| 17 | the runs tool no longer marks a finished run as completed | the runs test (re-authored: the first version was a duplicate case — a compile error, not a catch) |
| 18 | **gate**: the contract names the platform again (a helper in `wardenapi` taking a `*bus.Bus`) | **archcheck** — the contract layer's purity (P4) |

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 310 packages placed; 78 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — unchanged, as designed |
| `go run ./tools/deadcodecheck` | OK — 28 public SDK findings and 1 cross-package test seam allowlisted |
| `staticcheck ./...` | clean (SC=0) |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** |

## What is deliberately left

`KernelDeps.K` — the live kernel. Three uses, each with its own shape:

1. the forge tool's runner (four verbs, one of them answering an approval
   decision — a row that would have to move to the contract too);
2. the kernel's code executor (`runtime.CodeExecutor`, a port that names only
   stdlib);
3. the artifact bytes the send_media tool resolves (`artifactapi.Index`
   already carries `Bytes`; the tool needs one more verb).

That is W3.39g, and with it the registry's own functions move to the contract
layer — and the last `plugin-reach` edge falls.
