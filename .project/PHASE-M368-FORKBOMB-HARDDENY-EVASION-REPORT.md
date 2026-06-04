# M368 — Fix fork-bomb hard-deny whitespace evasion (SPEC-06 §3.2)

## SPEC audit (read-vs-code)
SPEC-06 §3.2 lists the immutable `hard_deny` floor — rules that ALWAYS deny
regardless of trust level — including:

> `- match: { tool: shell, command_glob: ["rm -rf /", ":(){ :|:& };:"] }`

The Edict engine implements this floor (`DefaultHardDeny`, DECISIONS F4): a
substring match, with `denyCandidates` normalising the input to defeat evasion
(JSON-escape decoding, whitespace-run collapse → `rm  -rf /` becomes `rm -rf /`).

**Verified gap (a real, demonstrated bug — not a coverage gap):** the fork-bomb
rule is stored as the no-space form `:(){:|:&};:`, but the matcher only collapsed
whitespace *runs* to a single space — it never removed the syntactically-optional
internal spaces of the **canonical** fork bomb `:(){ :|:& };:`. Reproduced via the
public `Engine.Decide(CapShell, …)`:

| input | before |
|---|---|
| `:(){:|:&};:` (no-space, the stored rule — not valid bash) | hard-denied ✓ |
| `:(){ :|:& };:` (canonical, valid) | **hard-denied = false** ✗ |
| `bash -c ':(){ :|:& };:'` | **false** ✗ |
| `{"command":":(){ :|:& };:"}` (the agent's tool-input shape) | **false** ✗ |

So the floor caught a variant that doesn't even run, and missed the one that
does — a genuine security-correctness bug in the immutable deny floor (priority
A).

## What
- **`kernel/edict/edict.go`** — `denyCandidates` now also derives a fully
  whitespace-STRIPPED candidate (`stripWhitespace` = `strings.Fields` joined
  with ""), in addition to the existing collapsed one. `:(){ :|:& };:` →
  `:(){:|:&};:` matches the floor rule; JSON-wrapped forms are decoded first then
  stripped. Rewrote `denyCandidates` with a dedup set so raw/collapsed/stripped
  variants never duplicate. Space-bearing rules (`rm -rf /`, `dd if=`,
  `shutdown -`) cannot match a stripped candidate, so they are unaffected and
  still fire via the raw/collapsed candidate — no regression, no behaviour change
  for them.

## Verification
- **`edict_forkbomb_test.go`** (3 tests): every fork-bomb spacing variant
  (canonical, no-space, extra padding, bash-wrapped, JSON-wrapped, tab-separated)
  is now hard-denied by rule `fork-bomb`; a benign-command set is NOT denied
  (the stripped candidate doesn't over-block ordinary commands); the space-
  bearing `rm -rf /` family still fires (collapsed path intact).
- All pre-existing edict tests still pass (no regression).
- Confirmed the one apparent over-match (`git commit -m "reboot the parser"`) is
  **pre-existing** behaviour: the raw input already contains the `reboot`
  substring and was hard-denied before this change too (verified by stashing the
  change) — the aggressive `reboot` substring rule is orthogonal to this fix.
- `gofmt -l` clean; `go vet` clean; `GOOS=linux go build ./...` exit 0. Full
  suite **2121** passing (was 2118; +3), `go test ./...` 0 failures.
  `go.mod`/`go.sum` unchanged. CHANGELOG (Security, user-visible safety).

## Scope notes
- Fail-closed bias is correct for a hard-deny floor: the stripped candidate can
  only *add* denials of whitespace-split dangerous tokens (which the no-space
  floor rules name), never relax one. For a security floor, slight over-blocking
  beats a fork bomb reaching the host.
- SPEC-06 §3.2 hard_deny floor now verified against its named dangers
  (`rm -rf /` ✓, fork bomb ✓ fixed, mkfs/dd/shutdown/reboot ✓). `file delete
  outside workspace` is enforced by the file tool's own containment
  (M252/M253), and `disable_audit`/`exfiltrate_secret` are abstract actions not
  expressible as shell substrings — noted, not gaps.
