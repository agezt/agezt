# M241 — `agt run --image` actually delivers the image to the model (Anthropic)

## Why
Auditing `agt run`'s exit code (the prior thread's open question) cleared the
exit path — `task.failed` is emitted **iff** `RunWith` returns an error, which
becomes a `RespError` and a non-zero CLI exit on every run path (stream, quiet,
`--json`, dry-run). No bug there.

The audit then surfaced a genuine, silently-misleading one: **the `--image`
vision feature was a no-op end to end.** The chain was:
1. CLI: `--image <path>` ran `os.Stat` then appended only `filepath.Base(path)`
   — the path and bytes were discarded.
2. Daemon: gated the run against the model's vision capability (M91) and
   journaled `images = <count>` (M93).
3. Provider: received `Message.Images []string` — but **no provider read it**.
   The Anthropic encoder emitted a text-only user block.

So a user running `agt run --image photo.png "describe this"` against a
vision-capable model got the capability gate, the attachment count in
`agt runs show`, a normal run — and a model that never saw a single pixel. A
bare basename is also unresolvable across processes (the daemon's cwd ≠ the
CLI's), so even a provider that *tried* to read it couldn't.

## What
A self-describing **`data:` URL carrier** — no wire-shape or type change
(`images` stays `[]string`), so the M91 vision gate, the journaled count, and
`agt runs show` are all untouched.

- **`cmd/agt/main.go`** — `--image` / `--image=` now call a new
  `loadImageDataURL(path)`: map the extension to an IANA media type (png, jpeg,
  gif, webp), read the bytes (the file is on the operator's machine, not the
  daemon's), and return `data:<media-type>;base64,<bytes>`. Empty files and
  unsupported types are rejected with a clear message; files over 12 MiB are
  refused client-side (base64 ≈ 4/3 × raw vs. the 16 MiB control-plane request
  cap) instead of producing a cryptic oversize-frame rejection. Added the
  `encoding/base64` import.
- **`plugins/providers/anthropic/anthropic.go`** — `canonicalToAnth` (shared by
  both `encodeRequest` and `encodeStreamRequest`, so streaming and
  non-streaming both benefit) now turns each `data:` URL on a user message into
  an Anthropic `type=image` block with a base64 `source`, placed **before** the
  text block (Anthropic's recommended order). New `anthImageSource` struct +
  `parseImageDataURL` helper. A non-data-URL entry (e.g. a legacy bare filename)
  has no deliverable payload and is skipped.

## Files
- `cmd/agt/main.go` — `loadImageDataURL` + `imageMediaType` helpers; both
  `--image` branches read bytes → data URL (edited).
- `cmd/agt/image_test.go` — 5 tests: data-URL round-trip, jpg/jpeg → image/jpeg,
  unsupported type, empty file, missing file (new).
- `plugins/providers/anthropic/anthropic.go` — `anthBlock.Source` +
  `anthImageSource`; `parseImageDataURL`; `canonicalToAnth` RoleUser emits image
  blocks (edited).
- `plugins/providers/anthropic/vision_test.go` — 4 tests: image block on
  `encodeRequest`, image block on `encodeStreamRequest` (default run path),
  non-data-URL skipped, `parseImageDataURL` good + malformed cases (new).

## Verification
- `go test ./cmd/agt/ ./plugins/providers/anthropic/` — green; full suite
  **1782 → 1791** (+9), 66 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean on all four files; `go vet` clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged (stdlib `encoding/base64` only).

## Scope notes
- Exit-code investigation closed with a negative result — no fix needed; this
  milestone is the real bug the audit found instead.
- The carrier is provider-agnostic (a standard `data:` URL). This milestone
  wires the **Anthropic** provider; OpenAI (native `image_url` data-URL support)
  and Gemini emission are the obvious follow-ups, each a tight milestone reusing
  the same CLI-side delivery.
- Nothing journals or renders the `Images` string values (only the count), so
  the large base64 payload never bloats the journal or `agt runs show`.
