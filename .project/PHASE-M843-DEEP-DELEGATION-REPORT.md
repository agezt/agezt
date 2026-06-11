# Phase M843 — deeper delegation by default (leader/worker trees)

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "agentlar alt
agentlar spawn edebilir, taskları birden çok parçaya bölerek kendileri leader
konumda durarak daha fazla agent çalıştırabilir."

## What shipped

Delegation nesting defaulted to **depth 1** — a lead could spawn workers, but
those workers could not delegate further, so no real leader/worker tree. Now:

- **`cmd/agezt`**: default `AGEZT_SUBAGENT_DEPTH` raised **1 → 3** — a lead can
  decompose a task, delegate the parts, and those sub-agents can delegate again.
- **Safety rail**: the code's own design (M629) notes depth>1 needs a tree-total
  cap (depth×fan-out alone can't bound a tree). So when the operator hasn't set
  `AGEZT_SUBAGENT_MAX_TOTAL` and depth>1, it now defaults to **48** total
  sub-agents per tree — far beyond any real task, but a hard stop against a
  fork-bomb. `depth==1` stays unbounded (unchanged). Both overridable by env.
- **`delegate` tool description**: now coaches the leader pattern — break a big
  task into parts, delegate each, and note that sub-agents can delegate further;
  prefer reusing an existing named roster `agent` over inventing an ad-hoc one
  (the owner's "reuse over create" guidance).

## Verification

- Build + existing delegation/subagent tests green (kernel default stays 1, so
  kernel tests are unaffected; only the daemon's product default changed).
- **Live** (isolated home, real agent): banner showed `delegation: depth≤3,
  fan-out unbounded, total ≤48`. A forced two-level delegation — lead → mid-lead
  → worker computing 17+25 — returned **42**, with `subagent.spawned` events at
  `depth:1` AND `depth:2`. Under the old depth=1 default the second delegation
  would have failed with "max sub-agent depth 1 reached".

## Gate

cmd/agezt + runtime tests green; vet/staticcheck unaffected; gofmt swept. go.mod
unchanged. Default-allow posture: deeper delegation on by default, bounded only by
the generous tree-total safety budget (the kind of budget rail the owner's law
keeps).

## Note

Reuse-over-create as an enforced roster dedup check (scan for a similar agent
before CREATING one) remains future work (task #40 / #53 reaper); this milestone
delivers the depth/leader half plus the description nudge.
