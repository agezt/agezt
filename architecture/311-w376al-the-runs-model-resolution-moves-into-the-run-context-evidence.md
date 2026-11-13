# W3.76al — the model resolution moves into the run context

**Slice:** W3.76al (the forty-sixth of the runtime's dissolution)
**Base:** `aee46271` (the W3.76ak merge)
**Date:** 2026-11-14

---

## 1. The family

A **run's model** is the run context's own vocabulary: the operator can pin a
model for one run (`runctx.WithModel`), and the run engine — which lives
**inside** the runs module — reads it back off the context it carries.

```go
m := runctx.ModelFromCtx(runCtx)
```

The **rule** that decides the answer was in the daemon. The daemon answered
the module's question for the module's own context:

```go
// kernel/runtime/accessors_ctx.go (before)
func (k *Kernel) ResolveRunModel(ctx context.Context) (string, bool) {
	if m := runctx.ModelFromCtx(ctx); m != "" {
		return m, true
	}
	return k.effectiveConfig(ctx).Model, false
}
```

**The host held the rule for a decision the context already held.** The round
trip was inside one module: the run engine (in `kernel/modules/runs/runner`)
called up to the daemon (in `kernel/runtime`) to learn what the context it was
already carrying said.

The daemon's copy was worse than a round trip: it was a **copy**. The daemon
also carried a private twin of the same rule for the agent profile path:

```go
// kernel/runtime/agentconfig.go (before)
func resolveRunModel(ctx context.Context, k *Kernel) (string, bool) { … }
```

— the same decision, written twice, in two files of the host.

## 2. What moved

The **rule** is the run context's now — **`kernel/modules/runs/runctx/model.go`**:

```go
func ResolveModel(ctx context.Context, defaultModel func() string) (string, bool) {
	if m := ModelFromCtx(ctx); m != "" {
		return m, true
	}
	return defaultModel(), false
}
```

Two things make it the module's and not a new host:

- the **per-run pick** is the context's own vocabulary (`WithModel` /
  `ModelFromCtx`), so the rule reads it without leaving the package;
- the **default** is the host's, and the rule asks for it **lazily** — a run
  that carries its own model never needs the host's config at all, and the
  host's config is a copy taken under a lock. A lazy default keeps the
  existing behaviour **exactly**, without paying for a config copy nobody
  reads.

The daemon keeps the **host's own state**, and only that: the live default
model (hot-swappable, M816) retuned by the running agent's own overrides
(M783). That is a **host state**, not a rule — it is what the host knows.

```go
// kernel/runtime/accessors_ctx.go (after)
func (k *Kernel) EffectiveModel(ctx context.Context) string {
	return k.effectiveConfig(ctx).Model
}
```

The **private copy** in `agentconfig.go` is gone: the agent-profile path asks
the same rule now, and there is one written answer to the question.

The **port** carries the host's state, not the rule:

```go
// kernel/modules/runs/runner/api.go
EffectiveModel(ctx context.Context) string
```

The **run engine** asks the module's rule, and asks the host for the default
only if the rule needs it:

```go
// kernel/modules/runs/runner/runner.go
model, modelExplicit := runctx.ResolveModel(runCtx, func() string { return r.k.EffectiveModel(runCtx) })
```

The engine is inside the module, so the rule is the module's own; the port
entry is only the host's **default model**, a piece of host state the rule
needs to answer.

## 3. The measurement

| what | before | after |
|---|---|---|
| the daemon's public verbs | 106 | **106** |
| the daemon's production lines | 5,213 | **5,213** |
| the daemon's production files | 52 | **52** |
| the app packages that import `kernel/runtime` | 40 | **40** |
| the run context's package | 7 files / 692 lines | **8 files / 716 lines** |
| the run engine's package | 6 files / 1,046 lines | **6 files / 1,064 lines** |
| the run engine's host port | 23 entries | **23 entries** |

