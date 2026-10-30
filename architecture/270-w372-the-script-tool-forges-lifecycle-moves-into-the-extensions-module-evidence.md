# W3.72 — the script-tool forge's lifecycle and the agent bridge move into the extensions module

## What the slice is

The **fifth** slice of the runtime's dissolution, and the second one to move a
family whose **data** was already in a module while its **operations** were in
the daemon.

The script-tool forge was split in half. Its **catalogue** — the durable
records, the store that keeps them, the rules that decide whether a script
may be drafted and whether it may go live — already lived in the extensions
module (`kernel/modules/extensions/toolforge`). Its **operations** lived in
the daemon (`kernel/runtime/scripttool.go`, 304 lines): the seven lifecycle
verbs that draft, edit, test, promote, ask the operator, quarantine and
remove a script, the sandbox test, the operator's queue — and the **bridge**
that folds the promoted scripts into a run's tool map as `forge_<name>` tools.

A module that keeps the catalogue and leaves the lifecycle in the daemon keeps
the data and gives away the meaning of the data. The target architecture says
so in one line: the extensions module owns the **toolforge**.

## What the slice found

The candidate was measured before it was chosen:

- the daemon's forge file: **304 lines**;
- the daemon's forge verbs: **8 public names** (`ToolForge`, `DraftScriptTool`,
  `UpdateScriptTool`, `TestScriptTool`, `PromoteScriptTool`,
  `RequestToolPromotion`, `QuarantineScriptTool`, `RemoveScriptTool`);
- the daemon's calls into the catalogue's store: **8**;
- the module's calls into the catalogue's store: **0**.

And the shape cut **both** ways, the same way it did for the MCP registry
(W3.71):

- the **lifecycle** and the **bridge** answer ONE question — which tools may
  this run call — and they read the same ACTIVE rows. So they moved
  **together**, as one service.
- the **read** (`ToolForge`) is the daemon's nil-safe view over the
  **contract's port** (W3.45) — the shape the MCP registry established. It
  stayed with the host, which is what opens the catalogue's file.

**A family splits along the line between the port and the operations** — the
rule W3.71 stated, now confirmed on a second family.

## What changed

### The module owns the lifecycle and the bridge

`kernel/modules/extensions/toolforge/forge.go` is the service: the seven
lifecycle verbs, the sandbox test, the operator's queue and the six journaled
transitions.

`kernel/modules/extensions/toolforge/bridge.go` is the bridge: the merge, the
forged tool adapter and the callable-name rule.

### The module's face grows a second service

```go
// kernel/modules/extensions/api/forge.go
type ForgeService interface {
	Draft(corr string, st toolforgeapi.ScriptTool) (toolforgeapi.ScriptTool, error)
	Update(corr, ref string, mutate func(*toolforgeapi.ScriptTool)) (toolforgeapi.ScriptTool, bool, error)
	Test(ctx context.Context, corr, ref, input string) (toolforgeapi.ScriptTool, string, error)
	Promote(corr, ref string) (toolforgeapi.ScriptTool, error)
	RequestPromotion(ctx context.Context, corr, ref string) (toolforgeapi.ScriptTool, approvalapi.Decision, string, error)
	Quarantine(corr, ref, reason string) (toolforgeapi.ScriptTool, error)
	Remove(corr, ref string) (bool, error)
	MergeTools(tools map[string]toolapi.Tool) map[string]toolapi.Tool
}

type ForgeDependencies struct {
	Store        toolforgeapi.HostStore
	ScriptRunner toolregapi.ScriptRunner
	AutoPromote  bool
	Approve      func(ctx context.Context, spec approvalapi.SubmitSpec) approvalapi.Outcome
	Publish      func(spec event.Spec)
}
```

