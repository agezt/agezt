# W3.36 — the channel factory layer moves down to the platform layer

**Date:** 2026-10-24
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the **channel factory layer** — the seam between the daemon and the
channel plugins. Each channel *kind* registers a `Factory` that builds its
configured instances (the default account plus every `#label` account), and the
daemon walks the manifest registry calling `BuildKind` per kind.

## The edge, and why it was in the wrong place

`plugin-reach plugins/builtinchannels -> kernel/channelwire` — the built-in
channels plugin named the factory layer. The layer was classified as a
**module** (`kernel/channelwire`, L3, module "channels") in
`tools/archcheck/layers.json`, and a plugin may not name a module.

But the layer was never a module's business:

- it depends only on the **contract** layer (`kernel/channel` — the channel
  vocabulary; `kernel/contract/pulseapi` — the briefing row) and the
  **platform** layer (`kernel/bus`, `kernel/settings`);
- it carries **no domain knowledge** beyond the channel vocabulary itself,
  which is the contract layer's;
- it is the **wiring** between the daemon — which owns the bus, the shared
  inbound handler and the configuration — and the channel plugins, which
  build their transports against it.

So it moved **down** to the platform layer: `kernel/channelwire` →
`kernel/platform/channelwire` (with `git mv`, so the history follows the
code), and the explicit L3 rule in `layers.json` was **deleted** (it would
have matched nothing and the "channels" module would have been left empty).

## The ratchet went DOWN

**303 packages, 82 import violations, 13 call sites** — from 83/13. The
`plugin-reach` edge is gone and the **stale allowlist entry was deleted**.

This is the same shape as W3.35 (the ACP client): a package that mixes an
**adapter's** job with **infrastructure**'s job ends up unreachable from the
plugins that legitimately need the infrastructure half. The fix is to split
the layer, not to allowlist the reach.

## The behaviour did not change — and that is the point

A pure move must not change a single behaviour, so the slice's proof is
mostly about the **reach** and about the behaviours that the move could have
broken:

- the factory layer still **registers** a factory per kind, **looks it up**,
  and **builds** the configured instances;
- the instance still carries the **brief sink** (one per `Built`, not per
  channel);
- the factory still receives the **config getter** and the **account
  label**;
- the plugin still names the contract's briefing row and never the pulse
  module.

## The measurement gap this slice found

The mutation "the factory layer hands over the **empty** account label
instead of the labelled one" **survived** the first round: no test built a
*labelled* account through `BuildKind`, so the label was never observed. The
label is what tells a factory **which** account it is building — its config
getter reads `<base>#<label>` — so a factory that always saw the empty label
would build the default instance's configuration twice and the labelled
account would never be configured at all.

The test was **strengthened**: it pins a labelled account of a real channel
kind (`AGEZT_TELEGRAM_TOKEN#bot2`) and asserts that the factory saw the
empty label for the default instance and `bot2` for the labelled one. The
mutation was caught on the second round.

The lesson is the W3.35 one in a new shape: *a path that is never taken is
not a path*. The handover existed, and nothing exercised it.

## The proof

### Permanent tests

- **The factory layer** (`kernel/platform/channelwire/channelwire_test.go`):
  a registration is found by a lookup and an unknown kind is not; an
  unconfigured kind builds no instance; a configured kind builds its
  channels with the right key, desc, kind and label; a multi-channel build
  keys its extras by their own name; and **the account label is handed over**
  (the default instance sees the empty label, the labelled one sees its own).
- **The factory layer's contract** (`kernel/platform/channelwire/contract_type_test.go`):
  the built row's sink is the **contract's** port, and the rule "this layer
  must not name the pulse module" is stated where a reader will find it.
- **The plugin** (`plugins/builtinchannels/contract_type_test.go`): the
  channels name the contract's rows, they do not name the pulse module, and
  the factory layer they register against is the **platform** one — the
  declared field type of its `Deps` is the platform's bus.
- **The drift alarm** (`plugins/builtinchannels/factory_ratchet_test.go`):
  every registered manifest kind has a factory — a channel shipped with a
  manifest but no wiring fails loudly.

### Mutation run

**8 mutations: 8 caught.** One mutation **survived** the first round (the
account label) and was caught after the test was strengthened; one mutation
was **ineffective** (it added the layer rule where the first-match rule
already won, so the classification never changed) and was re-authored to
apply the rule where it wins.

| # | Mutation | Result |
|---|---|---|
| M01 | the factory layer is classified as a module again | caught — **by the archcheck gate** |
| M02 | the channels name a module again | caught — **by the archcheck gate** |
| M03 | a registration is dropped | caught |
| M04 | a lookup never finds the factory | caught |
| M05 | a kind builds no instance | caught |
| M06 | the account label is not handed over | survived the first round → test strengthened → caught |
| M07 | the config getter is not handed over | caught |
| M08 | the brief sink is dropped from the instance | caught |

M01 and M02 are measured by the **gate**: M01 says the classification *is*
the rule (put the layer back where it was and the plugin's reach becomes a
violation again), and M02 says the gate still sees this plugin's reaches at
all. Neither is visible to a test — the reach is not a behaviour.

### Gates

The build, gofmt, vet, dead-code, archcheck (**303 packages, 82/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so
is the full test suite.

## What is deliberately left

- The **channel vocabulary** (`kernel/channel`) stays in the contract layer —
  it is the contract the transport plugins implement.
- The **channel plugins** (`plugins/channels/*`) stay plugins: they are
  transport implementations, and that is their job.
- The remaining plugin-reach edges are the **registration seams**: the
  built-in tools reach `kernel/toolreg` (the tool registry), `kernel/runtime`
  (the live kernel) and the plugin host (`kernel/modules/extensions/plugin`).
  Those are the hard ones to invert: the plugin *registers into* a registry
  the daemon owns, which is a dependency in the opposite direction of the
  one the rule forbids.
