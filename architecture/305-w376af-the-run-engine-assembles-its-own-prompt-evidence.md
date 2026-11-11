# W3.76af — the run engine assembles its own prompt

**Slice:** W3.76af (the fortieth of the runtime's dissolution)
**Base:** `686cb9ea` (the W3.76ae merge)
**Date:** 2026-11-11

---

## 1. The family

Before a run starts, the daemon's **prompt assembly** decides what the run
knows: the live identity, the operator profile, the taste exemplars, the
recalled memory, the resolved world entities and the activated skills — plus
the host-environment preamble that tells the model which machine it is on.

That assembly was the **runs module's** since W3.76e
(`kernel/modules/runs/prompt`). The daemon kept two **doors** over it:

```go
func (k *Kernel) BuildRunPrompt(runCtx context.Context, corr, actor, intent string,
    systemAgent bool, skillDirective skillapi.ActivationDirective) (string, []string) {
    return k.prompt.Build(runCtx, corr, actor, intent, systemAgent, skillDirective)
}

func (k *Kernel) InjectHostEnvironment(system string, tools map[string]toolapi.Tool) string {
    return k.prompt.BuildEnvironment(system, tools)
}
```

And the **run engine** — which lives *inside* the runs module, next to the
prompt service — asked the **host** for both of them through its
`KernelAPI` port:

```go
system, activatedSkillIDs := r.k.BuildRunPrompt(runCtx, corr, actor, intent, systemAgent, skillDirective)
system = r.k.InjectHostEnvironment(system, lc.Tools)
```

**A port entry that is a round trip inside one module is a promise the host
never had to keep.** The module has the body; the module calls it.

The proof that the wiring was wrong was already in the tree: the
**delegation engine** — which lives in the same module, one package over —
held the very same service:

```go
// kernel/modules/runs/delegation/host.go
Prompt *prompt.Service
```

Two neighbours in one module, one holding the service and one asking the
host for it.

## 2. What moved

The run engine holds the module's prompt service now, exactly the way the
delegation engine holds it:

- `Runner.prompt *prompt.Service`, handed over at construction
  (`runner.New(k, rosterSvc, k.runState, k.prompt)`);
- `r.prompt.Build(...)` and `r.prompt.BuildEnvironment(...)` replace the two
  host calls;
- a host that wired **no** prompt service leaves the engine quiet: it says
  `runner: the prompt assembly is not available` — **before** the run is
  registered, so a run that cannot be assembled leaves no live-run state
  behind.

The daemon's two doors are gone, and so are the two port entries. The daemon
keeps the **service** and the three-verb host port the module asks for (the
live identity, the bus, the clock) — plus **one** call of its own: the
context-selection manifest it derives from the run's journal
(`context_selection.go`).

The daemon's file was renamed with the doors it lost: `prompt_doors.go` →
`prompt_host.go` (it holds the host port and the config snapshot now).

## 3. The port

The run engine's host port lost **two** entries — `BuildRunPrompt` and
`InjectHostEnvironment` — and carries **32** now (it carried **34**). What
stayed is what the **host** owns: the run's setup helpers that are genuinely
the host's, the resume handshake, the roster's read and the host's own state.

## 4. The measurement

| what | before | after |
|---|---|---|
| the daemon's public verbs | 118 | **116** |
| the daemon's production lines | 5,418 | **5,417** |
| the daemon's production files | 55 | **55** |
| the app packages that import `kernel/runtime` | 40 | **40** |
| the run engine's host port (`KernelAPI`) | 34 entries | **32 entries** |
| the run engine's package | 935 | **970** |
| the prompt module's package | 6 files / 756 lines | **6 files / 756 lines** |

Two verbs off the daemon's surface, and the line count barely moves: the two
door bodies left the daemon, the engine's own refusal and the account of the
move arrived in the module, and the daemon's file kept its name and its
content minus the doors.

## 5. The ratchets — and the probe

**The daemon's side** (`kernel/runtime/prompt_ratchet_test.go`): the ratchet
that pinned "the daemon KEEPS the doors" now pins the opposite — the daemon
holds **no** verb over the assembly, and no production file of the daemon
may call `k.prompt.Build(` or `k.prompt.BuildEnvironment(` either. The
module's service is the module's to call.

**The module's side** (`kernel/modules/runs/runner/runner_prompt_ratchet_test.go`,
new): the port carries **no** prompt entry, the engine **holds** the module's
service, and a host that wired none leaves the engine quiet instead of
running a bare prompt.

The daemon's own surface ceiling followed the floor again: **118 → 116** (220
→ 201 → 194 → 179 → 173 → 166 → 164 → 156 → 152 → 148 → 143 → 135 → 129 →
125 → 121 → 118 → 116), and the two retired names joined the retired-verb
list.

**The probe** put the daemon's two doors back, put the two entries back into
the engine's port and made the engine ask the host again — the world before
this slice: the retired-verb ratchet, the surface ceiling (118 > 116), the
daemon's prompt-door ratchet, the module's port ratchet, the module's wiring
ratchet and the module's refusal ratchet all went red at once — **six**
ratchets in one shot, and nothing else broke. The probe was reverted and the
suites went green again.

## 6. Verification

- the compiler (`go build ./...`, `go vet ./...`) and `gofmt`;
- the daemon's tests, the run engine's tests, the prompt module's tests and
  the applications' tests;
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 7. What is left

The run's **prompt assembly** is the module's, and the daemon holds no verb
over it.

The daemon holds **116** public verbs and **5,417** production lines;
**forty** application packages still name it; the run engine's host port
carries **32** entries.

What is left in the daemon is the run's **remaining machinery**: the publish
helpers (`PublishIntentInterpreted`, `PublishContextFailureAnalysis`), the
plan runner's registration, the resume handshake and the prompt doors' last
host-side sibling — `ResolveRunModel` and `BuildLoopConfig`, the two doors
the run engine still opens for the host's own state.

The **standing package** itself still lives at `kernel/standing`, and its
eventual home is the **triggers module** (`kernel/modules/triggers`) — the
architecture gate already places both `kernel/cadence` and `kernel/standing`
in that module, so the family's boundary is drawn; the module's directory
does not exist yet, and the move is a path move (four production importers),
not a lifecycle one.
