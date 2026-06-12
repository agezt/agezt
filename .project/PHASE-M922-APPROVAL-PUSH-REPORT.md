# M922 — Approval-needed push to channels

## Problem

The owner's pain (verbatim): *"Approvals da bekleyen bir şeyi webui da görme
ihtimalim az"* — a run blocked on a human approval is easy to miss. M913 added a
header **ApprovalsBell** and M919 added opt-in desktop notifications, but both
require the **console to be open** (or at least a browser tab alive). A run can
sit BLOCKED on a HITL decision indefinitely while the operator is away from the
screen entirely.

The daemon already has the right machinery for "reach the operator with no
console open": `kernel/alerter` (M782) watches the bus and pushes
warning/critical proactive signals — run failures, halts, budget trips, blocked
egress, rate-limits — through the configured **Pulse channel sinks**
(Slack/Telegram/Discord). It just didn't classify `approval.requested`, which is
exactly the event the owner cares about.

## Change

One classifier case, no new wiring (the alerter already subscribes to `>`):

- `kernel/alerter/alerter.go` — added `case event.KindApprovalRequested` to
  `Classify`: `LevelWarning`, title **"approval needed"**, detail =
  `capability`/`tool_name` + `reason`, source `"approval"`. Updated the package
  doc comment to note approvals are now covered and that the console deliberately
  surfaces them via its own ApprovalsBell (not the Alerts view), so the two stay
  intent-aligned even though only the daemon classifies approvals as an alert.
- `kernel/alerter/alerter_test.go` — two table cases in
  `TestClassify_MirrorsConsoleAlertRules`: capability+reason detail, and
  tool_name fallback.

`approval.requested` is emitted by `kernel/approval/approval.go`
(`publishRequested`) with payload keys `approval_id, capability, tool_name,
input, reason, timeout_unix, created_unix` — so the alerter sees it on the bus
the moment a run blocks.

Delivery honours every existing gate: `MinLevel` (warning passes by default),
per-source mute (`MuteSources`), quiet hours (warnings held overnight; the
approval re-pushes when the next signal lands or can be granted from the
console), 5-minute cooldown per `kind+correlation`, and the per-window rate cap.
Default-allow posture preserved — nothing new to opt into; if you've wired a
channel you now hear about pending approvals for free.

## Why no new milestone for the rest of "proactive reach-out"

Auditing the daemon-side push surface for the owner's four directions: failures,
halts, budget, rate-limit and egress were **already** pushed by the M782
alerter. The only high-signal proactive event missing was `approval.requested`.
M922 closes that single gap; the proactive-reach-out direction is now complete on
the daemon side (it complements the M919 browser-side desktop notifications).

## Verification

- `go test ./kernel/alerter/ -count=1` → `ok` (0.037s).
- `go build ./...` → clean.
- `gofmt -d` on LF-normalized copies of both files → no diff (the `gofmt -l`
  listing is the Windows CRLF working-copy artifact; git stores LF).
- Go-only change: no frontend, no `kernel/webui/dist` rebuild.

Scope: `kernel/alerter/{alerter.go,alerter_test.go}` only — both non-contested
files (the concurrent session's uncommitted work is in cmd/agt + kernel/memory +
kernel/controlplane + kernel/runtime, untouched here). Shipped from an isolated
worktree branched off `origin/main`.
