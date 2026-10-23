# W3.35 — the ACP bridge's two reaches become a platform client and a port

**Date:** 2026-10-24
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the **acp_agent** tool — the bridge that delegates a task to an
*external* agent that speaks the Agent Client Protocol (Gemini CLI, Claude
Code's adapter, Codex, …). The tool is a PLUGIN.

## Two edges, and they were two different mistakes

The plugin reached into two places:

- `plugin-reach plugins/tools/acpagent -> kernel/acp` — the **ACP protocol
  client**. `kernel/acp` is the daemon's ACP **server**: the inbound adapter
  an IDE (Zed) drives. The tool used the *client* half of that package — and
  a plugin may not reach an adapter at all.
- `plugin-reach plugins/tools/acpagent -> kernel/modules/extensions/acpcatalog`
  — the **catalogue** of installed ACP agents: which agents this host has, and
  which launch command a chosen slug maps to. That is a MODULE's business, and
  a plugin may not name a module.

So the two halves are handled the two ways the architecture allows:

- The **client** is protocol infrastructure with no domain knowledge — it
  speaks JSON-RPC over an io.Reader/io.Writer and knows nothing about Agezt's
  agents, roster or journal. That is the definition of the **platform** layer,
  and a plugin may reach the platform. So the client **moved down** to
  `kernel/platform/acpclient` (with `git mv`, so the history follows the code),
  and the shared JSON-RPC wire and message framing moved with it — the server
  and the client speak the same wire, so the wire lives with the protocol.
  `kernel/acp` keeps the server only, and imports the platform package for
  the framing.
- The **catalogue** is a module's business, so it answers through the
  contract's **port** (`kernel/contract/acpapi`, three methods), the app-layer
  adapter (`kernel/app/acpbridge`) answers that port from the module, and the
  daemon hands the port over at the composition root.

## The ratchet went DOWN by TWO

**303 packages, 83 import violations, 13 call sites** — from 85/13. Both
`plugin-reach` edges are gone and **both stale allowlist entries were
deleted**.

## The rule that travels with the port

The catalogue's resolution is a **security rule** (CWE-78): the selector
arrives in agent/LLM tool input, which prompt injection can steer, so it is
treated strictly as a catalogue slug — it must name an installed catalogued
agent, and the resolved command is taken from the trusted catalogue, never
from the caller's string. A non-slug selector is refused rather than executed
as a raw shell line; a raw command is accepted only from the operator's
`AGEZT_ACP_AGENT_CMD`, and only when no selector was given.

That rule now lives behind the port, so it is measured **at the port and at
the tool**:

- the adapter never resolves a selector the catalogue refuses, and never runs
  one;
- the tool never resolves anything itself — it asks the port, and a tool with
  no port says so instead of calling into nothing;
- the wiring at the composition root supplies the port, and the port answers
  the **live** catalogue.

## The measurement gap this slice found

The adapter's test first compared the adapter's answers against the
catalogue's own answers. That comparison is **vacuous on a host where no ACP
agent is installed** — which is every CI machine: both sides answer "nothing
installed", so an adapter that lied about it passed. Two mutations survived
the first round for exactly that reason.

The test was **strengthened**: it now pins a KNOWN catalogue state (one agent
whose binary is the test binary itself), so a lying adapter is caught on any
host. The lesson is the same one W3.33 recorded, in a new shape: *a
comparison against a possibly-empty truth proves nothing about the liar*.

## The plugin's test no longer reaches the adapter

The plugin's tests used the daemon's real ACP **server** as the fake external
agent. Test imports do not enter archcheck, so that reach was invisible to
the gate — and a test-only reach is one careless edit away from a production
one. So the fake peer now **speaks the wire directly** (the same JSON-RPC the
client speaks), and the plugin's package reaches only the contract and the
platform layers, in production *and* in test.

The round-trip proof did not get weaker: it moved to the **server's** package
(`kernel/acp/roundtrip_test.go`), where the real client drives the real server
over a pair of pipes — and the server's package may reach the client.

## The proof

### Permanent tests

- **The port's shape** (`kernel/contract/acpapi/acpapi_test.go`): exactly
  three methods; no struct rows in or out; the three signatures pinned (a
  bool, a `[]string`, and the `(cmd, ok)` pair).
- **The adapter** (`kernel/app/acpbridge/catalog_test.go`): it answers the
  port's type; it answers the **live** catalogue against a known catalogue
  state; an unknown slug does not resolve; the operator's fallback does; and
  an **injected selector** (`gemini; rm -rf /`) never resolves to a command.
- **The plugin** (`plugins/tools/acpagent/contract_type_test.go`): the tool's
  catalogue field is the **contract's** port type, and the rule "this package
  must not name the catalogue module" is stated where a reader will find it.
- **The plugin's behaviour** (`plugins/tools/acpagent/acpagent_test.go`): the
  streamed answer is relayed; a failing agent is surfaced; an empty task is
  refused; a tool **without** a catalogue says so instead of calling into
  nothing; an unknown agent's answer names the installed agents; a catalogued
  agent resolves; and `New` registers the tool only with a default command or
  an installed agent.
- **The wiring** (`plugins/builtintools/acp_ports_test.go`): the built tool's
  answer comes from the catalogue the daemon handed over, and a daemon that
  hands over nothing leaves the tool disabled (or reporting that it has no
  catalogue).
- **The wiring at the root** (`cmd/agezt/acp_ports_test.go`): the daemon
  supplies the port, and it answers the live catalogue.

### Mutation run

**13 mutations: 13 caught.** Two **survived** the first round — the adapter
lying about "nothing is installed" and about "no installed slugs" — because
the test compared it against a possibly-empty catalogue; the test was
strengthened (a known catalogue state) and both were caught on the second
round. One mutation produced a **compile error** (the unused import the
mutation left behind) and was re-authored so the code still compiles.

| # | Mutation | Result |
|---|---|---|
| M01 | the catalogue answers that nothing is installed | survived the first round → test strengthened → caught |
| M02 | the catalogue names no installed slugs | survived the first round → test strengthened → caught |
| M03 | the catalogue ignores the selected slug | caught |
| M04 | the catalogue ignores the operator fallback | caught |
| M05 | the catalogue runs the selector as a command | caught |
| M06 | the bridge runs the selector as a command | caught |
| M07 | the bridge calls into a missing catalogue | caught |
| M08 | the bridge registers without a catalogue | caught |
| M09 | the catalogue port is not handed over | compile error → re-authored → caught |
| M10 | the tool is built without the catalogue port | caught |
| M11 | the port renames a method | caught |
| M12 | the bridge names the catalogue module again | caught — **by the archcheck gate** |
| M13 | the bridge names the ACP server again | caught — **by the archcheck gate** |

M12 and M13 are measured by the **gate** rather than by a test, and that is
not a weaker measurement: the module's functions and the client's constructor
are concrete, so a plugin that named them again would compile and pass every
test in the package. The gate is the only thing that can see it — the same
truth the W3.17, W3.33 and W3.34 slices recorded.

### Gates

The build, gofmt, vet, dead-code, archcheck (**303 packages, 83/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so
is the full test suite.

## What is deliberately left

- The **catalogue module** keeps everything about the catalogue: the curated
  agent list, the host detection, the registry client and the slug-only
  resolution rule. The adapter is a thin forwarding layer.
- The **ACP server** stays an inbound adapter (`kernel/acp`) — that is what
  it is. Only the client half moved.
- The remaining plugin-reach edges: the built-in tools reach `kernel/runtime`,
  `kernel/toolreg` and the plugin host (the registration seam — hard to
  invert), and the built-in channels reach `kernel/channelwire` (they carry
  the wiring's rows and the bus — a separate, expensive slice).
