# M234 — Chunk over-long Telegram/Discord messages

## Why
A real, commonly-hit delivery bug. Chat platforms cap a single message:
Telegram at **4096 UTF-16 code units**, Discord at **2000 characters**. Agent
answers (summaries, code, briefs) routinely exceed those. The send paths posted
`out.Text` as a single request with no length handling:

- `telegram.send` → one `sendMessage` → Telegram returns **400** for >4096.
- `discord.Send` → one `POST .../messages` → Discord rejects >2000.

So a long answer wasn't truncated — it was **rejected wholesale and lost**, with
only an error in the daemon log. The user saw nothing.

## What
- **`kernel/channel/split.go`** (new) — `SplitText(text, limit) []string`,
  shared by the channel plugins. It splits into pieces each at most `limit`
  **UTF-16 code units** (what Telegram counts; also a safe bound for rune/code-
  point-counting platforms since a rune is never more UTF-16 units), preferring
  to break just after the last newline/space that fits and hard-cutting an
  unbroken run. It is **lossless**: concatenating the pieces reproduces the input
  exactly (no characters added or dropped).
- **`plugins/channels/telegram/telegram.go`** — `send` loops over
  `SplitText(out.Text, 4096)`, posting each chunk; any non-2xx aborts. One
  `channel.outbound` journal event per logical message (unchanged).
- **`plugins/channels/discord/discord.go`** — `Send` loops over
  `SplitText(out.Text, 2000)` the same way.

(Slack's limit is ~40000 and effectively never hit by agent replies; its send is
left as-is. The shared helper is ready if it's wanted later.)

## Files
- `kernel/channel/split.go` — `SplitText` + UTF-16 helpers (new).
- `kernel/channel/split_test.go` — 7 tests (new): within-limit unchanged, every
  piece within limit across several limits, the lossless-rejoin invariant
  (incl. multibyte text), boundary preference, emoji counting as 2 UTF-16 units,
  long-word hard-cut, non-positive limit.
- `plugins/channels/telegram/telegram.go` — chunked send + `telegramMaxChars`
  (edited).
- `plugins/channels/telegram/telegram_chunk_test.go` — 2 tests (new): a 10k-char
  message goes out as ≥3 sends each ≤4096 units and rejoins to the original; a
  short message is still one send.
- `plugins/channels/discord/discord.go` — chunked send + `discordMaxChars`
  (edited).
- `plugins/channels/discord/discord_chunk_test.go` — 1 test (new): a 5k-char
  message goes out as ≥3 messages each ≤2000 chars and rejoins.

## Verification
- `go test ./kernel/channel/ ./plugins/channels/...` — green; full suite
  **1763 → 1773** (+10), 66 packages.
- `gofmt -l` (CRLF-normalised) clean; `go vet` clean on all three packages.
- `GOOS=linux go build ./...` clean; `go.mod` / `go.sum` unchanged.
- **Proof:** the channel tests drive the real `Send` against an httptest stand-in
  for each platform's API and assert the over-limit message produces multiple
  in-limit POSTs that rejoin to the original — the exact before(lost)/after
  (delivered) behaviour, network-free.

## Scope notes
- Discord's interaction follow-up path (`followUp`) wasn't changed; it serves
  slash-command replies and is a separate flow. The primary `Send` (Pulse
  briefs, channel replies) is the high-traffic path and is now safe.
- Markdown/formatting that spans a chunk boundary (e.g. a code fence split across
  two messages) can render imperfectly — an acceptable trade for delivery over
  loss; a fence-aware splitter could be a later refinement.
