# W3.12 — the providerboot half of the providers module: the engine has a home

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 step 10 —
`modules/providers` (catalog, chatgptauth, providerboot).

**Scope:** the step's **other half**. W3.11 moved the catalog and the ChatGPT
auth; this slice moves the boot engine itself and it is a **ports rewrite**, not
a path change — the reason is in "Why this slice is not a move" below.

## What moved

| Before | After | What it is |
|---|---|---|
| `plugins/providerboot` (14 files) | `kernel/modules/providers/boot` (13 files + 4 new test files) | the daemon's provider bootstrap: primary selection, alternate registration, the governor's construction, the hot-reload path, the ChatGPT model discovery and catalog seeding |
| — | `kernel/modules/providers/api` (new) | the module's gate: the model row the module owns, the token seam, and the `Ports` interface the engine asks the adapters for |
| `plugins/providerboot.Middleware` | `kernel/agent.MiddlewareFromEnv` (new) | the environment-driven middleware stack, next to the middlewares it builds |
| — | `cmd/agezt/boot_providerports.go` (new) | the composition root's binding: the module's ports ↦ the real wire adapters |

The package was renamed `providerboot` → `boot` (the module path already says
`providers`) and its seven files lost the now-redundant `providerboot_` prefix;
the seven `chatgpt_*` files kept their names.

## Why this slice is not a move

`plugins/providerboot` was not "a library that happens to sit under `plugins/`".
It imported the **concrete** provider packages — `plugins/providers/compat`,
`mock`, `openairesponses` — and it reached into `kernel/agent` for the
middleware stack. Moving it into the module as-is would have added **four**
edges the ratchet refuses to grow: three **upward** ones (module → plugin) and
one **cross-module** one (providers → runs).

So the slice is a ports rewrite, the shape W3.7's workflow engine and W3.9's
reaper proved:

- the module asks for the adapters through a **port** — `api.Ports`, an
  **interface**, so a missing adapter is a compile error at the wiring site
  and not a nil dereference on a boot path;
- the module keeps **its own vocabulary** — `api.ModelInfo` replaces
  `openairesponses.ModelInfo`, and the module owns the picker rule (which
  entries reach an operator) and the ordering rule (the backend's priority);
  the adapters map their wire shapes into it;
- the **middleware stack stays the host's** — it lives in the runs module
  (`agent.MiddlewareFromEnv`) and reaches the engine through the `Wrap` port,
  so the stack is built **once per boot cycle** and applied to the primary and
  every alternate alike (the M997 drift fix wrapped both; the wrap must not
  fall between the two);
- the composition root (`cmd/agezt/boot_providerports.go`) binds the ports to
  the real adapters. Root → plugin is free (L7 → L6), root → module is free
  (L7 → L3), so the binding costs no edge at all.

## The ratchet went DOWN

**276 packages, 123 import violations, 13 call sites** — the ratchet dropped
from **126/13 to 123/13**, because the three edges the plugin needed are gone:

- `plugin-reach plugins/providerboot → kernel/agent` — the middleware stack is
  asked for through the port now;
- `plugin-reach plugins/providerboot → kernel/modules/providers/catalog` — the
  catalog is now the module's own package, an intra-module edge (no edge);
- `plugin-reach plugins/providerboot → kernel/modules/providers/chatgptauth` —
  the same.

No new entry was added to the allowlist.

## The proof

### Permanent tests

The engine's own suites moved with it (boot/reload parity, the middleware
survives reload, the live down-route eligibility set, the sentinel promotion,
the alternate reconciliation, the ChatGPT discovery ladder and the seeding
contract) and the module's tests now drive the engine through **fake ports**
(`ports_test.go`) — the module never imports a plugin, so its tests must not
pretend to. The fake mirrors the real adapters' contracts (an empty model id
and a model the entry does not serve are both refused), because a fake that
accepts what the real adapter rejects hides exactly the bugs the run is
looking for.

The **real** adapters are measured where they are bound, in the root
(`cmd/agezt/boot_providerports_test.go`): the demo echo provider's answer, the
compat construction, the empty-model refusal, the family gate, the placeholder
model, the ChatGPT base URL, the backend-reply mapping and the token function's
two branches.

**New tests the slice needed** (each one closes a gap the port seam opened):

- `TestEligibleSkipsAnUnsupportedFamily` — a credentialed provider of a family
  the adapter cannot serve must not be registered as an alternate. Before the
  ports rewrite the family check lived in the plugin; now it is a port, and a
  port that is wired wrongly would register a provider that can only fail at
  call time.
