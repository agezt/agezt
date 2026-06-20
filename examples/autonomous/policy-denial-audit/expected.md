# Expected Output

The exact timestamps, ids, and counts will differ run-to-run. The `run.sh` script greps for stable substrings rather than literal equality, so this file describes the shape, not a golden transcript.

## Normal (fresh daemon) output

```text
building binaries...
starting keyless echo daemon...
  ok: daemon ready
  ok: control plane reachable

=== policy dry-run: catastrophic shell input ===
decision : deny (level=...)
...
DEMO FAIL: expected 'rm -rf /' to be denied (exit 3), got exit 0
```

Wait — that is the failure path if the built-in hard-deny floor does not catch `rm -rf /`. On a correctly-loaded daemon the expected output is:

```text
starting keyless echo daemon...
  ok: daemon ready
  ok: control plane reachable

=== policy dry-run: catastrophic shell input ===
decision : deny (level=...)
hard-deny: ...
  ok: catastrophic shell input hard-denied

=== policy dry-run: benign shell input ===
decision : ... (level=...)
  ok: benign shell input reports decision + level

=== policy decision audit surface ===
  ok: edict log returned a decisions array
  -- OR --
  note: no policy decisions journaled yet (fresh daemon) — honest empty result

=== policy decision aggregate ===
  ok: edict stats returned (total=0)
  -- OR --
  ok: edict stats returned (total=N)

=== audit chain (agt why) ===
  ok: agt why walked the chain for <event_id>
  -- OR --
  note: no journaled events yet — skipped agt why (fresh daemon)

=== shutdown ===
  ok: graceful shutdown, 0 panics

POLICY DENIAL & AUDIT DEMO: PASS
```

## Key assertions

1. `agt edict test shell "rm -rf /"` exits **3** (deny), not 0 and not 1.
2. `agt edict test shell "echo hi"` prints a `decision` line and a `level` line.
3. `agt edict log --json` returns a JSON object with a `decisions` field, or the daemon honestly reports none yet.
4. `agt edict stats --json` returns a JSON object with a `total` field.
5. `agt why <event_id>` prints a line containing `events in correlation` when an event exists.
6. The daemon shuts down cleanly with no panic in its log.

## Honest-empty states

Because `agt edict test` is a dry-run and does not journal, a freshly booted daemon may have zero `policy.decision` events. The demo treats both populated and empty results as valid — AGEZT surfaces "no decisions yet" honestly rather than inventing them. The strong claims of the demo are the two dry-run decisions (steps 2 and 3), which always run against the live policy snapshot.

## If it fails

- **`daemon did not become ready`**: check `$AGEZT_HOME/daemon.log`. The most common cause is a stale Go build cache or a missing `AGEZT_DEMO_ECHO=1` (the daemon refuses to start without a provider unless echo mode is on).
- **`expected 'rm -rf /' to be denied`**: the hard-deny floor for the shell capability did not load. Run `agt edict show` and confirm a rule matching `rm -rf /` is present under `hard-deny rules`.
- **`agt status could not reach daemon`**: the control-plane socket is not where the CLI expects it. Confirm `AGEZT_HOME` is exported in the same shell that runs `agt`.
- **panic in daemon log**: a genuine bug. Capture the log and the AGEZT version (`agt version`) and report it.
