# W3.74 — the policy hook's guards move into the platform

## What the slice is

The **seventh** slice of the runtime's dissolution, and the second one that
moves a **rule** (W3.73 moved the epistemic guard).

The daemon's tool-call policy hook (`kernel/runtime/policy.go`) is the gate
every tool call passes. It holds three **fail-closed guards** — rules that
stop an otherwise-allowed call and send it to a human:

- the **epistemic escalation** (the platform's own guard, W3.73);
- the **intent/regret gate** (the operator's frame says the call's regret axes
  need a confirmation);
- the **prompt-injection guard** (an effectful call downstream of a
  directive-like untrusted observation).

It also holds the **session grant's arbitration**: the blanket capability
grant ("auto-approve Tool Forge this session") answers the Edict Ask axis and
nothing else.

Those are rules, not plumbing. They own no store, they publish nothing, and
every host that gates a tool call asks the same ones. The target architecture
says where infrastructure lives: `kernel/platform`. So the guards moved to
`kernel/platform/policy/guards`.

## What the slice found

The candidate was measured before it was chosen:

- the daemon's guard block: **57 non-blank lines** of conditions and
  arbitration in `policy.go`;
- the prompt-injection **mode** (the operator's setting, its parser and its
  three values): **27 non-blank lines** in `runctx_security.go`;
- the daemon's copies of the guards' rules: **3** (one per guard);
- the daemon's own stores: **0** — the guards read the context and the verdict,
  and they own neither.

**A rule that owns no store is the platform's** — the rule W3.73 stated, now
confirmed on a second rule.

## What changed

### The platform keeps the guards

`kernel/platform/policy/guards/guards.go` (167 lines) is the guard set: the
three rules, the order they are asked in, the mode and the session grant's
arbitration.

```go
// kernel/platform/policy/guards/guards.go
type Input struct {
	Allow                bool
	AskRequired          bool
	AskReason            string
	EffectClass          string
	Epistemic            epistemic.Decision
	RequiresConfirmation bool
	ConfirmationPrompt   string
	DirectiveLike        bool
	TaintSources         []string
	TrustedObservations  bool
	EpistemicEscalation  bool
	IntentRegretGating   bool
	PromptInjection      PromptInjectionMode
	AutoApprove          bool
}

type Outcome struct {
	RequiresApproval bool
	Reason           string
	Guard            string
	Warn             bool
	AutoApproved     bool
}

func Evaluate(in Input) Outcome
```

The host fills the input in and applies the outcome. **Three properties are
the platform's, and they are the ones that used to be the daemon's:**

1. **The order.** The epistemic escalation, then the intent/regret gate, then
   the prompt-injection guard — and the **first** one that raises wins. A
   guard that has already stopped the call cannot be answered by a later one,
   and the operator is shown one reason: the guard's own.
2. **The grant's reach.** The session's blanket capability grant answers the
   Edict Ask axis and nothing else. A guard the operator armed deliberately
   asked a different question, and a blanket grant is not an answer to it.
3. **The mode.** `off`/`warn`/`on`, the operator's spellings and the default
   — the platform's rows, with the daemon's names kept as aliases.

### The daemon keeps the doors

`kernel/runtime/policy.go` now assembles the evidence, asks, and applies what
comes back:

```go
outcome := guards.Evaluate(guards.Input{ /* seventeen rows of evidence */ })
if outcome.Guard != "" {
	verdict.Allow = false
	verdict.WouldAsk = true
}
if outcome.Guard == guards.GuardIntentRegret {
	k.publishIntentConfirmationRequired(...)
}
if outcome.Warn {
	k.publishPromptInjectionWarned(...)
}
```

The daemon still reads the context, still publishes the events and still
submits the approval. What it no longer owns is **when** a guard raises.

### The daemon's names stay

`PromptInjectionMode`, `PromptInjectionOn`, `PromptInjectionWarn`,
`PromptInjectionOff` and `ParsePromptInjectionMode` are the daemon's public
surface, and `cmd/agezt` parses the operator's setting with them. They are now
**aliases** of the platform's rows:

```go
type PromptInjectionMode = guards.PromptInjectionMode

const (
	PromptInjectionOn   = guards.PromptInjectionOn
	PromptInjectionWarn = guards.PromptInjectionWarn
	PromptInjectionOff  = guards.PromptInjectionOff
)
```

**No caller outside the runtime changed**: **0 production files** in
`kernel/app`, and `cmd/agezt` still calls `runtime.ParsePromptInjectionMode`.

## What the slice cost

**Nothing to wire.** The same reason as W3.73: the guards' inputs are things
the host already has — the context, the verdict, the bundle, the settings.

- a move into an **existing** face costs **nothing** (W3.70: 0 sites);
- a move that invents a **new** face costs a wiring in every host that has one
  (W3.68: 11, W3.69: 6, W3.71: 6, W3.72: 9);
- a move of a **rule that owns no store** into the **platform** costs
  **nothing** (W3.73: 0 sites, W3.74: 0 sites).

## The rule

The host keeps the **doors**; the platform keeps the **rule**.

W3.73 said it for one guard; W3.74 says it for a guard **set**, and the
difference shows in what the platform had to be given: not ports, but
**evidence** — the facts the rules reason over. The host gathers the facts
and cannot decide anything with them.

## The measurement that did NOT move

The daemon's line count barely moved: `policy.go` went from **231** to **228**
lines, and `kernel/runtime` from **8,802** to **8,796**.

That is the honest result, and it is worth writing down rather than dressing
up: **moving a rule moves the CONDITIONS, not the lines.** The daemon's
conditions are gone (a ratchet test holds them gone), but the host's *wiring*
— seventeen rows of evidence, each one a fact the host read — took their
place. The wiring was implicit before (it was inside the conditions) and it is
explicit now.

What moved is the **ownership**: the platform holds **167** lines of rule that
every host asks, and the daemon holds none.

## What did NOT move, and why

- **The verdict's shape did not change.** The same thirteen epistemic fields,
  the same `allow`/`would_ask`/`hard_denied` flags and the same
  `policy.decision` rows reach the journal.
- **The events did not change.** `intent.confirmation_required`,
  `policy.auto_approved` and `prompt_injection.warned` are the same three
  kinds, with the same subjects and the same payload rows — only the place
  that decides to publish them moved.
- **The order of the guards did not change**, and neither did the reason the
  operator sees: the first guard's own.
- **The grant's reach did not change**: it answers the Ask axis, and it never
  answers a guard.
- **The live HITL path did not change.** The daemon still submits the
  approval and still interprets the operator's decision.

## The contract row

**None.** The guards have no refusal to share: they answer with a decision,
not an error. (The contract's rows exist for the two-sided refusals —
`mcpapi.ErrNoRegistry`, `toolforgeapi.ErrNoStore`.)

## The tests

**23 permanent tests** (17 platform, 4 runtime, 2 internal), plus the two the
slice had to add because the mutations showed the gap:

- `kernel/platform/policy/guards/guards_test.go` — **17 tests**: the epistemic
  guard raises when armed and stays silent when not, the intent/regret gate
  raises with the frame's own prompt, the **first guard wins**, the
  prompt-injection guard blocks an effectful call and names its sources, warns
  in warn mode, warns for a run the operator trusted, ignores a read-only
  call and stays silent when off, the grant answers the Ask axis, the grant
  does **not** answer a guard, the grant does not create an approval, a denied
  call raises no guard, the Ask axis without the grant needs a human, the mode
  parsing, and an allowing decision does not raise.
- `kernel/runtime/policy_guards_test.go` — **4 tests**: the reason the
  operator is shown is the **platform's** reason for the same evidence (the
  delegation itself), the guard's verdict is journaled as a call that *would
  have asked*, the warn mode's warning is journaled, and the intent gate's
  question is journaled.
