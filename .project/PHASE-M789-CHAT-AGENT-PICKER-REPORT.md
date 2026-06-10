# Phase M789 — converse AS a named agent (per-conversation agent picker)

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** multi-agent identity, step 7
(the user-visible link: M783 identities reachable from the default surface).

## What

Chat composer gains an **agent picker** (next to ModelPicker): pick a roster
agent and the conversation runs AS it — soul, model chain, memory scope,
budget (M783/M786/M787). Per-thread persistence (M712 pattern); finished
turns show "as <slug>" in the meta.

## Changes

- Go: webui run-proxy body allowlist += `agent` (forwarded to CmdRun's
  existing M783 seam — unknown/paused refused there); handleRun's terminal
  result += `agent: <slug>` when a profile ran.
- frontend: conversations.ts `agent` field + activeConvAgent/
  withActiveConvAgent; chatStore exposes agent/setAgent + sends the arg;
  chat.ts ChatTurn.agent folded from the done frame; new
  components/AgentPicker.tsx (trigger + dropdown; enabled agents only via
  /api/agents; "default identity" option; empty-state points at Roster);
  Chat.tsx renders the picker + TurnMeta "as <slug>". Dist rebuilt (LF).

## Tests (5 new; 455 vitest total green)

conversations per-thread agent set/clear/persist (mirrors the M712 model
test) · done-fold captures `agent` (and stays undefined on plain runs) ·
AgentPicker lists enabled-only + picks (paused hidden) · clears to default
identity + trigger shows the active slug.

## Browser verification (real browser, isolated daemon, new dist)

Created "researcher" via CLI → #chat → picked it (trigger showed the slug) →
sent a message → answer rendered with **as researcher** in the turn meta —
the run provably executed AS the profile end to end. 0 console errors during
the session. Clean shutdown, 0 panics, smoke dir removed.

## Gate

Full Go suite + vet + staticcheck green; 455 vitest; tsc + vite build clean;
dist rotation committed; go.mod unchanged. CI org-billing still blocked →
local battery + arc-authority merge.

## Next in the arc

Board view to/reply_to threading · workdir wiring · per-agent daily budget
ledger · ask-wake recipe (standing order on board.dm.<slug> + --agent plan).
