# Expected Output

The exact tool counts, hashes, and timestamps will differ run-to-run. The `run.sh` script greps for stable substrings rather than literal equality.

## Normal output

```text
starting keyless echo daemon...
  ok: daemon ready
  ok: control plane reachable

=== tool registry (what the model sees) ===
  ok: tool list shows N registered tools

=== policy gating: catastrophic tool input denied ===
decision : deny (level=...)
  ok: catastrophic shell input denied by policy (not just unadvertised)

=== tool invocation audit surface ===
  ok: tool log returned an invocations array
  -- OR --
  note: no tool invocations yet (fresh daemon) — honest empty result

=== plugin pin hashing (BLAKE3-256) ===
  ok: plugin hash produced a valid BLAKE3-256 digest
    digest: a1b2c3d4e5f6...
    usable as: AGEZT_PLUGIN_PINS="myprefix=..."

=== plugin list (external plugin trust surface) ===
  ok: plugin list correctly reports no external plugins (fresh daemon)
    to load one: AGEZT_PLUGINS="prefix=/path/to/plugin" AGEZT_PLUGIN_PINS="prefix=<hash>"

=== governance chain (tool → policy → audit) ===
  ok: edict log shows policy decisions (governance audit trail)
  -- OR --
  note: no policy decisions journaled yet (edict test is a dry-run)
  ok: edict stats shows aggregate policy metrics

=== governance model summary ===
  tool list    → what the model CAN call
  edict test   → what policy WOULD decide (dry-run)
  edict log    → what policy DID decide (journaled)
  tool log     → what the agent ACTUALLY ran
  plugin list  → what external plugins are loaded + pinned
  plugin hash  → how to pin a plugin binary
  ok: governance is a layered chain, not a single gate

=== shutdown ===
  ok: graceful shutdown, 0 panics

PLUGIN AND TOOL GOVERNANCE DEMO: PASS
```

## Key assertions

1. `agt tool list` reports a non-zero tool count (the daemon advertises tools to the model).
2. `agt edict test shell "rm -rf /"` exits 3 (denied) — proving policy gates tool capabilities at runtime.
3. `agt tool log --json` returns a JSON object with a `calls` field, or honestly reports no invocations yet.
4. `agt plugin hash <binary>` produces a valid 64-character lowercase hex BLAKE3-256 digest.
5. `agt plugin list` reports no external plugins (fresh daemon) or shows loaded plugins with pin status.
6. `agt edict log --json` and `agt edict stats --json` return policy decision data.
7. The demo prints a governance model summary showing the layered chain: tool list → edict test → edict log → tool log → plugin list → plugin hash.

## Honest-empty states

- On a fresh daemon with no agent runs, `tool log` may be empty. This is valid — no tools have been invoked yet.
- `edict log` may be empty because `edict test` is a dry-run that does not journal. A live agent run with a denied tool call would populate it.
- The echo daemon has no external plugins by default. `plugin list` reports this honestly.

## If it fails

- **`tool list should report tool count`**: the daemon may not have registered any tools. Check `agt tool list --json` for the raw response.
- **`plugin hash did not produce a 64-char hex digest`**: the hash target (the agt binary itself) may not be readable. Check the path.
- **`expected 'rm -rf /' to be denied`**: the hard-deny floor for the shell capability did not load. Run `agt edict show` and confirm a rule matching `rm -rf /` is present.