**The daemon's surface is unchanged in count, and that is the honest number**:
one verb left (`ResolveRunModel`) and one arrived (`EffectiveModel`) — the
daemon still answers the host's own question, but it no longer answers the
**module's** question, and it no longer answers it **twice**.

File by file, the daemon's body did not grow either:

| file | before | after |
|---|---|---|
| `kernel/runtime/accessors_ctx.go` | 113 | **119** (+6, the new `EffectiveModel`) |
| `kernel/runtime/agentconfig.go` | 140 | **134** (−6, the private copy gone) |
| `kernel/runtime/workflow_host.go` | 133 | **133** (the port call, re-pointed) |
| `kernel/runtime/agentconfig_internal_test.go` | 137 | **129** (−8, the moved test) |
| `kernel/modules/runs/runner/api.go` | 125 | **139** (+14, the port entry and the note) |
| `kernel/modules/runs/runner/runner.go` | 326 | **330** (+4, the rule call) |

So the daemon's **lines are flat** and the module's are **up by 48** (24 into
the run context, 18 into the run engine, 14 into the port): the rule's body
**moved into the module** and the host kept the state it alone owns.

## 4. The ratchets — and the probe

**The daemon's side** (`kernel/runtime/model_ratchet_test.go`, new): the daemon
holds **no** verb per model resolution (`ResolveRunModel`), carries **no**
private copy of the rule (`func resolveRunModel(`), holds the host's default
(`EffectiveModel`), and asks the **module** for the rule
(`runctx.ResolveModel(`).

**The module's side** (`kernel/modules/runs/runner/runner_model_ratchet_test.go`,
new): the port carries the **host's state** and not the rule (no
`ResolveRunModel` entry), and the engine asks the module's rule
(`runctx.ResolveModel(`) and the host's default (`r.k.EffectiveModel(`).

The daemon's own surface ceiling **did not move**: **106 → 106** — one verb
left, one arrived, and the retired name joined the retired-verb list.

The **probe** put the world back before this slice — the daemon's verb with
its **real** body, the daemon's private copy, and the engine asking the host
to apply the context's own pick: the retired-verb ratchet, the daemon's four
model ratchets, the re-aimed port ratchet and the engine-asks-the-module
ratchet all went red at once — **seven** ratchets in one shot, and nothing
else broke; the probe was reverted and the suites went green again.

## 5. The guard

One earlier ratchet was **re-aimed**, not replaced: the run engine's
`TestThePortStillCarriesTheHostsOwnState` pinned the port's list of what the
host must keep; it named the **resolved model**, which was the rule, and it
now names the **effective default** — the host's state the rule asks for.
The rule itself is the module's, and the re-aimed list says so.

The **control plane's** runs family changed too: `ResolveRunModel` was in the
list of verbs that belong to **no** family — a verb the kernel had and no
guard named. `EffectiveModel` is the **persona/identity** family's business
(the effective model is the host's live default, retuned per agent), so the
guard now names it there: a verb the kernel has and the guard does not name
is a verb nobody guards.

## 6. Verification

- the compiler (`go build ./...`, `go vet ./…`) and `gofmt`;
- the daemon's tests, the run context's tests, the run engine's tests and the
  control plane's tests;
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 7. What is left

The daemon holds **106** public verbs and **5,213** production lines; **forty**
application packages still name it; the run engine's host port carries **23**
entries.

What is left in the daemon is the run's **remaining machinery**, and it is
small: one more door the run engine still opens for the **host's own state**
(`BuildLoopConfig` — the module's rule built from the daemon's live state)
and the host's own identity (`NewCorrelation`, `SubjectForRun`).

The **standing package** itself still lives at `kernel/standing`, and its
eventual home is the **triggers module** (`kernel/modules/triggers`) — the
architecture gate already places both `kernel/cadence` and `kernel/standing`
in that module, so the family's boundary is drawn; the module's directory does
not exist yet, and the move is a path move (four production importers), not a
lifecycle one.
