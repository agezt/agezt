# Phase M821 — Vision sidecar (auto-describe images on non-vision models)

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: when an image
arrives on a channel and the active model isn't vision-capable, analyze it
internally with a keyed vision provider instead of failing. Owner chose the
**sidecar** approach (keep the active model; a vision model captions the image;
inject the caption).

## Why

A photo sent to the bot, or `agt run --image` on a non-vision model, hard-failed
with `model does not support vision`. The owner has keyed providers; if any has a
vision model, the system should use it transparently.

## What shipped

- **Catalog finder** `Catalog.VisionCapableAmong(providerEligible) (modelID, ok)`
  (kernel/catalog/types.go) — first eligible+credentialed provider's
  largest-context vision model (deterministic), mirroring the M37/M40
  `ToolCapableAlternativeAmong` pattern + `pickBestVision`.
- **Kernel sidecar** `Kernel.DescribeImages(ctx, corr, images, hint) (string,
  error)` (kernel/runtime) — one-shot governor completion to the vision model
  (req.Model = vision model, Messages[0].Images = images), returns the caption,
  journals a `capability.rerouted` (capability="vision") event for `agt why`.
  `ErrNoVisionModel` when none is configured. Picker injected via new
  `Config.VisionModel func() (string, bool)`.
- **Daemon wiring** (cmd/agezt/main.go): `cfg.VisionModel` resolves a keyed vision
  model from the LIVE catalog (eligibility = supported family + credentialed, same
  as buildGovernor's registered set), so a freshly synced vision provider is
  picked up without a restart.
- **Replace the hard reject at both image entry points:**
  - `makeChannelHandler` (Telegram/Slack/Discord inbound): non-vision active model
    + image → caption via the sidecar, inject `[Image description (analyzed by a
    vision model): …]` into the intent, don't forward raw images. Vision-capable
    model → unchanged pass-through. No keyed vision model → the clear gate error.
  - controlplane run handler (`agt run --image` / API): same logic; corr is now
    pre-generated before the gate so the sidecar event links to the run.

## Tests

- catalog: `VisionCapableAmong` picks largest-context vision model (repeated for
  map-order determinism), skips ineligible/text-only providers, returns false
  when none.
- runtime: `DescribeImages` routes to the vision model with the images and
  returns the caption; `ErrNoVisionModel` when unset.
- Full suite green (all packages; cmd/agt verified in isolation — 238s — the
  capped parallel run merely exceeded the default 10m wall, no regression);
  vet + staticcheck + linux cross-build clean.

## Note

A live positive smoke needs a keyed *vision-capable* provider; the unit tests
cover the routing + caption path deterministically. The channel path (the owner's
actual use) and the CLI path both now degrade to the sidecar instead of erroring.
M822 will persist these inbound images as browsable artifacts.
