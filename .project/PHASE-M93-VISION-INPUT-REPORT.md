# Phase Report — Milestone M93 (vision image input — bytes plumbed to the provider)

> Status: **shipped** · Date: 2026-06-01 · SPEC-14 vision.

## Why

M91 added the vision capability *gate* (reject images on a non-vision model);
M92 surfaced the rejections. The remaining half — making a vision-capable model
actually *receive* the image — was the long-deferred "blocked on the message
type" work. M93 does it, and (the part that kept it deferred) makes it
**demo-gatable offline** with a vision-capable mock, so it meets the project's
live-proof bar instead of shipping unproven.

## What shipped

- **`agent.Message.Images []string`** — additive, omitempty. The attachment
  references the model receives. Providers that don't read it are unaffected.
- **`agent.LoopConfig.Images` + `agent.Run`** — the loop attaches the images to
  the initial user message, and stamps the count on `task.received` for run
  provenance.
- **`runtime.WithImages(ctx, …)` / `imagesFromCtx`** — threads images from the
  control plane into `RunWith` without a signature change (same ctx-value pattern
  as `WithModel`).
- **`handleRun`** — on a *passing* M91 gate, carries the image refs into the run
  ctx so they reach the message; a failing gate still rejects (M91, unchanged).
- **`mock.Provider.Responder`** — an optional input-reflecting responder, so a
  demo/test mock can compute its reply from the request (here: echo the image
  count).
- **`AGEZT_DEMO_VISION=1`** — registers a synthetic vision-capable "mock" catalog
  entry (so the gate passes) and a mock that echoes the received image count.
  Production catalogs and the default mock are untouched, so M91's reject demo and
  tests still hold.
- **Arc provenance** — `agt runs show` renders `inputs: N image attachment(s)`
  from `task.received`.

## Design decisions

- **References, not bytes, in the kernel.** `Images` carries handles; how a
  provider fetches/encodes the actual bytes is its own concern. The kernel's job
  is the capability gate (M91) and provenance — both satisfied by presence.
- **Demoability was the gating constraint, so it was built first.** The
  vision-mock + synthetic catalog entry make the accept-path provable offline;
  without them this would have been the session's first unproven change.
- **Additive + ctx-threaded.** No provider signature broke, no `RunWith` signature
  broke; existing callers and providers compile and behave identically.

## Tests

- `TestRun_ImagesReachProvider` — `LoopConfig.Images` lands on the initial user
  message and reaches the provider's `CompletionRequest`.
- (M91's `TestRun_VisionGate_RejectsImageOnNonVisionModel` still passes — the
  reject path is unchanged.)

Test count: **1337 → 1338**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ AGEZT_DEMO_VISION=1 agt run --image a.png --image b.png "describe these images"
  [offline-mock vision] received 2 image attachment(s); a real vision model would describe them here.
$ agt runs last
  intent     : describe these images
  inputs     : 2 image attachment(s)
# and without AGEZT_DEMO_VISION, the same --image run is REJECTED (M91).
```
