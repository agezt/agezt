# M140 — `agt inbox --channel KIND` filter

## Why
M138 and M139 added Slack and Discord, so the daemon now drives three duplex
channels (Telegram, Slack, Discord). The Unified Inbox (`agt inbox`, SPEC-07 §4)
folds `channel.inbound` / `channel.outbound` events from ALL of them into one
newest-first thread list — which is the right default, but with three platforms
an operator triaging "what came in over Slack?" had to eyeball a mixed stream.
The fold already carries each thread's `channel_kind`; it just wasn't filterable.
A one-axis, server-side filter closes that gap and rides the existing journal-fold
pattern (no new store, no new event).

## What
- **Server** (`kernel/controlplane/inbox.go`) — `handleInbox` reads an optional
  `channel` arg (string), normalized to lowercase/trimmed. Threads whose
  `channel_kind` doesn't match are dropped **before** the limit is applied, so
  `limit` counts within the filtered set ("the last N Slack threads", not "Slack
  threads among the last N"). The applied filter is echoed back as `channel` in the
  result for transparency; empty/absent means "all channels" (unchanged behavior).
- **CLI** (`cmd/agt/inbox.go`) — `agt inbox [N] [--channel KIND] [--json]`. Accepts
  both `--channel slack` and `--channel=slack`; `--channel` with no value is a usage
  error (exit 2). The empty-result message becomes kind-specific
  ("inbox empty (no slack messages yet)"). Help documents the flag and the known
  kinds (telegram|slack|discord).
- **Protocol** (`kernel/controlplane/protocol.go`) — `CmdInbox` doc updated: `channel`
  arg + `channel?` in the return shape.

## Files
- `kernel/controlplane/inbox.go` — `channel` arg parse + filter + echo.
- `cmd/agt/inbox.go` — `--channel` flag, kind-specific empty message, help.
- `kernel/controlplane/protocol.go` — `CmdInbox` doc comment.
- `kernel/controlplane/inbox_test.go` — `TestInboxFilterByChannel`.
- `cmd/agt/inbox_test.go` — `TestCmdInbox_ChannelNeedsValue`,
  `TestCmdInbox_HelpMentionsChannel`.

## Tests (+3, all passing)
- `TestInboxFilterByChannel` — three single-message threads (telegram/slack/discord);
  unfiltered → 3; `channel:"DISCORD"` (uppercase) → exactly the discord thread, with
  `channel:"discord"` echoed; an unknown kind ("matrix") → empty.
- `TestCmdInbox_ChannelNeedsValue` — `--channel` with no value → exit 2 + "needs a
  value".
- `TestCmdInbox_HelpMentionsChannel` — help text documents `--channel`.

## Live proof (offline mock, real booted daemon)
A daemon was booted with `AGEZT_PROVIDER=mock` and the Discord channel enabled; a
throwaway Ed25519 signer fired one signed slash command (channel `D9`), producing a
real inbound+outbound pair in the journal. Then the CLI:

```
$ agt inbox                       # all channels
1 thread(s):
── discord/D9
   ← hi from discord
   → [offline-mock] I ran a directory listing via the shell tool. This project is Agezt…

$ agt inbox --channel discord     # matches
1 thread(s):
── discord/D9
   ← hi from discord
   → [offline-mock] …

$ agt inbox --channel slack        # no Slack traffic
inbox empty (no slack messages yet)
```

Filter-match, filtered-empty (kind-specific message), and the all-channels default
all confirmed end-to-end; case-insensitivity (`DISCORD`) is covered by the unit test.

## Verification
- `go.mod` / `go.sum` unchanged.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all touched files.
- `go test ./...` — **FAIL 0**, **1444 tests** (was 1441; +3).

## Result
The Unified Inbox is now scopable to a single channel kind server-side, so triaging
one platform's conversations among three is a flag, not a scan — and the fold stays
a single read over the journal with no new state.
