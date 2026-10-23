# W3.33 — the guardian seeder's two reaches become one port

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the guardian seeder — the plugin that plants and reconciles the
daemon's internal fleet (the health responder, the trust-ceiling sweeper and
the rest). It writes to three stores: the roster, the standing orders and the
schedules.

## Two edges, one port

The seeder reached into two places, and both were the same mistake in
different clothes:

- `plugin-reach plugins/builtinguardians -> kernel/runtime` — the plugin
  carried a `kernelHost` adapter **inside itself** (`kernelhost.go`), holding
  the live `*runtime.Kernel`. A plugin may not hold the daemon's
  implementation.
- `plugin-reach plugins/builtinguardians -> kernel/modules/agents/roster` —
  the plugin named the roster module for two rows: `roster.Profile` and
  `roster.NoisePolicy`. Both are **aliases of the contract's own rows**
  (`agentapi.Profile` and `agentapi.NoisePolicy`) — the plugin was reaching
  into a module to name rows that already lived in the contract.

So:

- **New** `kernel/contract/guardianapi/guardianapi.go` — the seeder's **port**:
  the eleven methods the fleet needs across the roster, the standing orders
  and the schedules, carrying the contract's own rows.
- **New** `kernel/app/guardians/host.go` — the adapter that answers it. The
  adapter needs the live kernel, and the app layer is the only layer that may
  see it (the same rule the overseer, introspect and config adapters follow).
- `plugins/builtinguardians/kernelhost.go` — **gone**; the plugin keeps no
  implementation.
- `plugins/builtinguardians/builtinguardians.go` — `Host` is now the
  contract's port, and the rows it writes are the contract's rows
  (`agentapi.Profile`).
- `plugins/builtinguardians/builtinguardians_helpers.go` — the noise policy is
  `agentapi.NoisePolicy`.
- `cmd/agezt/guardians.go` — **new**: the daemon's seeding, in a named
  function, through the contract's port.

## The ratchet went DOWN by TWO

**299 packages, 87 import violations, 13 call sites** — from 89/13. Both
`plugin-reach` edges are gone and **both stale allowlist entries were
deleted**.

## The port is the whole of the reach

Eleven methods, and the test says so: a twelfth would be a new reach into the
daemon and it would have to be earned. Each signature is pinned too — the
profile edit answers the contract's `agentapi.Profile` (not a copy of it),
the standing order and the schedule answer their own contract rows, and the
cadence is a plain duration rather than a module's row.

## A nil host is refused, not called into

The seeder's host is a port, and a daemon that handed over nothing must hear
that the fleet was not seeded — not a panic on the first read. `SeedAll`
refuses a nil host, and the daemon's named seeding function is measured
against a **live** kernel: the guardians the seeder reports are in the
**roster** afterwards, and a daemon that seeded nothing would be caught.

## The rules that were never measured

The guardians' quiet defaults — silent on success, no memory writes — are the
operator's contract, and they had never been asserted. They are now.

## The proof

### Permanent tests

- **The port's shape** (`kernel/contract/guardianapi/guardianapi_test.go`):
  exactly eleven methods; the rows it carries are the contract's; the
  mutators take the contract's rows; the cadence is a duration.
- **The adapter** (`kernel/app/guardians/host_test.go`): a missing kernel is a
  nil port; the agent it adds is in the **live** roster; the edit it makes is
  **stored** — measured on a field the roster does not default, because the
  system-guardian defaults would fill a noise policy of their own and mask a
  dropped edit; an unknown agent is answered as "not found" and no error; the
  schedule it plants is in the **live** schedule store.
- **The seeder names the contract** (`plugins/builtinguardians/contract_type_test.go`):
  the seeder's host **is** the contract's port; the noise policy it writes is
  the contract's row; the port names no module.
- **The seeder's own rules** (`plugins/builtinguardians/seed_port_test.go`): a
  nil host is refused; the quiet defaults are pinned; the seeded profiles are
  the contract's rows.
- **The wiring** (`cmd/agezt/guardians_test.go`): the daemon's seeding is
  real — the reported guardians are in the live roster — and a daemon with no
  kernel is refused.

The plugin's own test files now speak the contract as well; the only module
left in the plugin is the roster's **validation rule** (`roster.Validate`),
which reads the edict's trust ceiling and is therefore not pure — it stays in
the module, and it is used in one test line.

### Mutation run

**13 mutations: 13 caught.** One mutation **survived** the first round — the
adapter dropped the profile edit, and the test did not notice, because the
roster's own system-guardian defaults had filled a noise policy of their own
and the assertion was looking at exactly that field. The test was
strengthened (the edit is now measured on a field the roster does not
default) and the mutation was caught on the second round. One mutation
produced a **compile error** (an unused import) and was re-authored.

| # | Mutation | Result |
|---|---|---|
| M01 | the port grows a twelfth method | caught |
| M02 | a missing kernel is not a nil port | caught |
| M03 | the agent is not added | caught |
| M04 | the profile edit is dropped | survived the first round → test strengthened → caught |
| M05 | every agent is found | caught |
| M06 | the roster lists nothing | caught |
| M07 | the schedule is not planted | caught |
| M08 | the guardians are not silent on success | caught |
| M09 | the guardians write memory | caught |
| M10 | a missing host is called into | caught |
| M11 | the seeder names the roster module again | caught — **by the archcheck gate**; the build still passes, because the module's alias is the same type, so the gate is the only thing that can see it |
| M12 | the daemon seeds with no host | compile error → re-authored → caught |
| M13 | the daemon seeds nothing | caught |

M11 is measured by the **gate** rather than by a test, and that is not a
weaker measurement: the roster module's `Profile` is an alias of the
contract's row, so a plugin that named it again would compile and pass every
test in the package. The gate is the only thing that can see it — the same
truth the W3.17 slice recorded.

The harness itself earned a fix during this slice, and the way it was caught
is worth recording: M11 edits one file three times, and the harness saved the
**mutated** text as the "original" for the second and third edits — so the
restore put the mutation back into the tree. The archcheck gate is what
noticed (`plugin-reach plugins/builtinguardians ->
kernel/modules/agents/roster` reappeared) and the harness was fixed to save
the pristine content once per file. The whole run was then repeated on the
corrected tree, and the result stands: 13 caught. The lesson for the harness
is in the memory file; the lesson for the slice is that **the gates are part
of the measurement**, and a mutation run that leaves a mark must be caught
before its numbers are believed.

### Gates

The build, gofmt, vet, dead-code, archcheck (**299 packages, 87/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so
is the full test suite.

## What is deliberately left

- The **roster module's validation rule** (`roster.Validate`) reads the edict's
  trust ceiling, so it is not pure and cannot live in the contract layer. It
  stays in the module — a separate slice, if it is ever wanted.
- The remaining plugin-reach edges: the ACP agent, the built-in tools, the
  built-in channels.
