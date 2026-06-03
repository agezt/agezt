# M255 — Vision-capability pre-gate for the API and channel run paths

## Why
The control plane's `CmdRun` handler pre-checks a run's model for vision
capability before any provider call (M91): an image attached to a non-vision
model is refused up front. But two other entry points reach the run path
WITHOUT going through that handler:
- the OpenAI-compatible API (`kernelAPIEngine.RunModel` → `RunWith`), and
- the chat channels (`makeChannelHandler` → `RunWith`).

Since M246–M249 wired image input on both, a user sending an image to a
non-vision model via the API or a channel got a cryptic downstream provider
error (and a wasted, billed provider call) instead of a clear pre-flight
rejection. This is the gap flagged in the M246/M247 reports.

## What
- **`cmd/agezt/main.go`** — new `visionGate(k, model, images)` and its pure core
  `gateVisionWith(cat, defaultModel, model, images)` mirror the control plane's
  M91 check: resolve the effective model (the request's model, or the kernel
  default), look it up in the catalog, and require `SupportsVision()`. Confirmed-
  or-reject — an unknown or known-non-vision model is refused when images are
  present.
  - `kernelAPIEngine.RunModel` calls it before threading images into the run; on
    rejection the API returns the error (surfaced as `upstream_error`/the JSON
    error envelope).
  - `makeChannelHandler` calls it before threading images; on rejection the
    channel replies with the clear "sorry — that failed: …" notice instead of
    running.

## Files
- `cmd/agezt/main.go` — `visionGate` + `gateVisionWith`; calls in
  `kernelAPIEngine.RunModel` and `makeChannelHandler` (edited).
- `cmd/agezt/vision_gate_test.go` — `TestGateVisionWith`: table of 7 cases (no
  images, vision/non-vision model, default-model fallback both ways, unknown
  model, nil catalog) (new).

## Verification
- `go test ./cmd/agezt/` — green; full suite **1827 → 1828** (+1), 66 packages,
  `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./cmd/agezt/` clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.

## Scope notes
- The gate logic is now in three places (control plane, plus this shared
  `gateVisionWith` for API + channels). Centralising all three into one kernel
  method — and having `RunWith` enforce it for every entry point — is a possible
  follow-up; this milestone keeps the change contained to the two ungated paths.
- Closes the last vision "remaining nit" I had tracked: vision is now complete
  AND consistently gated across the CLI/control-plane, the OpenAI API (chat +
  responses), and all three channels.