- `TestAlternatesAreWrappedToo` — the middleware stack must reach the
  alternates, not only the primary. The old code wrapped both in one place;
  after the rewrite the wrap is a value that is passed around, and a value
  passed around can be dropped on one path.
- `TestNilWrapperIsLoud` — a mis-wired adapter (a nil wrapper) is reported,
  not dereferenced.
- `TestPortsNotWired` — a `Deps` without ports is a wiring error, and the
  engine says so instead of running without an adapter.
- `TestBootDepsWiresThePorts` — the daemon's **one** wiring point carries the
  ports. This test exists because the e2e smoke found the gap: two boot paths
  in `cmd/agezt` assembled the bundle by hand and one of them forgot the
  ports, so the daemon refused to start. The loud guard turned that into a
  named error (`the wire adapters are not wired`) instead of a nil
  dereference, and the bundle is now assembled in exactly one place
  (`bootDeps`), which both the boot and the reload path go through.
- `TestChatGPTCatalogEntryUsesTheAdapterBaseURL` — the catalog entry records
  the base URL the **adapter** serves; the module does not hardcode one.
- `TestSyncChatGPTCatalogDropsTheWeakerMemo` — signing in unlocks the backend
  source, so a memo that came from the weaker Codex CLI cache must be dropped.
- `TestModelInfoListed`, `TestSortModels` — the module's own picker and
  ordering rules, now that they are the module's own.
- `TestMiddlewareFromEnv_*` (six) — the middleware stack moved packages, so it
  needed its own tests: empty by default, the sampling defaults, a malformed
  value, the reasoning-extraction knob and its "on" spellings, the streaming
  simulation, and the nil-reader default.

### Mutation run

**19 mutations, 19 caught.** The run is what found the gaps above; two
mutations needed a second round and both are recorded:

| # | Mutation | Result |
|---|---|---|
| M01 | `api.ModelInfo.Listed()` drops the hidden-model rule | caught |
| M02 | `api.SortModels` loses the backend priority | caught |
| M03 | `Eligible` drops the family gate | caught (second round: the first anchor pointed at the wrong file) |
| M04 | the alternates skip the middleware stack | caught |
| M05 | the primary skips the middleware stack | caught |
| M06 | the nil-wrapper guard is gone | caught |
| M07 | the unwired-ports guard never fires | caught (second round: the plain removal was a **build error** — `fmt` became unused — so the guard was neutered instead) |
| M08 | the cross-provider model repair is gone | caught |
| M09 | hidden models reach the picker | caught |
| M10 | hidden models lose their instructions | caught |
| M11 | the weaker cache memo survives sign-in | caught |
| M12 | a builtin set overwrites the stored entry | caught |
| M13 | the reasoning-extraction knob is ignored | caught |
| M14 | the temperature default is ignored | caught |
| M15 | the root's `FirstModelID` returns nothing | caught |
| M16 | the root's family gate says yes to everything | caught |
| M17 | the demo echo loses its prefix | caught |
| M18 | the ChatGPT base URL is empty | caught |
| M19 | the backend-reply mapping loses the instructions | caught |

No equivalent mutant was found: every mutation changed an observable
behaviour that a permanent test names.

### Gates

The build, gofmt, vet, dead-code, archcheck (**276 packages, 123/13**),
structure (and its check), changelog-lint, docclaimscheck and staticcheck are
green, and so is the full test suite.

The **e2e smoke** earned its keep here: it caught a boot path in `cmd/agezt`
that assembled the bundle by hand and forgot the ports, so the daemon
refused to start. The unit suite did not catch it (no unit test boots the
whole daemon), which is exactly what the e2e is for. The fix is not a patch
at the call site: the bundle is now assembled in one place (`bootDeps`), and
a permanent test pins that it carries the ports.

## What is deliberately left

- The **agents module's third item** — the **overseer ops** (the overseer tool
  as an L6 caller of `app` ops). The api door exists since W3.9; the ops
  themselves are the next piece of that step.
- **`modules/runs`** last: what remains of `kernel/runtime` + `runexec` + the
  agent loop + resume + delegation + intervention. `kernel/runtime` is deleted
  when empty.
- The **dated** reports (`.project/PHASE-*`, `.project/AUDIT-*`,
  `.project/REVIEW-MAP.md`, `CHANGELOG/unreleased/m*.md`, the dated scans and
  audits, the W3.11 evidence) describe the tree as it was and are left alone.