The ports are the host's: the catalogue's store (the contract's port), the
sandbox runner (a contract seam since W3.45), the operator's approval registry
and the bus to publish on. Every port is a way to **ASK**; none is a way to
reach.

### The daemon keeps its names and forwards

```go
// kernel/runtime/scripttool.go
func (k *Kernel) DraftScriptTool(corr string, st toolforgeapi.ScriptTool) (toolforgeapi.ScriptTool, error) {
	svc, err := k.forgeOrRefusal()
	if err != nil {
		return toolforgeapi.ScriptTool{}, err
	}
	return svc.Draft(corr, st)
}
```

**No caller outside the runtime changed**: **0 production files** in
`kernel/app` changed, and the plugin (`plugins/tools/forgetool`), the app's
lifecycle surface, the marketplace and the control plane all keep speaking
the daemon's eight names.

### The wiring is a builder, not a setter

The daemon hands the kernel the module's **BUILDER** (`cfg.ForgeBuilder`),
and the kernel calls it with the store, the sandbox runner, the operator's
approval registry and its own publisher at `Open` — the W3.57 shape, and the
right one here: the forge's store and the sandbox runner were already **cfg
seams** (W3.45).

### What the slice cost

**Nine test setups** had to learn the builder: the runtime's two forge
setups, the daemon's own forge helper, the application layer's forge ports
test, the control plane's shared helper and its three forge tests, and the
built-in tools' forge test. The **daemon itself cost nothing** — `cmd/agezt`
wires its own builder by default.

The rule the slices now measure, twice over:

- a move into an **existing** face costs **nothing** (W3.70: 0 sites);
- a move that invents a **new** face costs a wiring in every host that has one
  (W3.68: 11, W3.69: 6, W3.71: 6, W3.72: 9).

## The rule

The daemon holds the module's **face**; the module holds the module's
**body**.

The rule is held by the rule's own observer: a fake forge records what the
daemon asks. A daemon that grew the lifecycle back would answer on its own
and the fake would record nothing.

## What did NOT move, and why

- **The catalogue's READ stayed in the daemon.** It is the nil-safe view over
  the **contract's port** — the shape W3.45 established — and the host is
  what opens the catalogue's file.
- **The sandbox runner stayed the host's seam.** The implementation lives in
  the platform, but the host injects it (`toolregapi.ScriptRunner`), so the
  module never names it, and a host with no runner refuses the test with a
  readable error instead of calling a nil func value.
- **The event's shape did not change.** The six kinds (`scripttool.created`,
  `scripttool.updated`, `scripttool.tested`, `scripttool.promoted`,
  `scripttool.quarantined`, `scripttool.removed`), the subject
  (`toolforge.<name>`), the actor (`toolforge`) and the payload rows are the
  same ones the daemon published — only the place that speaks them moved.
- **The rules did not change.** Only TESTED code goes live, any edit demotes
  the tool back to draft with its test record cleared, and the OPERATOR — not
  the agent — grants a promotion. The store keeps those rules; the module now
  keeps the flow around them.

## The contract row

The refusal became a **contract row** (`toolforgeapi.ErrNoStore`), because two
sides answer it and a caller compares it with `errors.Is`: the **module**
raises it when the daemon handed over no catalogue, and the **daemon** raises
it when the module was never wired. Two look-alike errors of their own would
leave every comparison silently false — the same reason `mcpapi.ErrNoRegistry`
exists (W3.71).

The daemon keeps the name (`runtime.ErrNoForge`) as the row's alias, so every
existing comparison keeps working.

## One refusal that the slice added

The promotion queue asks the OPERATOR. The daemon's own approval registry is
always there today — so the old code called it unconditionally. The module
cannot: it holds a **port**, and a host with no approval registry must get a
plain refusal, not a panic. So the module gained one:

> `toolforge: promotions need an operator, and this daemon has no approval registry`

A refusal that did not exist before the move, because the move made the
missing port visible.

## The tests

**22 permanent tests**, in four places:

