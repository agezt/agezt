# M547 — Pin the inbound media-download size caps (telegram/discord/slack)

## Context
The three media-capable channels each download an attachment from an *untrusted
external source* (a Telegram file, a Discord CDN url, a Slack url_private) and
inline it as a data: URL for a vision model. Each bounds that download with the
same idiom so a hostile/oversized body can't be streamed unbounded into the
daemon's memory. M540 pinned the *auth gate* on these (only an allowlisted sender
triggers a fetch); this pins the *size cap* itself. `GOMAXPROCS=3`.

## The shared idiom and its two genuine gaps
```go
data, _ := io.ReadAll(io.LimitReader(resp.Body, MaxRaw+1))   // read one past the cap
...
if len(data) > MaxRaw {                                       // reject if over
    return "", fmt.Errorf("... exceeds %d bytes", MaxRaw)
}
```
`MaxRaw` is `12 << 20` (12 MiB) in all three. The happy-path image tests
(`Test*ImageBecomesDataURL`) use a handful of bytes, so neither boundary was
covered. Two mutation points survived per channel:

1. **`len(data) > MaxRaw → >= MaxRaw`** — a file of *exactly* MaxRaw bytes (a
   legitimate max-size upload) would be wrongly rejected.
2. **`io.LimitReader(resp.Body, MaxRaw+1) → MaxRaw`** — the `+1` is load-bearing:
   it lets `len(data)` reach `MaxRaw+1` so the `> MaxRaw` check can fire. Drop it
   and an oversized body reads as *exactly* MaxRaw → `> MaxRaw` is false → the file
   is silently **accepted, truncated** instead of rejected. That defeats the whole
   DoS guard (and hands a corrupted image to the model).

## Fix
Added `Test*SizeCapBoundary` to each channel, calling the fetcher directly against
an httptest body of exactly `MaxRaw` (must be accepted, returns a `data:` URL) and
`MaxRaw+1` (must be rejected with "exceeds"):
- telegram `inbound_image_size_test.go` → `fetchPhotoDataURL`
- discord `inbound_image_size_test.go` → `fetchAttachmentDataURL`
- slack `inbound_image_size_test.go` → `fetchFileDataURL`

## Negative control (manual, CPU-capped)
For each channel, both mutants make the matching subtest FAIL (exactly-max
rejected → `exactly max accepted` fails; `+1` dropped → `one past max rejected`
fails). All six confirmed in-source before running (the discord `>` guard is at
line 390, not 392 — caught a silent no-op per the M530 lesson). All three sources
restored byte-for-byte (`git diff --ignore-all-space` empty).

## Verification / gate
- Tests pass; `go vet` + `staticcheck` clean on all three; gofmt-clean on the
  staged LF blobs.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Cross-package pattern note
This is the read-bounded-download cousin of the inclusive-max frame idiom pinned in
plugin readFrame (M509), control-plane readBoundedLine (M531), mcpbridge (M538),
and acpagent (M542). The `LimitReader(_, Max+1)` + `len > Max` form (read one extra
to detect overflow) recurs in every channel media fetcher and is now pinned in all
three — the load-bearing `+1` especially, which is easy to "simplify" away.
