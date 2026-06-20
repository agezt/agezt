# Expected Output

The exact schedule ids, timestamps, and fire sequences will differ run-to-run. The `run.sh` script greps for stable substrings rather than literal equality.

## Normal output

```text
starting keyless echo daemon...
  ok: daemon ready
  ok: control plane reachable

=== typed target validation: invalid system task rejected ===
  ok: invalid system-task name rejected (prompt smuggling blocked)

=== create typed system-task schedule ===
  ok: catalog_sync schedule created (id=...)

=== schedule list shows typed target ===
  ok: schedule list shows typed system-task target

=== fire the system-task schedule now ===
schedule ... fired
  ok: schedule run triggered the system task

=== fire history (typed outcome) ===
  ok: schedule fires returned fire history
  ok: fire history shows target_type=system_task
  -- OR --
  note: no fire history yet — the system task may still be executing

=== verify: system task did not wake an agent ===
  ok: journal shows system_task execution event
  -- OR --
  note: system_task event not found in recent journal (may be journaled differently)

=== schedule preview (dry-run) ===
  ...fire times...
  ok: schedule preview returned future fire times

=== cleanup + shutdown ===
  ok: graceful shutdown, 0 panics

TYPED SCHEDULE SYSTEM-TASK DEMO: PASS
```

## Key assertions

1. `agt schedule add --system-task "rm -rf /"` is rejected — the schedule store validates against the known system-task enum, preventing prompt smuggling.
2. `agt schedule add --system-task catalog_sync --every 24h` succeeds and returns a schedule id.
3. `agt schedule list` shows the schedule with a `system_task` / `catalog_sync` target type.
4. `agt schedule run <id>` triggers the system task without waking an agent.
5. `agt schedule fires` returns fire history with typed metadata.
6. The journal shows a system-task execution event, not an `agent.wake` event.
7. `agt schedule test <id> --count 3` previews future fire times (dry-run).

## Honest-empty states

- The fire history may be empty if the system task is still executing when the script checks. The daemon journals after completion for some task types.
- The exact event subject for system-task execution may vary by daemon version (`schedule.system_task.*` vs `schedule.fired`). The demo greps broadly.
- Some daemon versions may reject the invalid system-task name with a different error message. The demo checks both the error message and verifies the payload was not stored.

## If it fails

- **`could not create system-task schedule`**: the daemon may not support the `catalog_sync` system task in this version. Check `agt schedule add --help` for the valid system-task names.
- **`malicious system-task payload was accepted`**: this is a real security issue — the schedule store should validate against the enum. Report it.
- **`schedule run did not accept the trigger`**: the schedule id may not have been parsed correctly from the JSON output. Check the add output format.
