# W3.34 — the briefing rows move to the contract layer

**Date:** 2026-10-24
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the pulse engine's briefing — the composed message the daemon
pushes at the operator when something is worth saying. The **brief** is
composed by the pulse module and delivered to surfaces: the daemon log, the
Telegram chat, the Slack channel, the push surface, the webhook. Those
surfaces are the built-in channels, and the built-in channels are a **PLUGIN**.

## One edge, and it was a row's edge

`plugin-reach plugins/builtinchannels -> kernel/pulse` — the plugin named the
pulse **module** for one thing: the row. `pulse.Brief` is the message the
engine composes and the plugin delivers, and `pulse.BriefSink` is the seam
between them. The plugin built its sinks with `pulse.SinkFunc`, carried them
as `pulse.BriefSink`, and formatted `pulse.Brief`.

A plugin may reach the **contract** and the **platform** layers and nothing
else. A row that a plugin names has to live in the contract layer — otherwise
the plugin reaches into a module to name something that was never the
module's to own.

So:

- **New** `kernel/contract/pulseapi/pulseapi.go` — the **row** and the sinks
  that carry it: `Brief`, `Disposition` (with its five values), `BriefSink`,
  `LogSink`, `SinkFunc` and `MultiSink`. The composition stays behind — the
  module keeps `composeBrief`, `composeDigest`, the quiet-hours window and the
  engine's loop, because those are the implementation.
- `kernel/pulse/briefing.go` and `kernel/pulse/pulse.go` — every name the
  module used is now an **alias** of the contract's row, so there is one row
  and one spelling of it. The module's own `Disposition` is the contract's
  `Disposition`.
- `plugins/builtinchannels/*` — eleven files now name `pulseapi` instead of
  `pulse`.
- `kernel/channelwire/channelwire.go` — the **channel factory layer** named
  the pulse module for the same row (`Built.Sink` is a brief sink), so it
  names the contract now.

## The ratchet went DOWN by TWO

**300 packages, 85 import violations, 13 call sites** — from 87/13. Both edges
are gone and **both stale allowlist entries were deleted** (a stale entry
makes archcheck red — the debt is paid, the list shrinks):

- `plugin-reach plugins/builtinchannels -> kernel/pulse` — the plugin named
  the module for the row.
- `cross-module kernel/channelwire -> kernel/pulse` — the factory layer named
  the module for the row as well, and that one was a **cross-module**
  violation: the channel wiring is below the pulse module in the layering,
  and reaching up into it for a row was the wrong direction.

The second edge is the more interesting one, because the row's move was what
made it closable. `kernel/pulse` drags `kernel/agent` into its transitive
dependencies, and the factory layer's own package doc says why it sits above
`kernel/channel`: *"importing pulse into kernel/channel would drag
kernel/agent into every transport plugin's transitive deps."* The layer was
already carrying that weight for one row. With the row in the contract, the
factory layer names the contract and the drag is gone.

## The wire matters here: the brief is what the operator reads

The brief is delivered to a human — printed by the CLI, rendered by the Web
UI, pushed into a chat channel — so its JSON keys are a contract with every
consumer outside the package. The wire had never been measured. It is now
pinned key for key: the four always-present keys (`title`, `disposition`,
`issue_key`, `items`) and the two that are **dropped when empty**
(`body`, `correlation_id`) — an empty `"body": ""` would be a different
message from the same brief written by another version.

The **dispositions** are wire too — they are stored, printed and branched on.
All five are pinned by value: `drop`, `digest`, `notify`, `alert`, `act`. A
renamed one would silently change what happens to a delta.

And the **format** the operator reads is pinned in both its forms: the
daemon log's `📣 BRIEF [notify] the fleet is quiet` line (with the body
indented under it) and the chat surface's `📣 three things` message (with the
body under the title). Those are two different renderings of one row, and
both were unpinned.

## The sinks' rules are the operator's contract too

- A sink with **no writer** is a no-op, not a panic — the daemon's stdout may
  be closed.
