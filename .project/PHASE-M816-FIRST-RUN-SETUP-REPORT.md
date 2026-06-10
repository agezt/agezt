# Phase M816 — first-run setup & onboarding (WebUI wizard + live provider switch)

**Date:** 2026-06-10 · **Status:** DONE · **Goal:** owner asked for a
guided first-run that helps on BOTH surfaces — "ilk kurulumda hem cli hem
webui tarafı ile bana yardımcı olan bir setup, first use olayı lazım".

## Why

Opening the daemon already self-bootstraps everything except the one thing
it can't conjure: a real LLM key. With no key it degrades to the offline
`mock` provider — a brand-new user lands on Chat, types something, and gets
the scripted mock text. There was no guided first-run, and `agt quickstart`
only *printed* a start command (the choice wasn't persisted).

## What shipped

**WebUI — `frontend/src/views/Setup.tsx` (new).** A self-contained 3-step
wizard over EXISTING routes (no new daemon commands):
1. **Catalog** — GET `/api/catalog`; if empty, "Sync models.dev" → POST
   `/api/catalog/sync`; else jumps straight to the provider step.
2. **Provider + key** — searchable picker (ranked credentialed → keyed →
   local); on submit POST `/api/provider/keys/add` `{env,label,value,active}`
   (key env resolved like Models.tsx) + `/api/config/set` `AGEZT_PROVIDER`
   (ApplyLive) + `/api/provider/reload`.
3. **Model** — list the provider's catalog models → POST `/api/config/set`
   `AGEZT_MODEL` → "You're ready" → `onDone()` drops into Chat.

**WebUI — `App.tsx`.** A permanent **Setup** nav entry (System group), plus a
first-run **auto full-screen overlay**: on mount GET `/api/catalog`; if NO
provider is `credentialed` and no `agezt.setup.skipped` flag, render the
wizard as a `fixed inset-0 z-[200]` layer. `onDone`→Chat; `onSkip` sets the
localStorage flag (Setup nav still reopens it). Once any provider is
credentialed the overlay never auto-shows again.

**CLI — `cmd/agt/quickstart.go`.** `[4/4]` now PERSISTS the choice: if a
daemon is reachable, `persistProviderModel` pins `AGEZT_PROVIDER`/`AGEZT_MODEL`
live via `CmdConfigSet` (no env vars, no restart) and prints "saved (live)";
otherwise it falls back to the env-var start command.

**CLI — `cmd/agezt/main.go` banner.** When the governor degrades to the
offline mock, a prominent can't-miss line prints right after the governor
banner: `⚠ setup needed : no provider key yet — run \`agt quickstart\`, or
open the Web UI (URL below) to add one`.

## The defect the smoke caught — live provider switch was broken

Verifying end-to-end against an isolated daemon revealed that setting
`AGEZT_PROVIDER` live did NOT actually switch the running provider:

1. **Model wasn't swapped.** `OnReload` rebuilt the provider but discarded the
   new model (`_ = model2 // defer to M1.r.x`), so requests hit the new
   provider carrying the OLD model id "mock" → deepseek's API rejected it
   (`you passed mock`).
2. **Mock primary was never demoted.** At boot with no creds, `buildGovernor`
   registers the offline mock as the PRIMARY. `Registry.Replace` only swaps an
   entry of the SAME name, so replacing "mock" with "deepseek" APPENDED
   deepseek behind the mock — mock stayed `primary[0]` and kept serving every
   run. The wizard would say "you're ready" while answers stayed
   `[offline-mock]`.

### Fixes

- **Live model field (mirrors the M710 persona pattern).** Added a
  mu-guarded `model` to the kernel, seeded from `cfg.Model`, exposed via the
  now-guarded `Model()` getter + new `SetModel()`. All hot-path readers
  (main agent loop, verify, forge/memory attribution) use `k.Model()`.
  `OnReload` calls `k.SetModel(model2)` so a provider switch updates the model
  in place — no restart.
- **Demote the stale mock primary on reload.** In `OnReload`, when the
  newly-selected primary is real, remove the mock primary from the registry
  and re-add it as a last-resort `IsFallback` entry (mirroring the boot path's
  "primary != mock ⇒ mock is fallback" rule), THEN `gov.Replace` the real
  provider. The governor rebuilds its primary/fallback slices from the
  registry, so deepseek becomes the sole primary with mock as a safety net.

## Tests

- **vitest** `Setup.test.tsx` (6): pure helpers (`providerKeyEnv`,
  `anyCredentialed`, `rankProviders`); full walk asserting exact POST args for
  key-add + `AGEZT_PROVIDER` + reload + `AGEZT_MODEL` + `onDone`; empty-catalog
  Sync; overlay Skip.
- **Go** `TestKernel_Model_LiveSwap`: `Model()` seeds from cfg, a run carries
  it, `SetModel` swaps it, and the NEXT run carries the new model — locking the
  live-swap behaviour.

## Smoke (isolated AGEZT_HOME — owner's real ~/.agezt never touched)

Empty home, no key → banner prints the **⚠ setup needed** line + Web UI URL;
governor = `primary=mock(offline)`. Playwright: the auto full-screen wizard
appeared (no credentialed provider), searchable provider picker, deepseek key
field "stored as DEEPSEEK_API_KEY". Walked the wizard routes with the owner's
REAL deepseek key (from the gitignored `.env`, never echoed): both config
steps returned `applied: live`. A real `agt run` then answered **`SETUP_OK_M816`**
from `deepseek-v4-pro · $0.0032`, `calls by primary: deepseek`, **0 fallbacks**
— **no restart**. Reload → auto-wizard did NOT reappear (deepseek
credentialed); the **Setup** nav entry still opens it. Filesystem isolation
confirmed (separate temp home, owner's standing orders untouched).

## Gate

Full `GOMAXPROCS=3 go test -p 2 ./...` green; vet + staticcheck clean; linux
cross-build OK; vitest green; `kernel/webui/dist` rebuilt (committed LF);
go.mod unchanged (1 dep). No new control-plane commands, env vars, or event
kinds — every route already existed.

## Backlog now

Remaining listed items are owner-gated (CI billing → green badge → v1.0.0)
and the provider-embeddings opt-in (needs an embeddings-capable keyed
provider — verify before building to avoid burning budget on a wire that may
not exist).
