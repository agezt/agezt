# W3.65 — the conversation collapse is the contract's

## What the slice is

W3.64 left the ratchet at **12/3**. This slice takes **two** import edges at
once and the ratchet goes **12 → 10**:

```
adapter-bypass kernel/openaiapi -> kernel/convo
adapter-bypass kernel/webui     -> kernel/convo
```

The forbidden-call ratchet is unchanged at **3** — the three sites that remain
are the ones the measurement already showed to be atomic by another means (a
staging directory swapped in as a whole).

The edge was **measured first**, against all twelve remaining import edges,
and the measurement picked this one: it is the only candidate that removes
**two** ratchet lines with the smallest possible port — **one row and one pure
function**, no publisher, no clock seam, no file, no rule change.

## What the package was

`kernel/convo` was **two symbols** in one file:

```go
type Turn struct {
	Role string
	Text string
}

func TranscriptIntent(turns []Turn) string
```

Its only import was `strings`. It had no store, no file, no bus, no model and
no clock — the same measurement that closed `kernel/modules/work/assure` in
W3.64 applies here: **a package that holds no state is not a module**. It sat
at layer 3 (module `runs`) only because that is where it was put.

And the two adapters that needed it are exactly the case the contract layer
exists for: an **adapter may reach the contract and the platform layers and
nothing else**. The OpenAI-compatible API and the Web UI Chat view had to
reach *into a layer-3 package* to render a conversation — the
`adapter-bypass` the ratchet was counting.

## What changed

### The row and the rule moved

`kernel/convo/convo.go` → `kernel/contract/convoapi/convoapi.go`. The body is
unchanged, line for line: `Turn`, `TranscriptIntent`. The rule's contract is
itself in the contract layer — *"Rows, constants and pure functions only. No
store, no file, no clock"* — and the rule is a pure function.

### The package is deleted

`kernel/convo/` — two files (`convo.go`, `convo_test.go`) — is gone, and so
is its hand-written layer rule in `tools/archcheck/layers.json`. The package
count stays at **314** (one package deleted, one added).

The package is deleted rather than kept as an alias house, and that is the
W3.64 lesson applied: a package left holding only a name for something else
is a package with no reason to exist, and Go has no alias for a function
anyway — a delegating wrapper would have been a second spelling of one rule.

### The callers name the contract

| File | Before | After |
|---|---|---|
| `kernel/openaiapi/openaiapi_helpers.go` | `convo.Turn`, `convo.TranscriptIntent` | `convoapi.*` |
| `kernel/webui/webui_security_helpers.go` | `convo.Turn` | `convoapi.Turn` |
| `kernel/webui/webui_security.go` | `convo.Turn`, `convo.TranscriptIntent` | `convoapi.*` |
| `kernel/app/chat/summary.go` | `convo.Turn`, `convo.TranscriptIntent` | `convoapi.*` |

`kernel/app/chat` was not an allowlisted edge (app → module is legal), but it
now names the contract too: there is **one** place that knows how a
conversation is collapsed, and every surface reaches it there.

**No behaviour changed.** The rule's body is byte-identical, no exported
signature names the row (every use is inside a function body), and no wire
shape moved: `Turn` has no JSON tags — it is not a wire row.

## The proof

### Permanent tests

`kernel/contract/convoapi/convoapi_test.go` — **nine** tests: the six that
travelled with the rule, and **three new** ones that the mutation run
demanded.

| Test | What it holds |
|---|---|
| `TestTranscriptIntent_SingleUserVerbatim` | a lone user turn passes through verbatim |
| `TestTranscriptIntent_MultiTurnTranscript` | a conversation becomes a labelled transcript |
| `TestTranscriptIntent_SystemHoisted` | system guidance goes to the front |
| `TestTranscriptIntent_SkipsEmptyTurns` | a blank turn is dropped |
| `TestTranscriptIntent_UnknownRoleVerbatimLine` | an unknown role is included verbatim |
| `TestTranscriptIntent_Empty` | no turns, no intent |
| **`TestTranscriptIntent_HoistsTheDeveloperRole`** | "developer" is leading guidance, exactly like "system" |
| **`TestTranscriptIntent_OneUserAndOneAssistantIsATranscript`** | the verbatim shortcut is for a **lone** user turn — one user turn plus an assistant turn is still a transcript |
| **`TestTranscriptIntent_TheRoleIsCaseInsensitive`** | the role vocabulary is matched case-insensitively |