- `kernel/runtime/policy_guards_internal_test.go` — **2 tests**: the ratchet
  (the daemon's policy hook must not grow a copy of the guards' conditions
  back) and the mode's alias identity.

### The two tests the slice had to add

The mutation harness found both, and both are the programme's standing rule:
**a mutation that survives means the test is missing something.**

- **The warning was invisible.** In warn mode the call runs, so a daemon that
  stopped publishing `prompt_injection.warned` would still pass every
  behaviour test. The event is now observed directly.
- **The guard's verdict was invisible.** The approval request carries the
  guard's reason, so a daemon that stopped setting the verdict's
  `would_ask` flag would still pass. The journaled `policy.decision` is now
  observed directly.

## The mutations

**23 mutations, 23 caught**, in two independent passes with **byte-identical**
logs.

SHA-256: `44EDB41A6422CF8D24AB1760671E49D625EE96CBB058F5F698C38C005F11A4C4`

The calibration had three survivors and one compile error, and all four were
the harness's fault or the test's gap, not the rule's:

- the mutation that swapped the guards' **order** swapped the conditions
  without the bodies — a chimera that still answered the test. The mutation
  now uses `fallthrough`, which really does let a later guard answer an
  earlier one.
- the mutation that dropped the prompt-injection guard's evidence left the
  `toolapi` import unused, so Go refused to compile. It now keeps the import
  alive.
