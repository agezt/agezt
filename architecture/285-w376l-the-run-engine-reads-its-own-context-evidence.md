# W3.76l — the run engine reads its own context from its own module

**Slice:** W3.76l (the twentieth of the runtime's dissolution)
**Base:** `15d75fe4` (the W3.76k merge)
**Date:** 2026-11-02

---

## 1. The family

**The run context's vocabulary** — the names that describe a run to itself:
the wake's provenance (source, reason, schedule, standing, trigger subject,
parent correlation), the attached images, the json-mode flag, the cost
ceiling, the model chain, the agent's daily ceiling and the actor.

The vocabulary belongs to **one** package: `kernel/modules/runs/runctx`. The
daemon kept **fifteen forwarding copies** of it — `WakeContextSource`,
`MaxCostFromCtx`, `WithActorCorrelation` and their siblings — and the **run
engine** asked the host for them, through the port:

```go
lc.WakeSource = r.k.WakeContextSource(runCtx)
lc.MaxRunCostMicrocents = r.k.MaxCostFromCtx(runCtx)
```

The run engine **lives inside the module that owns the vocabulary**. It was
asking the host to read the run's own decoration, and the host was keeping a
second spelling of the module's names so that it could answer.

## 2. The distinction the slice turns on

The daemon keeps some of these doors **on purpose**, and the reason is real:
a **module** that binds a run must not name the runs module, or it creates a
cross-module edge. So the daemon keeps `WithAgentProfile` and `WithMaxCost`
— the doors the self-repair module uses.

But the **run engine** is not such a module: it is the runs module's own
engine. For it, the port hop bought nothing and cost a duplicate spelling.

| who asks | how it asks | why |
|---|---|---|
| a module (self-repair) | `k.WithAgentProfile(ctx, p)` — the host's door | naming the runs module would be a cross-module edge |
| the **application** | `runctx.WithAgentProfile(ctx, p)` | the app already decorates runs through the module |
| the **run engine** | `runctx.WithAgentProfile(ctx, p)` | it IS the module — the host hop bought nothing |

The application already named the module for the **writers** (see
`kernel/app/runs/run_context.go:36-40`); only the **read** side went through
the host. This slice makes the two sides agree.

## 3. What moved

The run engine now asks the module:

```go
runCtx = runctx.WithActor(runCtx, actor, corr)
systemAgent := runctx.SystemAgentFromCtx(runCtx)
modelChain := runctx.ModelChainFromCtx(runCtx)
lc.AgentDailyCeilingMc = runctx.AgentDailyMcFromCtx(runCtx)
wake := runctx.WakeContextFromCtx(runCtx)
lc.WakeSource = wake.Source
...
lc.Images = runctx.ImagesFromCtx(runCtx)
lc.JSONMode = runctx.JSONModeFromCtx(runCtx)
lc.MaxRunCostMicrocents = runctx.MaxCostFromCtx(runCtx)
```

The fifteen daemon copies are gone, and so are the fifteen entries of the
host's port.

**What stays on the port** is the host's own state — the answers the module
cannot give: the effective config (the model the run falls back to, the
heuristic bypass), the resume ticket, the seed and the checkpoint callback.

## 4. The measurement

| measure | before | after |
|---|---:|---:|
| the daemon's public verbs | 220 | **205** |
| the daemon's production lines (non-blank) | 6,200 | **6,124** |
| the daemon's production files | 54 | 54 |
| the run engine's port (`runner.KernelAPI`) | 81 methods | **66** |
| the contract's rows / the control plane / the app layer's production files | — | **+0 / +0 / +0** |

The port lost **fifteen** methods and the daemon **fifteen** verbs — the same
fifteen, one spelling each.

## 5. The tests that moved with it

Four test files read the run's context through the daemon's copies. They now
read it through the module's names, which is the reader side of the same
decoration the application writes:

- `kernel/app/runs/run_context_test.go` — the decoration test reads the wake,
  the ceiling, the system flag and the model chain through `runctx.`;
- `kernel/app/correlation/ports_test.go` — the actor is read through
  `runctx.ActorFromCtx`;
- `kernel/controlplane/app_host_test.go` — the routed actor is read the same
  way;
- `cmd/agezt/resume_profile_test.go` — the restarted run's ceiling is read
  through `runctx.MaxCostFromCtx`.

And the application's correlation port now holds the module's writer as the
capability (`actor: runctx.WithActor`), next to the daemon's mint — the
daemon's own mint stays, because the correlation is the daemon's.

## 6. The ratchets — and the probe

**The daemon's ratchet shrank, honestly.** `runctx_ratchet_test.go` listed
fourteen module names the daemon had to read the run's dispatch through;
fifteen of them are the module's own now, so the list is the six the daemon
still reads: the model, the actor, the profile, the ceiling, the timeout and
the auto-approve merge. The same file now also pins the **absence** of the
nine forwarding copies the daemon used to keep.

**The runner got its own ratchet** (`kernel/modules/runs/runner/runctx_ratchet_test.go`,
three tests): the run engine asks the module for the run's context; the host's
port promises none of that vocabulary; and the port still carries the host's
own state.

**The probe.** Putting `SystemAgentFromCtx(ctx context.Context) bool` back
into the port made the port's ratchet fail:

```
--- FAIL: TestThePortCarriesNoRunContextVocabulary
    runctx_ratchet_test.go:81: the host's port still promises SystemAgentFromCtx(ctx context.Context) — the run context's vocabulary is the module's
```

The probe was reverted and the tree scanned for leftovers (none).

## 7. Verification

- **the compiler**: `go build ./...` and `go vet ./...` clean;
- **the ratchet probe** (§6);
- **the full test suite**: `go test ./...`;
- **the thirteen gates**: gofmt, build, vet, archcheck, deadcode, depscheck,
  sdkparity, structure-md, changelog-lint, docclaims, docsindex, staticcheck,
  test.

**No mutation run.** This slice redirects calls and removes a port's
promises; there is no rule whose reversal a mutation could expose. The
evidence is the compiler (every caller and the port's shape type-checked),
the ratchets and their probe, and the full suite — the same four legs
W3.76i, W3.76j and W3.76k stood on.

## 8. What is left

The daemon holds **205** public verbs and **6,124** production lines. The
daemon's remaining ctx doors are the host's own: the effective config, the
resume state, and the two doors a module binds a run through.

The rest of the W3 exit is unchanged: the daemon's composition root
(`compose.go`, 497 lines), its `Kernel` (`runtime.go`, 665), the accessors
family (7 files, 965) and the control plane (`kernel/controlplane`, 7,570).

Evidence: `architecture/285-w376l-the-run-engine-reads-its-own-context-evidence.md`
