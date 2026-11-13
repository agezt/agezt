# W3.77 — the loop configuration's door moves into the runs module

**Slice:** W3.77 (the forty-seventh of the runtime's dissolution)
**Base:** `71fe5762` (the W3.76al merge)
**Date:** 2026-11-14

---

## 1. The family

A **governed run's loop configuration** is the shape every run's loop must
have — root (`RunWith`) or delegated (`executeSubAgent`): the tool surface, the
tool selector, the context budget, the artifact offload, the cost function and
the elided-output summarizer.

The **rule** had already moved to the runs module in W3.76g
(`kernel/modules/runs/loopcfg`). What stayed in the daemon was the **door**:

```go
// kernel/runtime/loopcfg_doors.go (before)
func (k *Kernel) BuildLoopConfig(runCtx context.Context, corr, model string) agent.LoopConfig {
	svc := k.loopcfg
	if svc == nil {
		svc = loopcfg.New(k.loopcfgDeps())
	}
	return svc.Build(runCtx, corr, model, loopcfgConfig(k.effectiveConfig(runCtx)))
}
```

— the daemon's projection of its effective configuration, the daemon's wiring
of its live collaborators, and a call to the module's own rule.

**Two round trips inside one module.** The **run engine** (which lives inside
the runs module) asked the **host** for the shape of its own run's loop
through its port:

```go
// kernel/modules/runs/runner/runner.go (before)
lc := r.k.BuildLoopConfig(runCtx, corr, model)
```

and the **delegation engine** (which lives next door in the same module) asked
the host for the child's base through a door of its own:

```go
// kernel/runtime/subagent_doors.go (before)
func (h delegationHost) BaseLoopConfig(ctx context.Context, corr, model string) agent.LoopConfig {
	return h.k.BuildLoopConfig(ctx, corr, model)
}
```

The module had the body in both cases. The host held a door over it.

## 2. What moved

The **entry** is the module's now — **`kernel/modules/runs/loopcfg`**:

```go
// Build assembles the loop configuration for a run, asking the HOST's live
// knobs for that run's configuration.
func (s *Service) Build(runCtx context.Context, corr, model string) agent.LoopConfig {
	return s.BuildWith(runCtx, corr, model, s.config(runCtx))
}

// BuildWith assembles it from an EXPLICIT configuration.
func (s *Service) BuildWith(runCtx context.Context, corr, model string, cfg Config) agent.LoopConfig { … }
```

The knobs are the **host's own state** — the effective configuration is
per-run (the daemon-wide config plus the operator's live edits plus this
agent's own overrides) — so the rule asks for them through a dependency the
host wires:

```go
// kernel/modules/runs/loopcfg/config.go
type Deps struct {
	// Config returns the run's EFFECTIVE configuration, projected into the
	// knobs the loop needs. It is asked PER RUN and never snapshotted.
	Config func(ctx context.Context) Config
	…
}
```

The **module never learns the host's knob names**: the daemon projects
(`loopcfgConfig`) and hands the projection over. A service wired without the
provider falls back to the **module's own** default ceiling — a zero ceiling
would end every run at its first turn, and a missing host is a wiring mistake,
not a reason to run with no limits at all.

The **run engine** holds the module's service — the very same service the
delegation engine holds — and asks the module:

```go
// kernel/modules/runs/runner/runner.go
if r.loopCfg == nil {
	return "", errors.New("runner: the loop configuration is not available")
}
…
lc := r.loopCfg.Build(runCtx, corr, model)
```

The **delegation engine** holds it too, in its `Deps` beside the prompt
service:

```go
// kernel/modules/runs/delegation/exec.go
if s.deps.LoopCfg == nil {
	return "", fmt.Errorf("sub-agent %s: the loop configuration is not available", p.ChildCorr)
}
…
lc := s.deps.LoopCfg.Build(p.ChildCtx, p.ChildCorr, p.SubModel)
```

The daemon keeps the **wiring** and nothing else. Its file is
`loopcfg_wiring.go` now (it was `loopcfg_doors.go`): the projection of its
effective configuration and the live collaborators the rule is built from —
the merged tool surface, the tool-phase service, the bus, the policy hook, the
noise-policy hook, the artifact port and the model catalog. The daemon
**never calls the rule itself**: the module's consumers ask the module.

## 3. The measurement

| what | before | after |
|---|---|---|
| the daemon's public verbs | 106 | **105** |
| the daemon's production lines | 5,213 | **5,214** |
| the daemon's production files | 52 | **52** |
| the app packages that import `kernel/runtime` | 40 | **40** |
| the run engine's host port | 23 entries | **22 entries** |
| the loop configuration's package | 6 files / 525 lines | **6 files / 566 lines** |
| the run engine's package | 6 files / 1,064 lines | **6 files / 1,098 lines** |
| the delegation engine's package | 10 files / 1,150 lines | **10 files / 1,174 lines** |

**One verb off the daemon's surface, and its lines are flat** — and that is
the honest number: the daemon's body did not shrink, it **moved**. File by
file:

| file | before | after |
|---|---|---|
| `kernel/runtime/loopcfg_doors.go` → `loopcfg_wiring.go` | 78 | **71** (−7, the door's body gone) |
| `kernel/runtime/subagent_doors.go` | 105 | **100** (−5, the delegation door gone) |
| `kernel/runtime/compose.go` | 657 | **665** (+8, the wiring and the two injections) |
| `kernel/runtime/runtime_kernel.go` | 253 | **258** (+5, the field's note) |
| `kernel/modules/runs/loopcfg/config.go` | 97 | **120** (+23, the provider and its note) |
| `kernel/modules/runs/loopcfg/service.go` | 90 | **99** (+9, the two entries) |
| `kernel/modules/runs/runner/runner.go` | 330 | **359** (+29, the field, the refusal, the call) |
| `kernel/modules/runs/delegation/host.go` | 106 | **118** (+12, the dep and the note) |
| `kernel/modules/runs/delegation/exec.go` | 162 | **172** (+10, the refusal and the call) |

The module grew by **99** lines (loop configuration **+41**, run engine **+34**,
delegation engine **+24**) and the daemon grew by **one**: the door's body
left, the wiring's note and the two injections came in.

## 4. The ratchets — and the probe

**The daemon's side** (`kernel/runtime/loopcfg_ratchet_test.go`, re-aimed): the
daemon holds the **wiring** (`loopcfgConfig`, `loopcfgDeps` — the file it used
to keep its door in) and holds **no** door: no `func (k *Kernel) BuildLoopConfig(`
in any production file, and no `k.loopcfg.Build` — the daemon must not speak
for the rule.

**The run engine's side** (`kernel/modules/runs/runner/runner_loopcfg_ratchet_test.go`,
new): the port carries **no** loop-configuration entry, the engine holds the
module's service, the engine does **not** ask the host for it, and a host that
wired none leaves the engine quiet — it says the configuration is missing
instead of running an ungoverned loop.

**The delegation engine's side**
(`kernel/modules/runs/delegation/delegation_loopcfg_ratchet_test.go`, new): the
host port carries **no** `BaseLoopConfig` door, the engine asks the module
(`s.deps.LoopCfg.Build(`) and never the host, and a host that wired none
leaves the child run ungoverned-free — it fails with a clear error.

Two earlier ratchets were **re-aimed**, not replaced:

- the W3.76g loop-configuration ratchet demanded that the daemon **keep** its
  three doors (the projection, the wiring and `BuildLoopConfig`); the daemon
  now keeps the first two and the third is the module's, so the ratchet
  demands the **absence** of the door;
- the delegation ratchet demanded that the daemon keep its `BaseLoopConfig`
  door; the same way, the ratchet now demands its **absence**.

The daemon's own surface ceiling followed the floor: **106 → 105** (220 → 201
→ 194 → 179 → 173 → 166 → 164 → 156 → 152 → 148 → 143 → 135 → 129 → 125 → 121
→ 118 → 116 → 114 → 111 → 108 → 107 → 106 → 105), and the retired name joined
the retired-verb list.

**The probe** put the world back before this slice — the daemon's door file
with its **real** body, the daemon's verb, the runner's port entry and the
engine asking the host again, the delegation door and the engine asking the
host for the child's base: the retired-verb ratchet, the surface ceiling
(106 > 105), the daemon's loop-configuration ratchet, the daemon's delegation
ratchet, the run engine's two ratchets and the delegation engine's two
ratchets all went red at once — **eleven** ratchets in one shot, and nothing
else broke; the probe was reverted and the suites went green again.

## 5. The guard

The **coverage moved with the subject**. The daemon's door test file
(`loopcfg_doors_test.go`) held eight tests; four of them were about the
**rule** (the governed shape, the catalog-derived budget, the deferred
discovery, the run allowlist) and the module's own tests already pin all four
— they moved to the module in W3.76g. The other four were about the **daemon's
own state** (the projection of its effective configuration, the agent's own
overrides, its live tool surface, its bus/policy/phases/notify) and they
stayed — **white-box**, in `kernel/runtime/loopcfg_wiring_internal_test.go`,
because the daemon's service is now the only place its wiring is observable.

The tool-phase assertions moved with them: the loop configuration carries the
**daemon's bound** phase service, bound once per kernel. That was observed
through the daemon's door; it is now observed through the daemon's own wiring.

The **control plane** needed no change: the retired verb belonged to no
family (a word rule cannot reach "BuildLoopConfig" — it carries neither "Run"
nor "With" nor a family's name), and a verb that no family names is a verb
the adapter may not name either.

## 6. Verification

- the compiler (`go build ./...`, `go vet ./…`) and `gofmt`;
- the daemon's tests, the loop configuration's tests, the run engine's tests,
  the delegation engine's tests, the control plane's tests and the
  application's tests;
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 7. What is left

The daemon holds **105** public verbs and **5,214** production lines; **forty**
application packages still name it; the run engine's host port carries **22**
entries.

What is left in the daemon is the run's **remaining machinery**, and it is
small: the host's own **identity** (`NewCorrelation`, `SubjectForRun`) and the
host's own **state** the run engine still reads — the live tools, the catalog,
the provider, the memory/world/forge ports and the file store. The run's
**rules** — the context, the prompt, the publish surface, the ticket
lifecycle, the retry rules, the tool fold, the model's resolution and now the
loop configuration — are all the module's own, and the run engine asks the
module for each of them.

The **standing package** itself still lives at `kernel/standing`, and its
eventual home is the **triggers module** (`kernel/modules/triggers`) — the
architecture gate already places both `kernel/cadence` and `kernel/standing`
in that module, so the family's boundary is drawn; the module's directory does
not exist yet, and the move is a path move (four production importers), not a
lifecycle one.
