# Phase Report — Milestone M91 (vision capability gate)

> Status: **shipped** · Date: 2026-06-01 · SPEC-14 capability enforcement.

## Why

Vision/attachment enforcement was the longest-deferred frontier — repeatedly
noted as "blocked on the agent message type carrying images". But the *enforcement*
half doesn't need the message type to change: the catalog already knows each
model's vision capability (`Model.SupportsVision()`, surfaced by `agt provider
check --caps`), and the kernel exposes both the active model (`k.Model()`) and the
catalog (`k.Catalog()`) at runtime. The missing piece was a runtime GATE: nothing
stopped an operator from attaching an image to a model that can't read one — a
guaranteed hard failure at the provider.

M91 adds that gate at the control-plane submission boundary, so the agent loop and
message type stay untouched.

## What shipped

- **`agt run --image <path>`** — attaches one or more images to a run (the CLI
  validates each path exists; sends the basenames as declared attachments).
- **Pre-flight vision gate (`handleRun`)** — a run carrying images is rejected
  *before any provider call* unless the active model is confirmed vision-capable
  (`cat.FindModel(k.Model()).SupportsVision()`). The rejection journals a
  `capability.rejected` event (`{model, capability: "vision", images_requested}`,
  the M23 pattern) and returns a clear error pointing at `agt provider check
  --caps`.

## Design decisions

- **Confirmed-or-reject, stricter than the M25 tool gate.** The tool gate allows
  *unknown* models (many tolerate tool schemas); but an image sent to a non-vision
  model is a guaranteed failure, so vision denies unless capability is positively
  confirmed. Unknown model ⇒ reject. This is the safe default for a hard
  capability.
- **Boundary enforcement, not a type change.** The gate lives where the request
  enters the daemon, so it needed no change to `agent.Message` (still a string),
  the agent loop, or any provider — zero hot-path risk. The image-*bytes*-to-
  provider plumbing (Message attachments + provider encoding) remains a separate,
  larger follow-on; this milestone is the *enforcement boundary* that follow-on
  will sit behind.

## Tests

- `TestRun_VisionGate_RejectsImageOnNonVisionModel` — a run with `images` on a
  non-vision model errors (mentioning vision) and journals
  `capability.rejected{capability:vision}`.
- `TestRun_NoImage_Unaffected` — an ordinary run is not gated.

Test count: **1334 → 1336**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt run --image photo.png "describe this image"
  agt run: controlplane: model "mock" does not support vision (image input);
           attach images only to a vision-capable model (see `agt provider check --caps`)
$ agt run "summarize"          # no image → unaffected
  --- final answer --- …
# capability.rejected journaled
```
