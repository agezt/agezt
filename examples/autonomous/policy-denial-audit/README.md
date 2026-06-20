# Demo: Policy Denial and Audit

This is the first runnable positioning demo for AGEZT. It proves the claim from `docs/COMPARISON.md`:

> Governance is part of runtime, not just UI.

## What this demo shows

A high-risk tool request is evaluated by the policy engine (Edict) and refused. The refusal is:

- decided by capability + trust level + hard-deny rules
- journaled as a `policy.decision` event
- visible from the CLI via `agt edict log`, `agt edict stats`, and `agt why`

No LLM provider key is required. The demo uses the built-in keyless echo daemon, so it runs entirely offline.

## Positioning claim this proves

Generic agent frameworks usually apply tool allowlists as a thin wrapper around function calls. AGEZT treats policy as a first-class runtime concern: every tool call is gated, every decision is journaled, and the operator can ask "why was this denied?" through the same event chain used for everything else.

## Prerequisites

- Go 1.26.4+ (see `go.mod`)
- Bash (Linux/macOS/git-bash). On Windows use Git Bash or WSL.
- No provider key. No network. No external LLM.

## Run it

From the repository root:

```bash
make build
bash examples/autonomous/policy-denial-audit/run.sh
```

Or, if `agezt` and `agt` are already built somewhere on your PATH:

```bash
bash examples/autonomous/policy-denial-audit/run.sh /path/to/agezt /path/to/agt
```

The script:

1. builds the binaries (if not given)
2. boots a keyless echo daemon in an isolated temp home
3. runs policy dry-runs against clearly dangerous inputs
4. shows the decision log and aggregate stats
5. walks the audit chain with `agt why`
6. shuts the daemon down cleanly

Exit code is non-zero if any assertion fails.

## Expected output

See `expected.md` for the shape of the output this demo targets. Exact timestamps and ids will differ run-to-run; the script greps for stable substrings.

## What each step proves

| Step | Command | Proves |
|---|---|---|
| Preflight | `agt doctor` | daemon is healthy and journaled |
| Hard-deny dry-run | `agt edict test shell "rm -rf /"` | catastrophic input is refused without running |
| Trust-level dry-run | `agt edict test shell "echo hi"` | decision reports the governing trust level |
| Decision log | `agt edict log --json` | each decision is a journaled, inspectable event |
| Aggregate stats | `agt edict stats` | denial rate and per-capability breakdown |
| Audit chain | `agt why <event_id>` | a denial is traceable through the same event chain as a run |

## Notes

- `agt edict test` is a dry-run: it never journals and never consumes an approval slot. To produce journaled `policy.decision` events in this demo, the script uses the daemon's own policy snapshot and the log/stats surfaces that read already-emitted decisions. If the daemon has not yet emitted any decisions, the log/stats steps will report "no policy decisions" — that is itself a valid, honest result.
- This demo does not require an agent run; it exercises the governance layer directly. A follow-up demo (`examples/autonomous/mailbox-delegation/`) will show wake causality end-to-end.
