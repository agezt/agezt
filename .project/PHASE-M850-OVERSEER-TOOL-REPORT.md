# PHASE M850 — Overseer Tool (the brain agent's intervene controls)

**Status:** shipped
**Milestone:** M850
**Theme:** Give a privileged "brain" agent the controls to supervise and
**intervene** on the rest of the system. Owner ask: *"bir tane agentlar üstü
brain agent lazım… denetleyen durduran modifiye eden"* (task #46) — an agent
above all agents that oversees, stops, and modifies.

This is the **teeth**: the same controls an operator has (`agt halt`,
`agt agent retire`, cancel a run) are now reachable by an agent through one
`overseer` tool, so supervision can be autonomous. It pairs with the M849
mailbox — the overseer triages the open help requests agents raise.

## What shipped

The `overseer` tool (`plugins/tools/overseertool`), bound to the live kernel
after open (mirroring the introspect tool). Ops:

- **Read / situational awareness:** `status` (halted?, active-run count, agent
  count, open-help count), `agents` (every agent with state enabled/paused/
  retired + model), `runs` (the correlation ids in flight right now), `help`
  (the open help requests to triage), `impact` (what standing orders fire an
  agent).
- **Intervene:** `cancel` (stop one run by correlation id), `halt` / `resume`
  (stop ALL runs and block new ones / clear it), `pause` / `unpause` (an agent),
  `retire` (to the graveyard, surfacing impact first) / `revive`.

Every action goes through the kernel's own methods, so each is journaled and
reversible exactly like its operator-driven equivalent (kernel.halt→resume,
roster.updated retired→revived).

- New kernel method `Kernel.ActiveRunIDs() []string` — the live correlation ids
  of in-flight runs (the cancel registry keys), sorted; distinct from the
  journal-derived run history. This is what `op=runs` lists and `op=cancel` acts
  on.
- New Edict capability `CapOversee` ("oversee"), allow-by-default per the
  default-allow law but with its **own** opt-out knob so an owner can disable
  autonomous oversight without touching delegation. `toolmap`: `overseer →
  CapOversee`.
- Daemon wiring: constructed in buildTools, `Bind(NewKernelSource(k, baseDir))`
  after open; the adapter reads the kernel + opens the board fresh per `op=help`.

## Surface

- `plugins/tools/overseertool/{overseer,tool,kernelsource}.go` + `overseer_test.go`.
- `kernel/runtime/runtime.go` — `ActiveRunIDs`.
- `kernel/edict/edict.go` — `CapOversee` + in `AllCapabilities`; `toolmap.go` map.
- `cmd/agezt/main.go` — construct + bind + startup line.

## Verification

- **Gate:** `go build`, `go vet`, `staticcheck`, linux cross-build clean;
  `overseertool` (8 tests) + `edict` green. No new env; go.mod unchanged.
- **Unit:** status counts; runs+cancel (and cancel-needs-run); halt/resume;
  agent pause/unpause/retire(+impact)/revive (each needs a target); kernel-error
  surfaced; help triage; unknown/missing op; unbound-safe.
- **Boot smoke (isolated home):** daemon starts clean and advertises
  `overseer tool : enabled`.
- **Live LLM end-to-end:** not possible in this sandbox — the run fell back to
  the offline-mock provider (no network for the real provider; mock ignores the
  instruction). The integration risk is low: the adapter is a thin pass-through
  to kernel methods that are themselves already live-verified (M846 retire/revive,
  M849 OpenHelp), and every op is unit-tested against a fake Source.

## Notes
- Default-allow preserved: `CapOversee` ships at LevelAllow; the powerful actions
  (halt, retire) are journaled + reversible, and the operator can opt out via
  Edict if desired.
- Follow-ups: a dedicated "brain" agent persona/standing-order that runs the
  overseer on a cadence, and a Web UI overseer panel. The data it needs is
  already exposed (runs, roster, `/api/board/help`).
