# Expected Output

The exact agent ids, event ids, and timestamps will differ run-to-run. The `run.sh` script greps for stable substrings rather than literal equality.

## Normal output

```text
starting keyless echo daemon...
  ok: daemon ready
  ok: control plane reachable

=== durable agent creation ===
  ok: leader agent created
  ok: worker agent created (parent=leader, managed sub-agent)

=== agent roster ===
  ok: roster shows leader + worker with parent/child relationship

=== effective authority (leader) ===
  ok: leader authority: trust ceiling + tool deny visible

=== effective authority (worker) ===
  ok: worker authority: lower ceiling + stricter deny visible

=== mailbox-wake setup (standing order) ===
  ok: mailbox-wake standing order armed
  -- OR --
  note: standing add output: ...
  note: mailbox-wake arming may require daemon version support — skipping

=== manual wake + wake causality ===
agent leader wake accepted corr=...
  ok: leader wake accepted
  ok: agt why walked the wake causality chain
  -- OR --
  note: no journaled events found to walk — this can happen if the wake is still in-flight

=== agent detail (durable identity proof) ===
  ok: agent show renders durable identity (slug, trust ceiling, tools)

=== shutdown ===
  ok: graceful shutdown, 0 panics

MAILBOX WAKE & AGENT HIERARCHY DEMO: PASS
```

## Key assertions

1. `agt agent add leader` and `agt agent add worker --parent-agent leader --direct-callable false` succeed.
2. `agt agent list` shows both agents with `parent=leader` on the worker.
3. `agt agent authority leader` shows trust ceiling + tool deny (shell denied).
4. `agt agent authority worker` shows a lower trust ceiling + stricter deny list.
5. `agt agent wake leader "..." --reason "demo"` produces an accepted event.
6. `agt agent show leader` renders slug, trust ceiling, and tools — proving durable identity.

## Honest-empty states

- The standing-order `--event "board.dm.leader"` may not be accepted by all daemon versions. The demo treats both success and skip as valid.
- The wake event may still be in-flight when the script tries `journal tail`. The demo notes this rather than failing.

## If it fails

- **`daemon did not become ready`**: ensure `AGEZT_DEMO_ECHO=1` is set (the daemon refuses to start without a provider unless echo mode is on).
- **`could not create leader agent`**: the roster may already have an agent with that slug from a previous run. The temp home prevents this, but a stale `AGEZT_HOME` can cause it.
- **`wake did not produce an accepted event`**: the echo provider may have returned before the control plane journaled the wake. Increase the `sleep 2` before journal tail.
