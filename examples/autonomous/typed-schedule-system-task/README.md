# Demo: Typed Schedule — System Task

This is the third runnable positioning demo for AGEZT. It proves the claim from `docs/COMPARISON.md`:

> Typed schedules, not cron-wrapped prompts.

## What this demo shows

1. **Typed target validation.** A schedule with a `--system-task` target accepts only known task names from an enum — you cannot pass arbitrary text or agent instructions.
2. **System-task execution without an agent.** Firing a `catalog_sync` system task runs daemon maintenance without waking an LLM agent — no prompt, no model cost.
3. **Fire history with typed metadata.** `schedule fires` shows the target type, target id, and outcome — proving schedules are auditable infrastructure, not hidden prompt storage.
4. **Prompt-smuggling resistance.** Attempting to add a schedule with an invalid system-task name is rejected.

## Positioning claim this proves

Generic agent frameworks often treat scheduled work as a prompt with a timer. AGEZT treats schedules as typed infrastructure: the target type determines what runs, the payload is validated, and system tasks execute daemon-side code without consuming LLM budget.

## Prerequisites

- Go 1.26.4+ (see `go.mod`)
- Bash (Linux/macOS/git-bash). On Windows use Git Bash or WSL.
- No provider key. No network. No external LLM.

## Run it

From the repository root:

```bash
make build
bash examples/autonomous/typed-schedule-system-task/run.sh
```

Or with prebuilt binaries:

```bash
bash examples/autonomous/typed-schedule-system-task/run.sh /path/to/agezt /path/to/agt
```

## Available system tasks

| Name | Category | What it does |
|---|---|---|
| `catalog_sync` | catalog | Download models.dev catalog, reload provider/model metadata |
| `artifact_collect` | storage | Index offloaded run artifacts |
| `memory_clean` | memory | Run memory maintenance + publish summary |
| `memory_tidy` | memory | Run lightweight memory hygiene |
| `log_clean` | logs | Inspect journal/log pressure + publish summary |
| `graveyard_scan` | graveyard | Report retired agents past retention window (notify-only) |

None of these wake an LLM agent. They are daemon-side maintenance jobs.
