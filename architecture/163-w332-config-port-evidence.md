# W3.32 — the config tool's reach into the runtime becomes a port

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the `config` tool — the agent's window into the Config Center. It
reads and writes the settings the daemon, the `agt config` CLI and the HTTP
routes share, registers a skill's own settings section, and it can write a
setting two ways: **live** (rebuilding the provider in place) or
**agent-scoped** (on one agent's profile).

This slice is the first one in this run that moves **no row at all**. Every
slice so far carried a row from a module to the contract layer; this one is
about the **shape of a reach** — and that is why the port's shape has to be
pinned as carefully as a row's wire.

## What the tool reached for

The tool is a **plugin**, so it may reach the contract and the platform
layers and nothing else. It reached into `kernel/runtime` for exactly three
things:

| What | Where it came from | What it is now |
|---|---|---|
| the roster's profile edit | `(*runtime.Kernel).UpdateProfile` | a port method |
| the provider rebuild | `(*runtime.Kernel).Reload` | a port method |
| the run's own config overrides | `runtime.AgentConfigOverrides(ctx)` — a package-level context reader | a port method |

So:

- **New** `kernel/contract/configapi/configapi.go` — the port, with those
  three methods and nothing else. It carries the contract's own
  `agentapi.Profile` row: the roster module's `Profile` **is** that row's
  alias, so the kernel's method already fits the port without a copy.
- **New** `kernel/app/configtool/source.go` — the adapter that answers the
  port. Answering it needs the live kernel, and the app layer is the only
  layer that may see it (the same rule the overseer and introspect adapters
  follow).
- `plugins/tools/config/config.go` — the tool's `kernel` field is now the
  contract's **port**, and the tool does not import the runtime at all.
- `kernel/toolreg.KernelDeps.Config` — the registry carries the port.
- `cmd/agezt/tooldeps.go` — the composition root hands the port over.
- `plugins/builtintools/inject.go` — the spec binds `d.Config`.

## The ratchet went DOWN

**297 packages, 89 import violations, 13 call sites** — from 90/13. The
`plugin-reach plugins/tools/config -> kernel/runtime` edge is gone and its
**stale allowlist entry was deleted**.

## The port's shape is the contract

Three methods, each of them a deliberate narrowing of what the kernel offers:

- `UpdateProfile(ref, mutate) (agentapi.Profile, bool, error)` — the kernel's
  own signature. The roster module's `Profile` is the contract row's alias,
  so the kernel already satisfies the port.
- `Reload() error` — the kernel answers `(*catalog.Catalog, bool, error)`.
  The port answers **the error only**: the catalogue the reload produced is
  the daemon's business, and the plugin may not name the catalog module. This
  is the shape that made an adapter necessary — and necessary is the point:
  a plugin that could ask for the catalogue would have to reach a module.
- `AgentConfigOverrides(ctx) map[string]string` — the run's overrides, which
  the daemon reads from its own context convention.

The port is pinned: it has **exactly three methods**, and the tests assert
each signature — including that `Reload` answers the error only.

## The nil-port rule, stated twice and measured twice

A daemon with no kernel must hand over a **nil** port, not a port wrapping a
nil implementation. The check is on the **concrete** type, in a named
function, in two places — at the composition root (`configSource`) and in the
adapter (`NewKernelSource`) — and each is measured on its own: a missing
kernel yields a nil port, and the tool degrades instead of dereferencing
nothing.

The tool's degradation is now explicit and measured: **no daemon → no agent
overrides** on the read path, and **"agent-scoped config needs a live
kernel"** on the write path — the same posture the write path already took.

## The behaviour that changed, recorded honestly

The run's overrides used to be read by a package-level function, so a tool
built **without** a daemon still saw them. They now arrive through the port,
so a daemon-less tool sees none. In the daemon nothing changes: the adapter
delegates to the very same reader, and the daemon always binds the port. The
only difference is for a tool that was never wired to a daemon at all — and
there, "no daemon, no overrides" is the truth rather than an accident.

## The proof

### Permanent tests

- **The port's shape** (`kernel/contract/configapi/configapi_test.go`): the
  port has exactly three methods, and each one's signature is pinned — the
  mutator's type, the `(Profile, bool, error)` answer, `Reload`'s error-only
  answer, the context the overrides are read with.
- **The adapter** (`kernel/app/configtool/source_test.go`): a missing kernel
  is a nil port; the profile edit reaches the **live** roster (a profile
  added to the roster is the profile the port edits, and the edit is really
  stored); an unknown agent is answered as "not found" and no error; the
  reload really reloads; the overrides are read from the run's context and
  are a **copy** the caller may mutate.
- **The tool names the contract** (`plugins/tools/config/contract_type_test.go`):
  the tool's port field **is** the contract's port, and `SetKernel` takes the
  contract's port — a plugin can never be handed the kernel itself.
- **The tool's behaviour through the port**
  (`plugins/tools/config/port_test.go`): a daemon-less tool has no overrides
  and refuses the agent-scoped write; the agent-scoped read answers the
  override the run carries; an unknown agent is reported by name; a
  **live-apply** field reloads the daemon exactly once and says so; a
  **restart-apply** field does **not** reload it; and a failed reload is
  reported instead of a silent success.
- **The wiring, measured twice**: the composition root's own test
  (`cmd/agezt/tooldeps_test.go`) asserts the port is present and reaches the
  **live** roster — a daemon that dropped it would leave the tool unable to
  write an agent's override on every live box, while every hand-built test
  stayed green; and the built-in tools' test
  (`plugins/builtintools/config_ports_test.go`) asserts the registered tool
  is bound to that port, that its agent-scoped write really lands in the
  live roster, and that a daemon which hands over only the kernel leaves the
  tool reporting that it needs a live kernel.

The plugin's own test was **rebuilt on a fake port** — the point of a port
is that the tool is testable without a daemon — and the real delegation is
covered by the adapter's test.

### Mutation run

**19 mutations: 17 caught, 2 equivalent.** Seven mutations were re-authored
after they produced a **compile error** rather than a kill (an unused
variable, an unused import) or a **stale anchor** (the indentation of an
anchor), and all seven were rewritten so the code still compiles → caught.

| # | Mutation | Result |
|---|---|---|
| M01 | the adapter does not check a missing kernel | caught |
| M02 | the adapter drops the profile edit | caught |
| M03 | the adapter finds every agent | caught |
| M04 | the adapter turns an unknown agent into an error | caught |
| M05 | the adapter reports no overrides | caught |
| M06 | the adapter hands out the overrides without copying them | caught |
| M07 | the adapter drops the reload's error | **equivalent** — the kernel's reload does not fail in a test, so the provider rebuild's failure is not reachable from here |
| M08 | the tool's `SetKernel` drops the port | caught |
| M09 | the read path calls into a missing port | caught |
| M10 | the write path calls into a missing port | caught |
| M11 | an unknown agent is not reported | caught |
| M12 | the agent-scoped read ignores the overrides | caught |
| M13 | the live apply does not reload | caught |
| M14 | every field reloads the daemon | caught |
| M15 | a failed reload is swallowed | caught |
| M16 | the registered tool is bound without the port | caught |
| M17 | the daemon supplies no port | caught |
| M18 | the composition root does not check a missing kernel | **equivalent** — the adapter's own check answers it, so the two checks are defence in depth |
| M19 | the port grows a fourth method | caught |

The two **equivalent** mutations are recorded as such rather than dressed up
as catches: the provider rebuild's failure is not reachable from a test
(M07), and the composition root's nil check is redundant with the adapter's
(M18) — defence in depth, kept on purpose.

### Gates

The build, gofmt, vet, dead-code, archcheck (**297 packages, 89/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so
is the full test suite.

One unrelated test — `kernel/controlplane`'s
`TestScheduleAddAndEditContinuousCycle` — flaked once during a full-suite
run under parallel load. It is a timing-sensitive schedule test, it passes
in isolation and in three consecutive runs of its own package, and it does
not touch the config tool; it is recorded here rather than hidden.

## What is deliberately left

- The **runtime** still owns the context convention the overrides travel
  through (`WithAgentProfile` / `AgentConfigOverrides`) — the run context is
  the runtime's own, and the port reads it from there.
- The **built-in tools'** own edges to the runtime and the tool registry stay:
  that plugin *is* the composition binder.
- The remaining plugin-reach edges: the ACP agent, the built-in tools, the
  built-in channels, the guardians.
