# W3.64 — the assurance loop lives in the contract, and the module is gone

## What the slice is

W3.63 left the ratchet at **13/3**. This slice is not a call-site slice: it
takes the **last** import edge that pointed *into* a module from the kernel,
and the ratchet goes **13 → 12** import edges. The forbidden-call ratchet is
unchanged at **3** — the three sites that remain are the ones the measurement
already showed to be atomic by another means (a staging directory swapped in
as a whole).

The edge was:

```
cross-module kernel/runtime/runexec -> kernel/modules/work/assure
```

`kernel/runtime/runexec` is the run engine — the module `runs`. The
`kernel/modules/work/assure` package was the **only** module implementation
the kernel reached.

## The measurement that decided the slice

The edge was measured before it was touched, and the measurement said the
package **was never a module**:

| Question | Answer |
|---|---|
| Does it hold a **store**? | no — no file, no database, no directory |
| Does it hold a **bus**? | no — it publishes nothing |
| Does it hold a **model**? | no — it calls no provider |
| Does it hold a **clock**? | no — it takes a budget, not a time |
| What does it hold? | two closures (`RunFn`, `VerifyFn`), a budget, and a loop |
| What did the contract already hold? | `DefaultMaxAttempts`, `MaxMaxAttempts`, `Verdict`, `Attempt`, `Result`, `ParseVerdict` (W3.31) |
| Who used it? | the run engine (3 files), `cmd/agt` (one constant), two test files |

A bounded context is a **bounded** thing: a domain with its own state, its own
effects and its own vocabulary. This package had none of that — it was a
**pure function of two closures**. A module that holds no state cannot be
given a store later without changing what it is; it was a package in the
module tree because that is where it was put.

And the contract layer's own rule already covered it: *"Rows, constants and
pure functions only. No store, no file, no clock."* The loop is a pure
function. The contract layer already holds pure engines of exactly this
shape — `contract/reasoningapi`'s runner, `contract/workflowapi`'s runner,
`contract/cadenceapi`'s engine.

## The decision

Two shapes were possible.

**(a) Keep the module as an alias and delegation house.** The rows stay
aliases, `ParseVerdict` stays a delegation, and the loop stays put. This was
**rejected**, for three measured reasons:

