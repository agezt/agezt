# PHASE M848 — Capability Self-Awareness Briefing

**Status:** shipped
**Milestone:** M848
**Theme:** Tell every agent, plainly, how much it is allowed to do — so it acts
boldly instead of timidly assuming limits. Owner ask: *"ajanların kendi
kapasitelerini bilmesi lazım… istedikleri kadar uygulama geliştirebilir, python
scriptler javascript çalıştırmak, CLI tool kurmak çalıştırmak, npm paketi kurmak
kullanmak gibi her şeyi yapabilirler… altını üstüne getirebilir, sınırları yok."*

M847 gave agents the *mechanics* (bundles ship scripts; shell + code_exec run
them). M848 gives them the *self-awareness*: a capability briefing in the run's
environment preamble so the model knows it may build apps, run Python/JS, install
and run CLIs and npm/pip packages — whatever it takes — with no artificial limit.

## What shipped

- **`capabilityBriefing(tools)`** in `kernel/runtime/runtime.go`, appended to the
  per-run environment preamble (`injectEnvironment`, right after the tool list).
  A "## What you can do — act without artificial limits" section that:
  - states plainly the agent is a full agent on a real machine, default-allow, and
    should marshal whatever it takes to finish the task;
  - lists the concrete powers **tuned to the tools actually present** this run:
    code_exec → write/run Python · Node/JS · Deno; shell → install & run CLIs,
    npm/pip/cargo/go packages, build systems, background services ("if a command
    is missing, install it, then use it"); file → build as many files/apps as
    needed; tool_forge → forge a durable tool; skill → capture it as a reusable
    bundled skill;
  - stays honest about the only real rails — explicit denials, spend budgets, and
    the network/secret guards (SSRF off, secrets redacted) — and tells the agent
    to default to action otherwise.
  - returns "" when the run has none of shell/code_exec/file (nothing to promise).

## Surface

- `kernel/runtime/runtime.go` — `capabilityBriefing`; `injectEnvironment` calls it.
- `kernel/runtime/environment_internal_test.go` — `TestCapabilityBriefing_TunedToTools`
  (full set → all lines + emphatic framing; no code_exec/tool_forge → those lines
  absent; no build/run tools → empty).

## Verification

- **Gate:** `go build ./...`, `go vet`, `staticcheck`, linux cross-build clean;
  `kernel/runtime` tests green. No new env; go.mod unchanged; no frontend change.
- **Unit:** the briefing is present and tool-tuned; absent powers are never
  promised; empty when nothing can build/run.

## Notes
- This is the system-prompt expression of the owner's standing **default-allow**
  law: the briefing is the agent-facing statement of "every capability on unless
  opted out". It is descriptive of the real posture, not a new grant — the actual
  capability ladder (Edict) is unchanged.
