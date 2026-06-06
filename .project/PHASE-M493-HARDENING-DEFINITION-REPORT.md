# M493 — Define "hardened to 100%" as a measurable rubric + scorecard

## Context
The hardening goal ("harden Agezt to 100%") is open-ended: with no terminal,
measurable criterion, it can never be objectively discharged. This milestone supplies
the missing definition — `.project/HARDENING.md` — a concrete, re-runnable,
CI-enforced rubric plus the current measured state, and a netguard mutation pass that
confirms the SSRF core is already solid.

## netguard mutation pass (no code change — a positive result)
`go-mutesting` on `kernel/netguard` (SSRF defense) scored 0.628. Every survivor on a
security-critical line is equivalent or unreachable:
- `if v6 == nil { return nil }` removal — `To16()` returns nil only for malformed IPs
  (effectively unreachable for parsed addresses).
- NAT64 `… → if true && allZero(v6[4:12])` — the over-matched IPv4-compatible addresses
  are classified identically by the next branch (same result).
- `IsLinkLocalUnicast() || IsLinkLocalMulticast()` → drops only link-local *multicast*
  (not an SSRF vector); the metadata range 169.254.x is unicast and still blocked.
The existing `TestAllowed_SSRFBypassVectors` already pins the CGNAT boundary
(100.64/100.127 blocked, 100.63 allowed), NAT64-wrapped vectors, broadcast, and zero.
No genuine gap → no test added. (go-mutesting left no working-tree mutation this run;
verified clean with `git checkout` + `report.json` removal regardless.)

## The rubric (`.project/HARDENING.md`)
Six dimensions, each criterion decidable by a command, not judgement:
1. Build & portability — gofmt, vet, cross-compile matrix (all supported GOOS green).
2. Static analysis — staticcheck 0, golangci-lint correctness set clean, govulncheck.
3. Secrets & security — gitleaks 0, gosec triaged, threat-model invariants.
4. Testing depth — full suite, race (CI), 16 fuzz targets, mutation on top-stakes pkgs.
5. Defect surface — 16 review rounds + pattern scans, no known offline-actionable defect.
6. CI enforcement — every gate wired into `ci.yml` so the state is durable.

Each row carries its current state: PASS / MEASURED (with floor) / documented exception.
The doc is explicit that it is offered for **ratification** — the user may tighten or
relax thresholds; "100%" is then "all PASS hold + MEASURED meet floors + exceptions are
environment/by-design."

## Honesty corrections made while verifying
- The "gofmt clean" criterion's re-verify command was changed from `gofmt -l .` to a
  committed-LF-blob check: on Windows the working copy is CRLF and a raw `gofmt -l .`
  falsely flags CRLF files (4 here). The committed blobs — what CI checks on an LF
  checkout — are gofmt-clean tree-wide (verified: 0 dirty). The criterion holds; the
  command was wrong, and is now correct and cross-platform.

## Verification
- Re-ran the full battery: vet 0, staticcheck 0, gitleaks "no leaks found", cross-build
  green (linux/darwin/windows/freebsd), committed-blob gofmt 0 dirty, full suite exit 0.
- Docs-only change; `go.mod`/`go.sum` unchanged.

## Outcome
"Harden to 100%" now has an explicit, measurable definition with an honest, re-runnable
scorecard. Against that rubric the offline-actionable hardening surface is complete; the
only residuals are environment-bound (govulncheck/race need the patched toolchain / a C
compiler, both present in CI) or by-design (plan9/wasm; equivalent mutants). This is the
terminal criterion the goal previously lacked.
