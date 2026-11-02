# W3.76m — the run engine's last two context doors

**Slice:** W3.76m (the twenty-first of the runtime's dissolution)
**Base:** `bbf14cd5` (the W3.76l merge)
**Date:** 2026-11-02

---

## 1. The family

W3.76l moved the run context's **read** vocabulary out of the host: fifteen
forwarding copies the daemon kept (`WakeContextSource`, `MaxCostFromCtx`,
`WithActorCorrelation` and their siblings) are gone, and the run engine
reads the run's own decoration through the module that owns it
(`kernel/modules/runs/runctx`).

Two doors of the same kind were left behind, because they sat in a different
file and nobody looked at them together:

- `AgentSlugFromCtx` — the named agent's slug on the run context;
- `AgentRetryPolicyFromCtx` — the named agent's retry policy.

Both were **pure forwarding copies** of the module's names
(`runctx.AgentSlugFromCtx`, `runctx.AgentRetryPolicyFromCtx`), both were
entries of the host's port, and both were read **only** by the run engine —
which lives inside the module that owns the vocabulary.

This slice closes the family: **the run engine's context doors are all gone.**

## 2. What moved

```go
lc.Agent = runctx.AgentSlugFromCtx(runCtx)          // runner.go
slug := runctx.AgentSlugFromCtx(ctx)                 // runner_lifecycle.go
agentSlug := runctx.AgentSlugFromCtx(ctx)            // runner_retry.go
if pol, ok := runctx.AgentRetryPolicyFromCtx(ctx); ok && pol.MaxAttempts > 1 {
```

The two daemon copies and the two port entries are gone. The run engine's
port now carries **no** run-context vocabulary at all.

## 3. The measurement

| measure | before | after |
|---|---:|---:|
| the daemon's public verbs | 205 | **203** |
| the daemon's production lines (non-blank) | 6,124 | **6,113** |
| the daemon's production files | 54 | 54 |
| the run engine's port (`runner.KernelAPI`) | 66 methods | **64** |
| the contract's rows / the control plane / the app layer's production files | — | **+0 / +0 / +0** |

Since W3.76k the daemon's public surface has gone **238 → 203** verbs and
**6,281 → 6,113** production lines, and the run engine's port **81 → 64**.

## 4. What the daemon still keeps — and why

The scan of the daemon's remaining doors found one family that stays on
purpose: the doors whose **state** is the host's.

- `BuildLoopConfig` — the daemon projects its *effective* configuration and
  wires its live collaborators (the tools, the tool phases, the bus, the
  policy hook, the artifact offload port, the model catalog); the module
  holds the rule (W3.76g).
- `BuildRunPrompt` / `InjectHostEnvironment` — the same shape for the prompt
  module (W3.76e).
- `WithAgentProfile` / `WithMaxCost` — the doors a **module** binds a run
  through without naming the runs module.
- `ResumeOwnedKindFromCtx` / `ResumeSeedFromCtx` / `ResumeCheckpointFn` /
  `ResolveRunModel` / `DisableHeuristicBypass` — the host's own state: the
  resume ticket, the seed, the effective configuration.

The distinction is the one the W3.76l evidence drew: the **vocabulary** is
the module's; the **state** is the host's. A door that answers from the
host's state is a real door; a door that forwards to the module's own
function is a second spelling.

## 5. The ratchets — and the probe

The daemon's ratchet now watches its **whole package** for the eleven
vocabulary doors (the check was file-scoped, and the vocabulary's doors were
split over two files — a file-scoped check would have watched only one of
them). The run engine's ratchet grew the two names into both of its lists:
the module's names the engine must use, and the port entries the host must
not promise.

**A third ratchet changed honestly.** The W3.76j ratchet that pins the
port's shape (`TestTheHostStillImplementsThePort`) listed `AgentSlugFromCtx`
among the runner's entry points — it was, until this slice. The list keeps
the two resume doors (the engine's real entry points into the host) and
pins the two vocabulary names' **absence**.

**The probe.** Putting `AgentSlugFromCtx(ctx context.Context) string` back
into the host's port made the port's ratchet fail:

```
--- FAIL: TestThePortCarriesNoRunContextVocabulary
    runctx_ratchet_test.go:85: the host's port still promises AgentSlugFromCtx(ctx context.Context) — the run context's vocabulary is the module's
```

The probe was reverted and the tree scanned for leftovers (none).

## 6. Verification

- **the compiler**: `go build ./...` and `go vet ./...` clean;
- **the ratchet probe** (§5);
- **the full test suite**: `go test ./...`;
- **the thirteen gates**: gofmt, build, vet, archcheck, deadcode, depscheck,
  sdkparity, structure-md, changelog-lint, docclaims, docsindex, staticcheck,
  test.

**No mutation run.** This slice redirects two calls and removes two port
entries; there is no rule whose reversal a mutation could expose. The
evidence is the compiler, the ratchets and their probe, and the full suite.

## 7. What is left

The daemon holds **203** public verbs and **6,113** production lines. Its
public surface has **no dead verb** and **no vocabulary door** left: every
public verb is a door somebody opens, and the ones that stay open from the
host's own state.

The rest of the W3 exit is unchanged: the daemon's composition root
(`compose.go`, 497 lines), its `Kernel` (`runtime.go`, 665), the accessors
family (7 files, 965) and the control plane (`kernel/controlplane`, 7,570).

Evidence: `architecture/286-w376m-the-run-engines-last-two-context-doors-evidence.md`
