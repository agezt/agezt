# W3.75 — the policy hook's admission rules move into the platform

## What the slice is

The **eighth** slice of the runtime's dissolution, and the third one that
moves a **rule** (W3.73 the epistemic guard, W3.74 the guards).

The policy hook was not finished after W3.74: the guards moved, but the hook
still owned two more rules — the **capability resolution** and the **approval
bundle**'s tables.

- The **capability resolution** answers: *which policy axis is this call
  judged on?* Three sources, in order of how specific they are: the tool's
  own declaration, the plugin manifest overlay (M900), and the name switch
  that still owns the dynamic surfaces no static declaration can cover
  (forged `forge_*` script tools, bridged `mcp_*` calls). And one rule that
  makes the order safe: a declaration naming a capability Edict does not
  govern is **ignored** rather than honoured — an unknown capability is
  default-denied, so honouring a typo would silently kill the tool.
- The **approval bundle** answers: *what is the operator told about this
  call?* The effect class, the predicted effects, the affected resources, the
  rollback notes and the confidence — each with a default when the tool
  declared none, and each default is a **row** (a capability's class, a
  class's rollback note, a class's confidence).

They are rules, not plumbing. So they moved to
`kernel/platform/policy/admission`, and the policy hook now asks.

## What the slice found

The candidate was measured before it was chosen:

- the daemon's capability resolution (`validatedToolCaps` + `capabilityFor`):
  **44 non-blank lines**;
- the daemon's bundle tables (`approvalBundle`, `approvalDecisionBundle`,
  `normalizeEffectClass`, `defaultEffectClass`, `affectedResourcesFromInput`,
  `defaultRollbackNotes`): **99 non-blank lines**;
- the daemon's copies of the rules: **6** functions and **1** type;
- the daemon's own stores: **0** — the rules read the call, the tool
  definition and the tool map, and they own none of them.

**A rule that owns no store is the platform's** — the rule W3.73 stated, now
confirmed on a third and a fourth rule.

## What changed

### The platform keeps the rules

`kernel/platform/policy/admission/admission.go` (201 lines) is the admission
rule: the capability resolution and the bundle's assembly, with the tables
that back them.

```go
// kernel/platform/policy/admission/admission.go
func ValidatedToolCaps(declared map[string]string) map[string]edict.Capability
func ResolveCapability(def toolapi.ToolDef, overlay map[string]edict.Capability, name string, input json.RawMessage) edict.Capability

type Bundle struct {
	EffectClass       string
	PredictedEffects  []string
	AffectedResources []string
	RollbackNotes     string
	Confidence        float64
}

type Lookup func(name string) (toolapi.ToolDef, bool)

func BundleFor(toolName string, cap edict.Capability, input json.RawMessage, def toolapi.ToolDef, lookup Lookup) Bundle
```

The one thing the platform cannot have is the **tool map** — the host builds
the tools — so the lookup is a **port**: a way to ASK for a definition by
name. A host with no tool map is not an error: the bundle then works with
the definition the caller handed it, and with nothing else.

### The daemon keeps the doors

```go
// kernel/runtime/policy.go
func (k *Kernel) capabilityFor(tc llm.ToolCall, def toolapi.ToolDef) edict.Capability {
	return admission.ResolveCapability(def, k.toolCaps, tc.Name, tc.Input)
}

// kernel/runtime/policy_helpers.go
func (k *Kernel) approvalDecisionBundle(toolName string, cap edict.Capability, input json.RawMessage, def toolapi.ToolDef) admission.Bundle {
	return admission.BundleFor(toolName, cap, input, def, k.lookupToolDef)
}
```

The daemon keeps its two doors — its **overlay** (`k.toolCaps`) and its
**tool map** (`k.tools`) — and asks.

## What the slice cost

**Nothing to wire**, for the third time and for the same reason: the rules'
inputs are things the host already has.

- a move into an **existing** face costs **nothing** (W3.70: 0 sites);
- a move that invents a **new** face costs a wiring in every host that has one
  (W3.68: 11, W3.69: 6, W3.71: 6, W3.72: 9);
- a move of a **rule that owns no store** into the **platform** costs
  **nothing** (W3.73: 0, W3.74: 0, W3.75: 0 sites).

## The rule

The host keeps the **doors**; the platform keeps the **rule**.

W3.73 said it for one guard, W3.74 for a guard set, and W3.75 for the rules
that decide **what the operator is told** — the last of the policy hook's
own rules. What is left in the hook is plumbing: reading the context,
publishing the events, submitting the approval.

## What did NOT move, and why

- **The agent-noise policy stayed in the daemon.** It is the host's: it reads
  the run's context and the daemon's own **state** (the notify cooldown), and
  it owns both. It is not a rule about a call — it is the host's noise
  bookkeeping.
- **The bundle's shape did not change.** The same five fields reach the
  approval request (`EffectClass`, `PredictedEffects`, `AffectedResources`,
  `RollbackNotes`, `Confidence`), and the same rows reach the journal.
- **The capability resolution did not change.** The same three sources, in
  the same order, with the same "ignore an unknown axis" rule.
- **The confidence row did not move a second time.** It was already the
  platform's (W3.73); the bundle now asks the platform for it.

## The contract row

**None.** These rules answer with a decision, not an error.

## The tests

**19 permanent tests** (16 platform, 3 internal):

