# W3.38 — the registry's pre-Open phase becomes a contract's port

**Date:** 2026-10-24
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the tool registry's **pre-Open phase** — the one moment before the
daemon's kernel opens where a tool may still change the daemon's own
configuration.

## One edge, made of one thing

`plugin-reach plugins/builtintools -> kernel/runtime` — the built-in tools
plugin named the daemon's runtime, and it named it for exactly one reason: the
`code_exec` tool (M683) has to install its **script runner** into the daemon's
configuration before the kernel opens, because the script-tool forge (M794)
executes forged tools through the code-execution tool's own runner and that
runner must be live from the very first run.

The hook's signature was the leak:

```go
PreOpen func(tool toolapi.Tool, cfg *runtime.Config)
```

`*runtime.Config` is the daemon's **live configuration type** — a plugin may
not name it, so the edge had to exist in the allowlist. The hook did not need
the configuration; it needed **one verb** on it.

## What moved

The verb moved to the contract layer, and the hook now answers the contract's
port:

- **`kernel/contract/toolregapi`** (new, L1, pure): `ScriptRunner` — the
  runner the forge executes a forged tool through, declared by the contract
  itself — and `Config`, the pre-Open port, with **exactly one method**:
  `SetScriptRunner(r ScriptRunner)`.
- **`kernel/toolreg`**: `Spec.PreOpen` now takes `toolregapi.Config` instead of
  `*runtime.Config`. The registry — the one place that knows both the contract
  and the daemon — wraps the daemon's configuration in a small adapter
  (`runtimeConfig`) and hands the port to the hook.
- **`plugins/builtintools`**: the hook answers the port
  (`cfg.SetScriptRunner(ce)`), and the plugin's import of `kernel/runtime` is
  **gone**.

The daemon's runner type lives in a module, and a contract may not name a
module — so the contract declares **its own** runner interface. The two have
the same method set (`RunScript(ctx, language, code, inputJSON) (string, bool,
error)`), so the daemon's runner satisfies the contract's one and the value is
assigned directly. The registry never names the module that declares the
daemon's own interface.

## The ratchet went DOWN

**306 packages, 80 import violations, 13 call sites** — from 81/13. The
`plugin-reach` edge is gone and the **stale allowlist entry was deleted**
(`plugin-reach plugins/builtintools -> kernel/runtime`).

One `plugin-reach` edge remains in the whole tree
(`plugins/builtintools -> kernel/toolreg`), and it is the registration seam:
the plugin *registers into* a registry the daemon owns, which is a dependency
in the opposite direction of the one the rule forbids. It is the next slice's
work.

## The rules that travelled, and the ones that stayed

- **Moved with the verb**: the *shape* of the reach — the port has exactly one
  method, the runner exactly one, with the signature pinned argument by
  argument. A second method on the port would be a new reach into the daemon
  and would have to be earned here.
- **Stayed with the daemon**: the *runtime* rules — when the pre-Open phase
  runs (after the tools are built, before `runtime.Open`), which spec's hook
  runs at all (only the specs that actually built a tool), and what the daemon
  does with the installed runner. The registry keeps the phase; the plugin
  keeps only the verb.

## The proof

### Permanent tests

- **The contract** (`kernel/contract/toolregapi/toolregapi_test.go`): the port
  is an interface with **exactly one** method, `SetScriptRunner`, taking the
  contract's own runner and answering nothing; the runner has **exactly one**
  method, `RunScript`, with its four arguments and three answers pinned by
  type. (An interface method's type carries **no receiver** — `m.Type.In(0)`
  is the first *argument*.)
- **The registry** (`kernel/toolreg/toolreg_test.go`): the pre-Open phase still
  reaches every spec that declared a hook, in registration order, alongside the
  late phase.
- **The plugin** (`plugins/builtintools/tools_test.go`): the wiring is measured
  against the **built instance** — after `ApplyPreOpen`, `cfg.ScriptRunner` is
  the `*codeexec.Tool` the spec built — and the **gated-off** case is measured
  too: with `AGEZT_SANDBOX=off` the tool is never built, so the pre-Open phase
  must leave `cfg.ScriptRunner` **nil** rather than install a runner for a
  tool that does not exist.

### Mutation run

**6 mutations: 6 caught.** No mutation survived, none produced a compile error
after the re-authoring below, and none was skipped on a stale anchor.

| # | Mutation | Result |
|---|---|---|
| M01 | the plugin's pre-Open hook does not install the runner | caught |
| M02 | the plugin's pre-Open hook is gone entirely | caught |
| M03 | the registry's adapter drops the runner | caught |
| M04 | the pre-Open phase never runs | caught |
| M05 | the contract's port gains a method | caught |
| M06 | the plugin names the runtime again | caught — **by the archcheck gate** |

M02 was **re-authored** after the first round: deleting the hook left the
contract import unused, so the mutation produced a *compile error* rather than
a measured failure. Deleting the import with it made the mutation a real
"the plugin no longer takes part in the pre-Open phase" — and the plugin's own
test caught it, because the forged-script runner would never be installed.

M06 is measured by the **gate**: the daemon's configuration type is concrete,
so a plugin that named it again would compile and pass every test in the
package. The gate is the only thing that can see it — the same truth the
W3.32, W3.33, W3.34, W3.35 and W3.37 slices recorded.

### Gates

The build, gofmt, vet, dead-code, archcheck (**306 packages, 80/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so is
the full test suite.

## What is deliberately left

- The **registry** keeps the phase and the adapter: it is the one place that
  knows both the contract and the daemon, and the daemon's configuration type
  is not pure — it cannot move to the contract layer.
- The **remaining plugin-reach edge** is the registration seam
  (`plugins/builtintools -> kernel/toolreg`): the registry's rows carry the
  live kernel, so splitting them is a bigger slice — the registry's own rows
  have to be broken apart first, one port family at a time.