- `kernel/modules/extensions/toolforge/forge_test.go` — **13 tests**: the
  service refuses to write without a catalogue (the contract's row), the draft
  lands in the catalogue and journals `scripttool.created`, an edit demotes
  the tool and clears its test record, an edit of an unknown ref answers
  false and no error, the test runs the sandbox and records the verdict, a
  failing sandbox run is recorded as a failure, the test refuses without a
  sandbox, the promotion asks the operator and a grant goes live, a denied
  promotion is a **verdict and not a failure**, an untested tool never reaches
  the operator's queue, the auto-promote goes live without asking, the
  quarantine is the kill switch and journals the operator's reason, and the
  remove takes the row and journals it.
- `kernel/modules/extensions/toolforge/bridge_test.go` — **5 tests**: the
  merge leaves the map untouched without a promoted script, the merge offers
  the promoted script under its `forge_` name and forwards the call to the
  sandbox, a registered tool wins the collision, the callable name is the
  `forge_` prefix, and a failing script is an error **result**.
- `kernel/runtime/forge_module_test.go` — **3 tests**: the daemon refuses
  every forge verb when the module is not wired, the daemon forwards every
  forge verb to the module (a **fake forge** records the arguments), and the
  daemon keeps the eight names.
- `kernel/runtime/forge_merge_internal_test.go` — **1 test**: the daemon's
  tool merge is a forwarder — with no module the map comes back untouched,
  and with one the module's tool is in the merged map.

## The mutations

**14 mutations, 14 caught**, in two independent passes with **byte-identical**
logs.

SHA-256: `DF2C53DC2ACFC095259C0875CC80486E8274E2CCBB4F46B8E44BDC5EABBF0B1B`

The first calibration reported **one BUILDERR** — the mutation that recorded
a failing sandbox run as a pass left `isErr` unused, so Go refused to
compile. A compile error is not a caught mutation, it is a mutation that never
ran: the mutation now keeps `isErr` alive (`ok := !isErr || isErr`).

- **M01**, **M02** the daemon stops asking the module, or swallows the
  refusal — the **fake forge** and the **refusal** catch them.
- **M03** the module accepts a draft with no catalogue — the module's own
  refusal catches it.
- **M04**, **M05**, **M06** the module journals the draft as an edit, the
  edit as a draft, or the test as a draft — the **journaling** catches them.
- **M07** the module records a failing test as a pass — the **verdict**
  catches it.
- **M08**, **M09**, **M10** the module promotes a denied tool, lets an
  untested tool reach the operator, or asks the operator even when
  auto-promoting — the **promotion gate** catches them.
- **M11**, **M12** the module does not journal the quarantine's reason, or
  journals the removal as a draft — the **journal** catches them.
- **M13**, **M14** the bridge offers the script under the wrong name, or lets
  a forged script shadow a registered tool — the **bridge** catches them.

## The measurement that moved

| | before | after |
|---|---|---|
| the daemon's calls into the catalogue's store | **8** | **0** |
| the module's calls into the catalogue's store | **0** | **8** |
| the daemon's calls into the module | **0** | **8** |
| the daemon's public forge names | **8** | **8** |
| the kernel's verbs | **241** | **242** |
| the application layer's production files that changed | — | **0** |
| the test setups that had to be wired | — | **9** |
| the contract's new rows | — | **1** |
| `kernel/runtime` (lines) | **9,170** | **9,080** |
| `kernel/modules/extensions` (lines) | **5,234** | **5,687** |
| `kernel/modules/extensions/toolforge` (lines) | **352** | **690** |
| `kernel/contract/toolforgeapi` (lines) | **186** | **194** |
| the daemon's `scripttool.go` (lines) | **304** | **160** |
| the module's `forge.go` (lines) | — | **242** |
| the module's `bridge.go` (lines) | — | **96** |
| the module's `api/forge.go` (lines) | — | **115** |

The daemon's calls into the catalogue's store fell from **8** to **0** — every
one of them is the module's now. The daemon's calls into the module are
**8**: the seven lifecycle verbs and the merge.

The kernel's verb count moved by exactly **one**: `SetForge`, the seam that
lets a host swap the module's service. The **eight forge names did not move
at all** — they are the daemon's names and always were; what moved is the body
behind them.

The lines are counted the way the previous slices counted them: **non-blank**
lines of the package's production files (`git ls-files`, upper level for the
runtime, recursive for the module). The daemon shrank by **90** lines and the
extensions module grew by **453**.

### The daemon's own name clash

The kernel already had a field called `forge` — the **skills**' forge (the
curator). The script-tool forge's service is therefore called `scriptForge`,
and the field's comment says why: two forges, two fields. A field that
silently changed its meaning would have been a bug waiting for a reader.

## What is left

The extensions module now keeps the whole of the MCP registry's business
(W3.71) and the whole of the script-tool forge's business (W3.72).

What is left of the runtime's dissolution stands at **242 verbs** in
**9,080 lines**. The nearest families are the **MCP registry's fourteen**
(now forwarders), the **forge's eight** (now forwarders), the **board's
sixteen**, the **workflow host's twelve** and the **reasoning module's six**.

The next candidates the target architecture names: the **epistemic gate**
(→ `modules/knowledge`), the **policy hook** (→ `platform/policy`) and —
last, because it is the largest and the most entangled — the **run engine**
(→ `modules/runs`).

The self-repair module's edge into the runtime is still in the archcheck
allowlist — it is the other program's work, and it waits there.
