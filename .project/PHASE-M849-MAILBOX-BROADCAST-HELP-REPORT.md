# PHASE M849 — Agent Mailbox: Broadcast + Help Requests

**Status:** shipped
**Milestone:** M849
**Theme:** Give agents the two communication primitives the board was missing —
**broadcast** (announce to every agent) and **help requests** (ask for assistance,
open until answered). Owner ask: *"agentlar arası sohbet… mailbox sistemi…
broadcast…"* (task #45).

The board (kernel/board, M647/M788) already did directed agent-to-agent DMs
(`send`/`inbox`/`reply`/`replies`, `board.dm.<slug>` events, standing-order wake).
Rather than stand up a parallel `kernel/mailbox` package that would duplicate the
store, journaling, and atomic-write machinery, M849 **extends the board into the
mailbox** the owner wants — the lean path consistent with AGEZT's single-store
ethos.

## What shipped

- **Broadcast.** `board.Broadcast(from, text)` posts a message addressed to
  `Everyone` ("*"); it lands in every agent's `Inbox` except the sender's. Tool
  `op=broadcast`; journaled under subject `board.broadcast`.
- **Help requests.** `board.HelpRequest(from, to, text)` flags a message `Help`:
  broadcast to all (or directed via `to=<slug>`), it stays in inboxes until any
  agent answers it. Tool `op=help` (with text → raise; **without text → list the
  open requests**). Journaled under `board.help` (or `board.help.<slug>` when
  directed), so a standing order can wake a responder.
- **Open-help view.** `board.OpenHelp(limit)` returns the still-unanswered help
  requests, newest first — the "who needs help" surface. Control plane
  `CmdBoardHelp`; read-only webui `/api/board/help`.
- **Inbox is broadcast-aware.** `Inbox(slug)` now also returns `To == "*"`
  broadcasts (excluding the reader's own), under the existing answered-filter (a
  broadcast/help clears for everyone once anyone replies).
- **Board view (Web UI).** A "N open help requests" strip at the top of the Agent
  Board, plus `help` (LifeBuoy) and broadcast `all` (Megaphone) tags on messages.

## Surface

- `kernel/board/board.go` — `Message.Help`, `Everyone` const, `Broadcast`,
  `HelpRequest`, `OpenHelp`, broadcast-aware `Inbox`; `board_test.go` (+3 tests).
- `plugins/tools/boardtool/{board,tool}.go` — store interface + `op=broadcast` /
  `op=help`, `help` in `msgView`; `board_test.go` (+3 tests, broadcast-aware fake).
- `cmd/agezt/main.go` — notifier subjects `board.broadcast` / `board.help[.<to>]`
  + `help` in payload.
- `kernel/controlplane/{board,protocol,server}.go` — `handleBoardHelp`,
  `CmdBoardHelp`, dispatch; `help` flag in the read view.
- `kernel/webui/webui.go` — `/api/board/help` (apiRoutes); `webui_test.go`
  readOnly guard entry.
- `frontend/src/views/Board.tsx` — open-help strip + help/broadcast tags; dist
  rebuilt (LF).

## Verification

- **Gate:** `go build`, `go vet`, `staticcheck`, linux cross-build clean;
  `kernel/board`, `boardtool`, `controlplane`, `webui` green; vitest **517 passed**;
  dist rebuilt. No new env (reuses `KindBoardPosted` + subjects, no contract
  churn); go.mod unchanged.
- **Unit:** broadcast lands in peers' (not sender's) inbox; help open-until-
  answered (clears OpenHelp + inbox on reply); directed help reaches only its
  target; tool ops raise/list/answer + bad-input cases.
- **Live (isolated home, pre-seeded board):** `/api/board/help` returned only the
  **open** help (excluded the answered request and the non-help broadcast); the
  board feed carried the `help` flag and the `to:"*"` broadcast.

## Notes
- Default-allow preserved: broadcast/help are ordinary board writes; no new gate.
- Builds toward the **brain overseer** (#46), which can read `OpenHelp` to triage
  and dispatch help, and pairs with the M781 unseen-alert / Cockpit surfaces.
