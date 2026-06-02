# M145 — Channel-arc hardening (code review)

## Why
After shipping the channel arc (M138–M144), a focused code-quality pass — an
independent review of the five new files plus the daemon wiring — surfaced several
real correctness, concurrency, security, and privacy issues. This milestone fixes
the legitimate findings. The review also verified the fundamentals are sound (HMAC
and Ed25519 verification fail-closed and constant-time, the Allowlist fails closed,
body reads are bounded, response bodies are closed, the process goroutines detach
correctly and publish through the mutex-guarded bus) — those were left as-is.

## Fixes

### 1. Boot-time data race in `notify` registration (Critical)
The `notify` tool was written into the kernel's live tool map
(`k.Tools()["notify"] = …`) AFTER the web/REST/OpenAI HTTP servers and the channels
had already started listening. A request arriving in that boot window would start a
run that iterates `k.tools` concurrently with the map write — a fatal
concurrent-map-write panic.
**Fix:** the tool is now created **unbound** and registered into the tool map
*before* the kernel is constructed (single-threaded boot, decided from env so it's
only added when a channel has an allowlist). Once the channels exist, `Bind` wires
the sender, synchronized by a `sync.RWMutex` against `Invoke`. The map is never
written after the kernel — and its servers — start. (`plugins/tools/notify/notify.go`,
`cmd/agezt/main.go`.)

### 2. Cross-user context bleed in shared channels (Privacy / High)
`ConversationHistory` (M144) folded by `(channel_kind, channel_id)` only. In a
shared Slack/Discord channel that carries many users, every user's messages folded
into one transcript — so user B's run got user A's prior messages as prompt context
(cross-user prompt-injection / info leak).
**Fix:** the fold now isolates per sender — it includes only the requesting sender's
own inbound messages and the agent replies that share one of their run correlation
ids (inbound and its reply share the run corr). A new test
(`TestConversationHistory_IsolatesBySender`) locks it: Alice's transcript never
contains Bob's messages or the replies to Bob. DMs (single user) are unchanged.
(`kernel/channel/history.go`.)

### 3. Slack replay within the signature window (Security / Critical)
The HMAC signature proves authenticity but its 5-minute freshness window still
permits replaying a captured signed body *without* the `X-Slack-Retry-Num` header,
which would reprocess the message and drive a fresh agent run each time.
**Fix:** a bounded FIFO seen-set keyed on the immutable `channel+ts` gives
exactly-once processing within the window; a replay is ACKed (200) but not
reprocessed. (`plugins/channels/slack/slack.go`,
`TestSlack_ReplayDeduped`.)

### 4. Slack `send` false-success (Correctness / Medium)
Slack returns HTTP 200 even on application errors (`{"ok":false,"error":…}`). The
old code only flagged an error when the body decoded AND `ok:false` AND a non-empty
error — so a decode failure (or `ok:false` with no error string) was treated as a
successful send and journaled as `channel.outbound` (delivered).
**Fix:** a decode failure or `ok:false` is now a real error and is NOT journaled as
delivered. (`plugins/channels/slack/slack.go`.)

### 5. `notify` silent partial failure (Correctness / Medium)
A multi-recipient notify that delivered to some but failed others returned a success
result (`IsError:false`) with the failures only mentioned in the text — so
automation keying on `IsError` saw "sent" even when the intended channel failed.
**Fix:** any failed delivery now sets `IsError:true` and names the failed
recipients. (`plugins/tools/notify/notify.go`, `TestNotify_PartialFailureIsError`.)

### 6. UTF-8-unsafe transcript clipping (Correctness / Medium)
`clip` truncated at a byte offset, which can split a multibyte rune (emoji/CJK —
exactly the international-chat content most likely to be long), producing invalid
UTF-8 in the prompt and journal.
**Fix:** rune-aware truncation (back up to a rune boundary). (`kernel/channel/history.go`.)

### 7. Discord prompt-option selection (Correctness / High)
`text()` returned the first string-valued option regardless of name or option type,
so a reordered or additional option could silently feed the agent the wrong field,
and sub-command option types weren't excluded.
**Fix:** the prompt is taken from the option explicitly named `prompt`, considering
only STRING options (type 3), with first-STRING as a fallback.
(`plugins/channels/discord/discord.go`.)

### 8. Missing `ReadHeaderTimeout` (Hardening / Low)
The Slack/Discord webhook `http.Server`s had no header-read timeout (gosec G112 /
slowloris). Both now set `ReadHeaderTimeout: 10s`.
(`plugins/channels/slack/slack.go`, `plugins/channels/discord/discord.go`.)

## Deliberately not changed
- **`agt send` targets any recipient id** (review H3): by design. It is gated by the
  primary control-plane token, which already grants full daemon authority (shell,
  file, etc.) — so constraining outbound messaging adds no real security. Documented
  as intentional in `send.go`.
- **`ConversationHistory` is O(journal) per message** (review H2.2): a real scaling
  consideration, but chat is low-frequency and the journal `Range` API has no
  bounded/reverse variant today. Noted as a known limitation; revisit if a tail-read
  API lands.

## Verification
- `go.mod` / `go.sum` unchanged.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all 8 touched files.
- `go test ./...` — **FAIL 0**, **1469 tests** (was 1465; +4 net), 61 packages.
- Live regression re-proven with the offline mock + fake Discord API: the `notify`
  tool still binds and the agent still fires it; same-sender multi-turn context
  still carries (turn 2 sees turn 1) — confirming the deferred-binding and
  sender-scoping changes are non-regressive.

## Result
The channel arc is now hardened against a boot-time crash, a cross-user privacy
leak, a webhook replay, two false-success/partial-failure reporting bugs, a UTF-8
corruption, a wrong-field selection, and slowloris — with tests locking the
security-relevant ones, and the end-to-end behavior unchanged.