- `kernel/platform/policy/admission/admission_test.go` — **16 tests**: the
  overlay keeps only known axes (and an unusable overlay is empty), the
  declaration wins, an **unknown** declaration is ignored (a typo must
  degrade to the fallback, never to default-deny), the overlay beats the name
  switch, the name switch still covers the dynamic surfaces, the bundle uses
  the tool's own effect, the bundle asks the host for a silent definition, the
  bundle without a lookup uses what it has, the bundle derives the resources
  from the input, the bundle falls back to the class's confidence, the effect
  class rows, the normalized classes, the resources from the input, the
  resources from an unreadable input, the resources skip empty values, and the
  rollback rows.
- `kernel/runtime/admission_internal_test.go` — **3 tests**: the capability
  resolution is the platform's (the delegation itself), the approval bundle
  is the platform's, and the **ratchet** — the daemon's policy hook must not
  grow a copy of the admission rules back.

### The test the mutations had to sharpen

The mutation that made the bundle **ignore the host's tool map** survived the
first calibration, and the reason is worth writing down: the test's looked-up
definition carried the **same** effect class the capability's default would
have produced, and the same confidence. A bundle that never asked would have
looked identical.

The test now asks for a definition whose effect **differs** from the
capability's default — so a bundle that does not ask cannot pass.

### The determinism the logs demanded

The two passes were not byte-identical at first, and the cause was the
**tests**, not the code: two table tests iterated a **map**, so the failing
row — and therefore the failure message in the log — changed from run to run.
The rows are **slices** now. A mutation log is evidence, and evidence that
changes between two runs of the same tree is not evidence.

## The mutations

**23 mutations, 23 caught**, in two independent passes with **byte-identical**
logs.

SHA-256: `29DC1C4F939A37AF7D0FE2F65115DC69652DDB6B0A3B97B2C0B1CD47D5FF3CE0`

The calibration reported **one survivor and three compile errors**, and all
four were the mutation's fault or the test's gap:

- the mutation that made the bundle ignore the host was **equivalent on the
  test that existed** — the test has been sharpened (above);
- the mutation that emptied the predicted effects left `fmt` unused, and the
  one that replaced the confidence left `epistemic` unused, so Go refused to
  compile — both now keep the import alive;
- the mutation that changed the read-only rollback row produced a **duplicate
  case**, which Go refuses to compile — it now changes the row's text.

- **M01**, **M02** the overlay keeps an unknown axis, or an unusable overlay
  is not empty.
- **M03**, **M04**, **M05**, **M06** the capability resolution ignores the
  declaration, honours a typo, loses the overlay, or stops covering the
  dynamic surfaces.
- **M07**, **M08**, **M09**, **M10**, **M11**, **M12**, **M13** the bundle
  ignores the tool's effect, does not ask the host, loses the class fallback,
  does not derive the resources, loses the call from the predicted effects,
  loses the class's rollback row, or loses the class's confidence.
- **M14**, **M15**, **M16**, **M17**, **M18**, **M19** the platform's tables:
  the compensable row loses capabilities, an unclassified axis is not the safe
  one, an unknown class is kept, the resources lose their values, a blank
  value names something, the read-only rollback row is wrong.
- **M20**, **M21**, **M22**, **M23** the daemon's wiring: it resolves the
  capability itself, builds the bundle itself, hides its overlay, or hides its
  tool map.

## The measurement that moved

| | before | after |
|---|---|---|
| the daemon's copies of the admission rules | **7** (6 functions + 1 type) | **0** |
| the platform's lines of rule | **0** | **201** |
| the daemon's calls into the platform's rules | **0** | **2** |
| the kernel's verbs | **242** | **242** |
| the application layer's production files that changed | — | **0** |
| the test setups that had to be wired | — | **0** |
| the contract's new rows | — | **0** |
| `kernel/runtime` (lines) | **8,796** | **8,687** |
| `kernel/platform` (lines) | **3,180** | **3,381** |
| the daemon's `policy.go` (lines) | **228** | **198** |
| the daemon's `policy_helpers.go` (lines) | **220** | **140** |
| the platform's `admission.go` (lines) | — | **201** |

The daemon's copies of the admission rules fell from **7** to **0** — and a
**ratchet test** holds them there, over both files the rules lived in.

The daemon shrank by **109** lines this time (W3.74 shrank by **6**), and the
difference is the rule: W3.74 moved a rule that the host's wiring had to
replace, and W3.75 moved rules the host only had to *ask* about. The
platform's **201** lines against the daemon's **143** of rules is the
platform's documentation and its exported faces — the rules themselves are
the same rows, in the same place.

The lines are counted the way the previous slices counted them: **non-blank**
lines of the package's production files (`git ls-files`, upper level for the
runtime, recursive for the platform).

## What is left

The runtime's dissolution stands at **242 verbs** in **8,687 lines**.

The **policy hook** is now finished as far as its rules go: the capability
resolution, the bundle, the epistemic gate and the guards are all the
platform's, and what is left in the hook is plumbing — reading the context,
publishing the events, submitting the approval.

What remains of the runtime's dissolution is the **run engine**
(→ `modules/runs`): the largest and the most entangled — `prepareSubAgent`
(279 lines), `executeSubAgent` (89), `subAgentInjectedSystem` (87),
`buildRunPrompt` (170), `buildLoopConfig` (78) and the runexec runners. It is
the last big family, and it is the one the target architecture wants in
`modules/runs`.

The self-repair module's edge into the runtime is still in the archcheck
allowlist — it is the other program's work, and it waits there.
