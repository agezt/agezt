# M476 — Telegram: stop dropping photo/caption-only inbound messages (HIGH, functional)

## Context
The Telegram channel long-polls `getUpdates` and dispatches each update to
`handleInbound`, which contains the full inbound-image pipeline (M247): for an
allowlisted sender it fetches the largest photo as a `data:` URL and attaches it to
the unified message for a vision model. A photo's text rides in `Caption`, not
`Text` (documented on the struct), and a photo may have no text at all.

## The bug (HIGH — a shipped feature is dead on the live path)
The poll loop gated dispatch on `Text`:

```go
for _, u := range updates {
    c.offset = u.UpdateID + 1
    if u.Message == nil || u.Message.Text == "" {
        continue   // drops photo-only AND caption-only messages
    }
    channel.Guard(c.bus, "telegram", func() { c.handleInbound(ctx, u.Message) })
}
```

A photo-only message has `Text == ""`, so it is skipped before ever reaching
`handleInbound` — the only place that fetches the image. So the entire inbound-image
feature (and any caption-only message) is silently dead on the real polling path.
It was invisible to the test suite because every poll-loop test sends `Text`, while
the photo/caption tests call `handleInbound` directly, bypassing the gate.

## The fix
Extract a `dispatchable` predicate and admit messages carrying a caption or photo:

```go
func dispatchable(m *tgMessage) bool {
    return m != nil && (m.Text != "" || m.Caption != "" || len(m.Photo) > 0)
}
```

`handleInbound` already reconstructs text from `Caption` and handles the photo/empty
combinations (including the non-allowlisted case), so nothing downstream changes.

## Test + negative control
`plugins/channels/telegram/telegram_test.go`: `TestDispatchable_AdmitsPhotoAndCaption`
— table test: nil/empty → false; text/caption-only/photo-only → true.

**Negative control:** reverting `dispatchable` to `m != nil && m.Text != ""` made the
caption-only and photo-only cases return false — the test FAILED on exactly those
two rows. Restored; test passes.

## Provenance
From the scoped review of the email + telegram channels (offset advance, synchronous
dispatch, response-body caps, credential scrubbing all reviewed CLEAN; email is
outbound-only). The email channel's bare-CR subject (LOW header-hardening) is noted
as a follow-up.

## Verification / gate
- `plugins/channels/telegram` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