1. It reverses a **documented decision** (W3.31: *"the loop stays in the
   module"*) without the loop's nature changing — and the reason W3.31 gave
   was *"the loop here is the implementation, not a row"*, which was true then
   and is false now that the implementation is also pure.
2. The package would become a **pass-through**: the loop is the only thing in
   it that is not a row, and the rows are aliases. A delegation that exists
   only to be delegated to is the shape the dead-code gate has to be taught to
   ignore — "a cross-package test seam" — which is how a package survives
   with no production reason to exist.
3. The kernel would still reach a module for a pure function. The edge would
   stay in the ratchet forever, and the ratchet is the **statement** of what
   the architecture still owes.

**(b) Move the loop into the contract and delete the module.** The rows and
the parser are already there (W3.31); the loop joins them. The module
package disappears, and so does the edge.

**(b) was taken.**

## What changed

### The loop moved

`kernel/modules/work/assure/assure.go` → `kernel/contract/assureapi/loop.go`.

The loop's body is **unchanged**, line for line: `RunFn`, `VerifyFn`,
`clampAttempts`, `Until`, `retryInstruction`. What moved is the question it
answers to:

```go
// before — the module held the loop, the contract held the rows
res, err := assure.Until(ctx, intent, maxAttempts, run, verify)

// after — one package holds both, so there is one place that knows how a
// task is assured
res, err := assureapi.Until(ctx, intent, maxAttempts, run, verify)
```

The module's **aliases and delegations are gone** — that is the point. There
is now one spelling of `Verdict`, one spelling of `ParseVerdict` and one
implementation of the loop, and none of them is a second name for the same
thing.

### The module is deleted

`kernel/modules/work/assure/` — four files (`assure.go`, `assure_test.go`,
`contract_type_test.go`, `parse_verdict_test.go`) — is gone. The package
count drops from 315 to **314**.

### The consumers name the contract

| File | Before | After |
|---|---|---|
| `kernel/runtime/runexec/api.go` | `assure.Result`, `assure.Verdict` | `assureapi.Result`, `assureapi.Verdict` |
| `kernel/runtime/runexec/runner_lifecycle.go` | `assure.Until`, `assure.Verdict`, `assure.Result` | `assureapi.*` |
| `kernel/runtime/runexec/runner_retry.go` | `assure.Verdict`, `assure.ParseVerdict` | `assureapi.*` |
| `cmd/agt/main.go` | `assure.DefaultMaxAttempts` | `assureapi.DefaultMaxAttempts` |
| `kernel/app/workboard/workboard_test.go` | `assure.Verdict` | `assureapi.Verdict` |
| `kernel/modules/work/workboard/proof_gate_test.go` | `assure.Verdict` | `assureapi.Verdict` |

**No signature changed.** `KernelAPI.RunAssured` returns
`(string, assureapi.Result, error)` — and it returned `(string, assure.Result,
error)` before, where `assure.Result` was a **type alias** for
`assureapi.Result`. An alias is the same type, so the interface, the callers
and the wire are byte-for-byte identical. The same is true of the module's
`ParseVerdict`, which delegated to the contract's.

### The ratchet

`tools/archcheck/allowlist.txt` loses the line:

```
cross-module kernel/runtime/runexec -> kernel/modules/work/assure
```

**13 → 12** import edges. The forbidden-call ratchet stays at **3**.

## The milestone this slice closes

After W3.64 the kernel reaches **no module implementation at all**. Measured
over every non-test file in `kernel/runtime`:

```
agents/api
reasoning/api
```

Two `api` sub-packages, and **nothing else**. An `api` package is the one
import a module is *allowed* to make across module boundaries (archcheck's
P5: *"a module may import another module's api package and nothing else"*) —
it is the vocabulary, not the implementation. So the kernel's reach into
`kernel/modules/` is now **zero implementations**: the run engine asks for
rows and ports, and every implementation lives behind the kernel's own API.

That is the whole point of the W3 series for this direction: the kernel is the
**host**, and a host that reaches into a guest's internals is a host that
cannot be reused.

## The proof

### Permanent tests

`kernel/contract/assureapi/loop_test.go` (nine tests) — the seven loop tests
moved with the loop, plus two new ones:

| Test | What it holds |
|---|---|
| `TestUntil_CompletesFirstTry` | the loop stops the moment a verdict says complete |
| `TestUntil_RetriesThenCompletes` | it retries, and the retry carries **both** the original task and the gap |
| `TestUntil_ExhaustsWithoutCompleting` | the budget is spent, the **last** gap is reported, the last answer is returned |
| `TestUntil_RunErrorAborts` | a run error aborts the loop (it is usually persistent) |
| `TestUntil_VerifyErrorReturnsProgress` | a verify error surfaces, and the produced answer is still returned |
| `TestUntil_ClampsAttempts` | `0` → the default budget, a huge value → the ceiling |
| `TestVerifyAgainstOriginalTask` | the verifier always sees the **ORIGINAL** task, never the gap-augmented one |
| **`TestTheAttemptNumberIsOneBased`** | the run closure is told which attempt it is, and the count starts at **one** |
| **`TestTheHistoryCarriesEachAttempt`** | the trail carries every attempt's number, answer and verdict |

`kernel/contract/assureapi/parse_verdict_test.go` (three tests) — the
malformed-object, fence-only and fence-without-JSON cases moved with the
parser's contract.

Two tests are **new**, and the first one exists because of the mutation run:
the attempt number was never asserted, so a loop that numbered its attempts
from zero would have passed every test in the package. The second pins the
history — the trail is the evidence of what the loop tried, and `agt why`
reads it.

The contract package now holds **20** tests: the eight it already had (the
wire shapes, the budget, the parser), the nine above, and the three parse
cases.

**One test was deleted, and it is worth naming.**
`kernel/modules/work/assure/contract_type_test.go` asserted that the
module's rows *are* the contract's rows (by reflection) and that the module's
parser *delegates* to the contract's. Its subject is gone: there is no second
spelling any more. That assertion is now **structural** — the compiler
enforces it, because there is only one type.

### The mutation run

**10 mutations: 10 caught, 0 survived, 0 build errors, 0 skipped** — in **two**
independent passes, with **byte-identical** logs (compared case-sensitively).

| # | Mutation | Caught by |
|---|---|---|
| 1 | the budget is off by one (`i < maxAttempts`) | `TestUntil_ClampsAttempts` |
| 2 | the retry keeps the bare task (no gap feedback) | `TestUntil_RetriesThenCompletes` |
| 3 | verification runs against the **augmented** task | `TestVerifyAgainstOriginalTask` |
| 4 | the unmet gap is not recorded in the result | `TestUntil_ExhaustsWithoutCompleting` |
| 5 | the attempt ceiling is not enforced | `TestUntil_ClampsAttempts` |
| 6 | the per-attempt history is not recorded | `TestUntil_RetriesThenCompletes` |
| 7 | a run error does not abort the loop | `TestUntil_RunErrorAborts` |
| 8 | completion is never claimed | `TestUntil_CompletesFirstTry` |
| 9 | the attempt number is zero-based | `TestTheAttemptNumberIsOneBased` |
| 10 | the retry instruction omits the gap text | `TestUntil_RetriesThenCompletes` |

Mutations 3 and 10 are the ones that name the loop's real contract: the
verdict must judge the **original** task (a gap-augmented task is a different
task, and "complete" would mean something else), and the gap must reach the
next attempt (a retry that does not know what was missing is a blind retry).

The harness classified every outcome into **CAUGHT / SURVIVED / BUILDERR /
NO-TESTS** separately, and the package and the `-run` filter were passed as
**separate arguments** — a `go test -run` whose name matches nothing answers
`no tests to run` and exits **0**, which would have been scored as a
survival that never happened.

### The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 314 packages placed; 12 allowlisted import violations and 3 allowlisted call sites remain (ratchet)** — **import edges down from 13** |
| `go run ./tools/deadcodecheck` | OK — 31 public SDK findings and 1 cross-package test seam allowlisted |
| `go run ./tools/depscheck` | OK — 24 core dependencies, all justified |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean (207 packages in the codemap) |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `staticcheck ./...` | clean |
| `go test ./... -count=1` | **all green** |

## What remains

The ratchet stands at **12/3**. The twelve import edges are:

| Kind | Edges |
|---|---|
| `adapter-bypass` | `controlplane → modules/extensions/toolbox`, `controlplane → runtime`, `controlplane → update`, `openaiapi → convo`, `restapi → update`, `webui → convo` |
| `cross-module` | `cadence/systemtasks → runtime`, `modules/agents/selfrepair → runtime`, `pulse → agent` |
| `upward` | `channel → bus`, `runtime → agentgw`, `runtime/runexec → agentgw` |

None of them is a kernel reaching into a module's **implementation** any more.
The `cross-module` edges left are *module → module* (a trigger reaches the
runner's runtime, the self-repair module reaches it, the pulse module reaches
the agent) and the `upward` edges are the bus and the gateway.

The **three** forbidden-call sites are the market's sync cache (2) and the
skill bundle (1) — both written into a **staging directory** that is swapped
in as a whole, so they are already atomic by another means. The roadmap's
**W5.4** wants the allowlist at **zero**, so they will be taken in a slice
that makes the swap visible to the gate.

And the ratchet's **blind spot** is untouched: `cmd/agt/market_publish.go`
writes an **Ed25519 private key** in place, `cmd/...` is layer **L7** (root)
and the root layer is **exempt** from the forbidden-call rules — and
`cmd/agt` has **no test** that calls `cmdMarketKeygen` or `cmdMarketPublish`
at all. Closing it needs the **rule** to change before the site can be
fixed.

## The rules the slice is really about

**A package that holds no state is not a module.** A module is a bounded
context: it owns state, effects and a vocabulary. A package that owns none of
them is a **function**, and a function that the host has to reach *into* a
directory to call is a function in the wrong place. The `work` module's tree
was the wrong place for a pure loop, and the cost was not aesthetic — it was
a permanent line in the architecture's debt list.

**The contract layer is not only rows.** The rule that governs it is *"rows,
constants and pure functions only"* — and the loop is a pure function. The
contract already holds pure engines (`reasoningapi`'s runner,
`workflowapi`'s runner, `cadenceapi`'s engine), so the loop has company, and
the rule was never violated by moving it there.

**A delegation that exists only to be delegated to is not an architecture.**
The alternative — keeping the module as an alias house — would have kept the
edge in the ratchet forever and taught the dead-code gate to forgive a
package with no reason to exist. The rule is simple: if the only thing left
in a package is a name for something else, the package is over.

**A deleted test is a finding.** `contract_type_test.go` asserted that two
spellings are the same type. After the slice there is one spelling, and the
compiler says so. The test did not get weaker — its subject disappeared.