The last three are the slice's honest finding: the rule's **doc comment**
promised all three ("Role is case-insensitive", "system and developer are
leading guidance", "a lone user turn"), and **no test asserted any of them**.
A rule whose contract is only a comment is a contract that can drift.

### The mutation run

**10 mutations: 10 caught, 0 survived, 0 build errors, 0 skipped** — in **two**
independent passes, with **byte-identical** logs (compared case-sensitively).

| # | Mutation | Caught by |
|---|---|---|
| 1 | the **developer** role is not leading guidance | `TestTranscriptIntent_HoistsTheDeveloperRole` |
| 2 | the **tool** role is labelled as dialogue | `TestTranscriptIntent_UnknownRoleVerbatimLine` |
| 3 | the "User:" label is lowercased | `TestTranscriptIntent_MultiTurnTranscript` |
| 4 | the "Assistant:" label is abbreviated | `TestTranscriptIntent_MultiTurnTranscript` |
| 5 | the verbatim shortcut fires with **any** single user turn | `TestTranscriptIntent_OneUserAndOneAssistantIsATranscript` |
| 6 | a blank turn is not skipped | `TestTranscriptIntent_SkipsEmptyTurns` |
| 7 | the role is matched case-sensitively | `TestTranscriptIntent_TheRoleIsCaseInsensitive` |
| 8 | the hoisted guidance is not separated by a blank line | `TestTranscriptIntent_SystemHoisted` |
| 9 | the lone user turn is not captured | `TestTranscriptIntent_SingleUserVerbatim` |
| 10 | the transcript lines are joined with spaces | `TestTranscriptIntent_MultiTurnTranscript` |

The **eleventh** thing this slice moved is **gate-measured**: an adapter
reaching a module again is not something a test can see, because the bytes
would be the same. The ratchet is the measurement — it went from 12 to 10
import edges, and the two deleted lines are the two adapters.

### The harness's own bug, and what it taught

The **first** pass reported **8 caught, 2 skipped** — and the skips were the
**harness's** fault, twice over.

The first skip was an honest one: the mutation's anchor did not exist. The
harness looked for `b.WriteString("User: "+text)`, but the rule builds the
dialogue in a slice (`dialogue = append(dialogue, "User: "+text)`) and joins
it later. The **SKIP** class did its job: an anchor that does not hold is
reported, never counted as a catch.

The second skip was the interesting one. The mutation that lowercases the
label (`"User: "` → `"user: "`) was reported as **SKIP** — "anchor not
found" — although the anchor was there. The harness compared the mutated
file with the original using PowerShell's `-eq`, and **`-eq` is
case-insensitive**: a mutation that changes *only* the casing of a string
looked like no change at all. The harness was blind to exactly the mutation
it was built to see.

The fix is one operator: `-ceq` instead of `-eq`. And the rule is worth
naming for the whole series: **a comparison that is not case-sensitive
cannot measure a case-sensitive change** — the same way a `-run` filter that
matches nothing cannot measure a test.

## The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 314 packages placed; 10 allowlisted import violations and 3 allowlisted call sites remain (ratchet)** — **import edges down from 12** |
| `go run ./tools/deadcodecheck` | OK — 31 public SDK findings and 1 cross-package test seam allowlisted |
| `go run ./tools/depscheck` | OK — 24 core dependencies, all justified |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean (207 packages in the codemap) |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `staticcheck ./...` | clean |
| `go test ./... -count=1` | **all green** |

## What remains

The ratchet stands at **10/3**. The ten import edges are:

| Kind | Edges |
|---|---|
| `adapter-bypass` | `controlplane → modules/extensions/toolbox`, `controlplane → runtime`, `controlplane → update`, `restapi → update` |
| `cross-module` | `cadence/systemtasks → runtime`, `modules/agents/selfrepair → runtime`, `pulse → agent` |
| `upward` | `channel → bus`, `runtime → agentgw`, `runtime/runexec → agentgw` |

The measurement that picked this slice also named the **trapped** ones, and
that is as valuable as the candidate:

- **`controlplane → runtime`** is trapped: the control plane holds the
  kernel's concrete `*runtime.Kernel` and calls **37** verbs on it. Closing it
  is a redesign — the control plane must reach the kernel through app
  operations, not through the kernel's god-object.
- **`modules/agents/selfrepair → runtime`** is trapped: the six agent rows
  could move to the reaper's `api` package, but `RunWith`, `RunWithRetry`,
  `ReaperScan` and `NewCorrelation` need a **runs port that does not exist**
  yet — that is a slice of its own, before the edge can be closed.
- **`channel → bus`** is genuinely trapped by a **typed-nil** trap: 26 plugin
  `Config`s hold `*bus.Bus` and nil-check it. An interface parameter turns a
  nil `*bus.Bus` into a **non-nil** interface, the guard stops firing, and the
  publish panics inside the deferred recover.
- **`pulse → agent`** is the smallest ready edge: one call site, one verb, and
  the project has already written the exact port — `memoryapi.Generator` is
  `agent.GenerateObject`'s signature. It needs a `Generator` field on
  `pulse.Config` and the daemon injecting it.
- **`runtime → agentgw`** and **`runtime/runexec → agentgw`** are free
  riders: `k.AgentGateway()` has **no caller** outside its own declaration.
  The fix is not a port but a **move** — the daemon builds the gateway — and
  the price is that the gateway's *wiring* is untested today.

## The rules the slice is really about

**An adapter that reaches a layer-3 package for a pure function is paying for
a layer it does not need.** The rule and the row were pure, and the contract
layer is the home of pure things. The adapters now name the contract, and the
ratchet — which is the architecture's own statement of what it still owes —
drops two lines.

**A rule's doc comment is not its contract.** Three claims in this rule's
comment ("case-insensitive", "developer is guidance", "a lone user turn")
were never asserted by a test. The mutation run found them in minutes. A
comment is a promise; only a test is a guarantee.

**A comparison that is not case-sensitive cannot measure a case-sensitive
change.** The harness's `-eq` made the label-casing mutation invisible. The
lesson generalises: every measuring tool has a blind spot, and the blind spot
is always exactly where the tool was not looking.
