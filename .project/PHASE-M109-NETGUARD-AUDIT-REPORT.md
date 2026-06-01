# Phase Report — Milestone M109 (egress-block audit)

> Status: **shipped** · Date: 2026-06-02 · SPEC-06 / M16 egress guard.

## Why

M105 let an operator PREVIEW the egress policy (`agt netguard test`). But the
guard refused connections silently at runtime — nothing recorded that a tool
actually tried to reach an internal/metadata address. A tool reaching for
169.254.169.254 is a strong signal of prompt injection or attempted
exfiltration; that belongs in the permanent, hash-chained audit trail, not lost
as a transient dial error.

## What shipped

- **`netguard.OnBlock(fn)`** — an option that fires a callback (resolved IP +
  reason) every time the guard refuses a dial. Cheap, non-blocking, on the dial
  path.
- **Wired through the http and browser tools** (`Tool.OnBlock`) and installed by
  the daemon (`wireNetguardAudit`, after the kernel exists, mirroring
  `gov.SetBus`) to publish a **`netguard.blocked`** event into the journal.
- **`agt netguard log [N] [--tenant <id>] [--since <dur>] [--json]`** — the
  audit timeline of refused egress (ts, IP, tool, reason). Tenant-routed and on
  the tenant-token allowlist.

## Design notes

- **Audit, not enforcement.** The block already happened (M16); this only
  records it. OnBlock is best-effort and never alters the refusal.
- **The host allowlist runs first.** A blocked-by-allowlist host never reaches
  the IP guard, so `netguard.blocked` specifically captures the dangerous case:
  an *allowed* host (or `AGEZT_HTTP_ALLOW_ALL`) resolving to an internal IP — a
  DNS-rebinding / SSRF trap.

## Tests

- `TestOnBlock_FiresOnRefusal` (netguard) — the callback fires once with the IP
  + reason on a blocked dial, and never on an allowed one.
- `TestNetguardLog` (control plane) — empty before any block; after journaling
  blocks, the fold returns them newest-first with tool/ip.

Test count: **1368 → 1370**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ AGEZT_DEMO_SSRF=1 AGEZT_HTTP_ALLOW_ALL=1 agezt &     # agent fetches the metadata endpoint
$ agt run "read the instance metadata"
$ agt netguard log
  2026-06-02 00:02:56  BLOCKED  169.254.169.254  via http  — link-local (cloud metadata / autoconf)
```
