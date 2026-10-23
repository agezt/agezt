# W3.37 — the plugin host's reach becomes a contract's rows and a spawn port

**Date:** 2026-10-24
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the built-in tools plugin's host of **out-of-process plugins** — the
child processes the daemon spawns, whose tools register into the daemon's tool
registry.

## One edge, and it was made of two different things

`plugin-reach plugins/builtintools -> kernel/modules/extensions/plugin` — the
plugin named the module that owns the child process. It named it for two
different reasons, and the two want different answers:

- **The operator's declarations.** `AGEZT_PLUGINS` (the plugins and their
  prefixes), `AGEZT_PLUGIN_PINS` (the BLAKE3 digests their binaries must
  match) and `AGEZT_PLUGIN_TOOLS` (the tools each plugin may advertise) are
  **pure rows with pure parsers**. A row a plugin names belongs in the
  contract layer — so they moved there, whole, with `git mv`: the
  `PinSpec`, the `ToolAllowlistSpec` and the `PluginSpecEntry`, their three
  parsers, and the BLAKE3 pin format check (which the parser that accepts a
  pin and the host that verifies the binary against it must agree on — one
  spelling of it).

- **The spawn itself.** Launching the child — the binary pinning, the frame
  caps, the callback bridge, the teardown — is the host's implementation, and
  it lives in the module. A plugin may not name a module, so the spawn goes
  through the contract's **port** (`pluginapi.Spawner`, one method) and the
  daemon hands the implementation over at the composition root. The
  app-layer adapter (`kernel/app/pluginhost`) answers that port from the
  module.

The running plugin answers **three verbs** through the port's other side
(`pluginapi.Spawned`): its tools, their declared capabilities, and the close.
The host's own verbs — invoke, reload, liveness — stay with the host: a
plugin that could name them could drive the process directly.

## The ratchet went DOWN

**305 packages, 81 import violations, 13 call sites** — from 82/13. The
`plugin-reach` edge is gone and the **stale allowlist entry was deleted**.

## The rules that travelled, and the ones that stayed

The security rules of this area are the point of the whole thing, and the
slice had to be careful about which of them moved:

- **Moved with the rows**: the *slug*-style rules — a malformed pin is
  refused, a **duplicate prefix is a hard error** (a duplicate pin would
  otherwise silently keep the last hash, shadowing the operator's intended
  one — M216), an empty tool allowlist is refused rather than read as "deny
  everything".
- **Stayed with the module**: the *binary* rules — the BLAKE3 digest of the
  file is compared against the operator's pin and a mismatch is a refusal
  (M1.ff), an oversized frame tears the plugin down rather than letting an
  untrusted child drive the host to OOM (M177), a callback the operator did
  not wire is rejected, and the plugin's environment is the **scrubbed** base
  plus exactly the variables `AGEZT_PLUGIN_ENV` grants.

The split is not cosmetic: the adapter is a thin forwarding layer, and the
host's rules are still the host's.

## The test moved with the promise — and split in two

The plugin's old test proved the strongest possible thing: it built a real
probe plugin, spawned it, and asked the **child process** what it saw in its
environment. That proof needed the host's module — which the plugin may not
name, and which the gate could not see anyway (test imports do not enter
archcheck).

So the promise was **split into the two halves that make it**, and both are
measured:

- The **plugin side** asks for an environment — and the plugin's test
  measures the **request** it hands the port: no `OPENAI_API_KEY`, no
  `AGEZT_VAULT_PASSPHRASE`, the granted variable present, `PATH` present,
  and the operator's **pin** and **tool allowlist** carried along.
- The **host side** hands the child that environment — and the app layer's
  test measures it against a **real child process** (the same probe, now a
  shared fixture in `kernel/internal/testfixtures`), including that a binary
  which does not match the operator's pin **does not start**.

Neither half reaches across a forbidden edge, and together they cover what
the old single test covered.

## The proof

### Permanent tests

- **The contract** (`kernel/contract/pluginapi/`): the pin parser's three
  refusals (malformed digest, duplicate prefix, empty spec) and the
  allowlist parser's four (no `=`, empty prefix, empty tool list, duplicate
  prefix); the pin format check; the stale-entry diff helpers; the plugin
  spec's eleven parsing cases (quoting, whitespace, duplicates, unterminated
  quotes); and the **ports' shape** — the spawn port has exactly one method,
  the running plugin answers exactly three verbs, and the spawn request
  carries no concrete host type.
- **The module** (`kernel/modules/extensions/plugin/`): the host's own rules
  stay measured where they live — the pin verification (match, mismatch,
  malformed, missing file) and the allowlist enforcement at spawn.
- **The plugin** (`plugins/builtintools/`): the environment it asks for, the
  pin and allowlist it carries, the tool registration under the prefix, the
  close, and the **hard error** when the daemon hands over no port.
- **The host** (`kernel/app/pluginhost/`): the child really gets the
  requested environment (measured against a real child process), and a binary
  that does not match the operator's pin does not start.
- **The wiring** (`cmd/agezt/plugin_ports_test.go`): the daemon supplies the
  port.

### Mutation run

**12 mutations: 12 caught.** No mutation survived, none produced a compile
error, and none was skipped on a stale anchor.

| # | Mutation | Result |
|---|---|---|
| M01 | a malformed pin is accepted | caught |
| M02 | a duplicate pin prefix is accepted | caught |
| M03 | a duplicate tool-allowlist prefix is accepted | caught |
| M04 | the pin does not reach the spawn request | caught |
| M05 | the tool allowlist does not reach the spawn request | caught |
| M06 | the child gets the daemon environment | caught |
| M07 | the granted variables are dropped | caught |
| M08 | the spawn request is built empty | caught |
| M09 | the spawn port is not handed over | caught |
| M10 | the child does not get the requested environment | caught |
| M11 | the binary pin is not enforced | caught |
| M12 | the plugin names the host module again | caught — **by the archcheck gate** |

M12 is measured by the **gate**: the module's `Spawn` and its parsers are
concrete, so a plugin that named them again would compile and pass every
test in the package. The gate is the only thing that can see it — the same
truth the W3.17, W3.33, W3.34 and W3.35 slices recorded.

### Gates

The build, gofmt, vet, dead-code, archcheck (**305 packages, 81/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so
is the full test suite.

## What is deliberately left

- The **host module** keeps the process: the pin verification, the frame
  caps, the callback bridge, the wire protocol and the teardown.
- The **remaining plugin-reach edges** are the registration seams: the
  built-in tools still reach `kernel/toolreg` (the tool registry — its rows
  carry the live kernel, so splitting them is a bigger slice) and
  `kernel/runtime` (the live kernel, through the registry's configure
  phase). Those are the hard ones to invert: the plugin *registers into* a
  registry the daemon owns, which is a dependency in the opposite direction
  of the one the rule forbids.