- A **multi sink** fans out to **every** sink even when one fails, and answers
  the **first** error: a Telegram outage must not suppress the daemon log
  line. This is exactly the rule the slice would have lost if the sink had
  stayed a module row nobody measured.
- A **nil sink** in the fan-out is skipped, not called into.

## The proof

### Permanent tests

- **The contract** (`kernel/contract/pulseapi/pulseapi_test.go`): the brief's
  wire shape (four always-keys, two dropped-when-empty, and the exact key
  count); the five disposition values; the log sink's rendering in both forms
  (with and without a body, and the body's indentation); the nil-writer
  no-op; the sink func; the multi sink's fan-out, its nil-sink skip and its
  first-error answer.
- **The module** (`kernel/pulse/contract_type_test.go`): every row the module
  keeps **is** the contract's row — by type identity, not by shape — and the
  five dispositions are the contract's by value. A copy would drift, and a
  drifted brief would no longer fit the brief a plugin delivers.
- **The plugin** (`plugins/builtinchannels/contract_type_test.go`): the
  channels name the contract's rows, and the rule "the plugin must not name
  the pulse module" is stated where a reader will find it (the archcheck gate
  is what enforces it).
- **The factory layer** (`kernel/channelwire/contract_type_test.go`): the
  channel wiring's `Built.Sink` is declared as the **contract's** port — the
  declared field type is measured, not the concrete value stored in it — and
  the rule "the factory layer must not name the pulse module" is stated where
  a reader of the package will find it.
- **The format** (`plugins/builtinchannels/brief_format_test.go`): the chat
  surface's message in both forms — the marker, the title, the body.

### Mutation run

**14 mutations: 14 caught.** No mutation survived, none produced a compile
error, and none was skipped on a stale anchor.

| # | Mutation | Result |
|---|---|---|
| M01 | the brief wire renames the issue key | caught |
| M02 | the notify disposition is renamed | caught |
| M03 | the log sink drops its marker | caught |
| M04 | the log sink does not indent the body | caught |
| M05 | the log sink calls a missing writer | caught |
| M06 | the multi sink stops at the first failure | caught |
| M07 | the multi sink calls a missing sink | caught |
| M08 | the sink func drops the brief | caught |
| M09 | a disposition points at another value | caught |
| M10 | the brief message loses its marker | caught |
| M11 | the brief message drops the body | caught |
| M12 | the channels name the pulse module again | caught — **by the archcheck gate** |
| M13 | the brief wire renames the items key | caught |
| M14 | the channel wiring names the pulse module again | caught — **by the archcheck gate** |

M12 and M14 are measured by the **gate** rather than by a test, and that is
not a weaker measurement: the module's `Brief` is an alias of the contract's
row, so a plugin (or the factory layer) that named the module again would
compile and pass every test in the package. The gate is the only thing that
can see it — the same truth the W3.17 and W3.33 slices recorded.

M14 is worth a note of its own: it was a **cross-module** violation, so it
was not even in the plugin's reach — the factory layer reached *up* into the
pulse module for a row it could have named in the contract. The mutation is
caught by the same gate that caught the original edge, which is exactly the
point: the gate remembers the rule, not just the instance.

### Gates

The build, gofmt, vet, dead-code, archcheck (**300 packages, 85/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so
is the full test suite.

## What is deliberately left

- The **composition** stays in the module: `composeBrief`, `composeDigest`,
  the quiet-hours window, the salience rules and the engine's loop. They are
  the implementation, not the row.
- The **quiet-hours rule** (`pulse.QuietHours`) stays in the module and the
  alerter still reaches for it — it is a rule, not a row, and it is not pure
  in the contract sense.
- The remaining plugin-reach edges: the built-in channels still reach
  `kernel/channelwire` (they carry the wiring's `Deps` and `Built` rows and
  the `*bus.Bus` — a separate, expensive slice), the built-in tools reach
  `kernel/runtime`, `kernel/toolreg` and `kernel/plugin` (the registration
  seam — hard to invert), and the ACP agent reaches `kernel/acp` and the ACP
  catalogue.
