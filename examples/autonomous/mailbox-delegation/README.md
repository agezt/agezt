# Demo: Mailbox Wake and Agent Hierarchy

This is the second runnable positioning demo for AGEZT. It proves the claims from `docs/COMPARISON.md`:

> Agents are durable roster identities, not prompt sessions.
> Wake causality is a product concept.

## What this demo shows

1. **Durable agent creation.** A leader agent and a worker sub-agent are created with an explicit parent/child ownership relationship.
2. **Effective authority.** `agt agent authority` shows the merged tool/trust/policy view for each agent.
3. **Mailbox-wake setup.** A standing order arms a board-trigger subject so a message addressed to the leader can wake it.
4. **Wake causality.** A manual wake produces an `agent.wake` event carrying an autonomy runbook; `agt why` walks the event chain.

## What this demo does NOT show (honest limitation)

The echo provider (`AGEZT_DEMO_ECHO=1`) returns text but does not produce LLM tool calls. Actual sub-agent delegation (the leader calling `delegate` to fan out to the worker) requires a real provider that emits tool-use requests. This demo proves the **identity, authority, and wake-causality infrastructure** that delegation builds on, not a live delegation execution.

To see live delegation, point the daemon at a real provider and run `agt run --agent leader "ask the worker to check the build"`.

## Prerequisites

- Go 1.26.4+ (see `go.mod`)
- Bash (Linux/macOS/git-bash). On Windows use Git Bash or WSL.
- No provider key. No network. No external LLM.

## Run it

From the repository root:

```bash
make build
bash examples/autonomous/mailbox-delegation/run.sh
```

Or with prebuilt binaries:

```bash
bash examples/autonomous/mailbox-delegation/run.sh /path/to/agezt /path/to/agt
```

## Expected output

See `expected.md` for the shape of the output this demo targets.

## Positioning claims this proves

| Claim | How |
|---|---|
| Durable identity, not a prompt session | Agents survive across runs; inspectable via `agt agent show` |
| Agent hierarchy | Parent/child ownership; managed sub-agents route through their parent |
| Effective authority proof | `agt agent authority` merges profile + policy overlay |
| Wake causality | `agent.wake` event → autonomy runbook → `agt why` audit chain |
| Typed triggers, not prompt storage | Standing orders use event subjects, not inline prompts |