- the mutation that let the session grant answer a guard was **equivalent**:
  the guards return early, so the grant never sees one — the platform had a
  **dead condition** that said the same thing twice. The condition is gone
  and the comment now says why a guard that raised has already returned.
- the mutation that made the daemon ignore the guard was caught only after
  the journaled verdict became observable.

- **M01**, **M02**, **M03** the platform's epistemic guard raises on the wrong
  answer, under another guard's name, or without its reason.
- **M04**, **M05** the intent/regret gate raises on the wrong answer or loses
  its prompt.
- **M06** a later guard answers an earlier one.
- **M07**, **M08**, **M09** the prompt-injection guard ignores its evidence,
  forgets a trusted run, or loses the warning.
- **M10**, **M11**, **M12** the session grant creates an approval, an
  auto-approved call still needs a human, or the guards fire on a denied call.
- **M13**, **M14** the mode loses its spellings or an unknown mode becomes
  `on`.
- **M15**, **M16**, **M17**, **M18** the daemon ignores the guard, the warning,
  the auto-approval or the intent question.
- **M19**, **M20**, **M21**, **M22** the daemon's wiring hides the untrusted
  observation, the effect class, the session grant or the epistemic setting.
- **M23** the daemon parses the mode itself.

## The measurement that moved

| | before | after |
|---|---|---|
| the daemon's copies of the guards' rules | **3** | **0** |
| the platform's lines of rule | **0** | **167** |
| the daemon's calls into the platform's rules | **0** | **1** |
| the daemon's public names for the mode | **5** | **5** (aliases) |
| the kernel's verbs | **242** | **242** |
| the application layer's production files that changed | — | **0** |
| the test setups that had to be wired | — | **0** |
| the contract's new rows | — | **0** |
| `kernel/runtime` (lines) | **8,802** | **8,796** |
| `kernel/platform` (lines) | **3,013** | **3,180** |
| the daemon's `policy.go` (lines) | **231** | **228** |
| the daemon's `runctx_security.go` (lines) | **102** | **99** |
| the platform's `guards.go` (lines) | — | **167** |

The daemon's copies of the guards' rules fell from **3** to **0** — and a
**ratchet test** holds them there, because a rule that can creep back is a
rule that will. The kernel's verb count did not move (**242 → 242**) and no
contract row was added.

The lines are counted the way the previous slices counted them: **non-blank**
lines of the package's production files (`git ls-files`, upper level for the
runtime, recursive for the platform).

### The honest line

`kernel/runtime` shrank by **6** lines and the platform grew by **167**. That
is not the daemon's 6 lines of rule — it is the platform's 167, and the
daemon's own **57** lines of guard rule are now **0**. What the daemon kept
is the wiring, and the wiring was always there: it was inside the conditions.

## What is left

The runtime's dissolution stands at **242 verbs** in **8,796 lines**.

The **policy hook** is not finished: the guards moved, but the hook still owns
the **capability resolution** (`capabilityFor`: the declaration, the plugin
manifest overlay and the name switch, in that order) and the **approval
bundle**'s tables (the effect class per capability, the rollback notes, the
resources derived from the input). Those are rules too, and they are the
natural next slice: the same shape, one platform package, and the daemon
keeping its names.

After that, the largest and the most entangled: the **run engine**
(→ `modules/runs`).

The self-repair module's edge into the runtime is still in the archcheck
allowlist — it is the other program's work, and it waits there.
