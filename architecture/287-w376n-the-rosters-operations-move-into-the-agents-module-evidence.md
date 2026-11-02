# W3.76n — the roster's operations move into the agents module

**Slice:** W3.76n (the twenty-second of the runtime's dissolution)
**Base:** `fa0b9a56` (the W3.76m merge)
**Date:** 2026-11-02

---

## 1. The family

The **roster** is the agents module's — the target architecture's module
catalogue says it in one line (`kernel/modules/agents/roster`, with
`Create/Edit/Remove/Wake`, `Repair` and `Overseer.*` as its operations). Its
**data** was already there: the store, the normalization, the validation and the
row live in `kernel/modules/agents/roster`, and the daemon holds the
**contract's port** over the file it opened (W3.56).

Its **operations** were the daemon's. `kernel/runtime/accessors_roster.go`
(163 lines) held the store, applied the write, published the event and knew
the two rules that make a roster a roster:

- what a roster change **says** — `roster.created`, `roster.updated`
  (`paused` / `resumed` / `retired` / `revived` / `edited`), `roster.removed`;
- that a **shipped guardian** (`System`) may be paused or retired but never
  removed (M961).

That is the split W3.71 and W3.72 named for the MCP registry and the script
forge — **a family splits along the line between the port and the operations**
— and the line here cut **both ways**:

| the family | the port | the operations |
|---|---|---|
| the reads (`RosterGet`, `RosterList`) | the host's nil-safe view over the contract's port (W3.58) | — |
| the writes (`AddProfile`, `UpdateProfile`, `RemoveProfile`, `SetProfileEnabled`, `SetProfileRetired`) | — | the **module's** |

The reads stayed with the host: a read over a port is the host's face, and the
store port is the host's file. The writes moved down: the module that keeps the
rows also keeps what a change to a row **means**.

## 2. What moved

The module grew a service (`kernel/modules/agents/roster/service.go`, 167
lines) over the contract's port, and the daemon's five verbs became forwards:

```go
func (k *Kernel) AddProfile(p agentapi.Profile) (agentapi.Profile, error) {
	svc := k.rosterService()
	if svc == nil {
		return agentapi.Profile{}, agentapi.ErrNoRoster
	}
	return svc.Add(p)
}
```

The module's face is `kernel/modules/agents/api/roster.go` (84 lines): the
`RosterService` (the five writes), the `RosterDependencies` (the store and the
bus to publish on) and the `RosterServiceBuilder`.

**The refusal moved to the contract.** The daemon's `errNoRoster` was a local
`errors.New`; the module cannot answer with a daemon-local sentinel, so the
refusal is now `agentapi.ErrNoRoster` — the same message, one home.

**A cycle was broken honestly.** The module's api package used to name the
module's implementation (`roster.Profile` in the reaper's ports). The service
lives in that implementation, so the api package may no longer name it: the
reaper's port now speaks the **contract's** row (`agentapi.Profile`), which is
what `roster.Profile` already was — an alias. One row, one spelling, and the
api package is free of the implementation it used to pull in.

## 3. The wiring — the builder, not a setter

The daemon may not name the module's implementation (archcheck's principle P5:
the daemon is the runs module, and a module may import another module's **api**
package and nothing else). So the composition root hands over the **BUILDER**
and the kernel calls it while opening itself — the same shape the MCP registry
(W3.71), the script forge (W3.72) and the skill curator already use:

```go
// runtime.Config
RosterService agentsapi.RosterServiceBuilder

// compose.go
var rosterSvc agentsapi.RosterService
if cfg.Roster != nil && cfg.RosterService != nil {
	rosterSvc = cfg.RosterService(agentsapi.RosterDependencies{
		Store:   cfg.Roster,
		Publish: rosterPublisher{b: kbus}.Publish,
	})
}
```

`cmd/agezt` sets the builder (`cfg.RosterService = roster.NewService`) and the
**test hosts** do the same: the twelve `withRoster` helpers, the control
plane's own fixture and the two fixtures that open a roster directly. A host
that hands over no builder has no roster lifecycle, and the writes refuse with
the contract's row — the same answer the daemon gave when it held no roster.

## 4. The measurement

| measure | before | after |
|---|---:|---:|
| the daemon's public verbs | 203 | **203** |
| the daemon's production lines (non-blank) | 6,113 | **6,156** |
| the daemon's production files | 54 | **55** |
| the agents module's production lines (non-blank) | 4,035 | **4,291** |
| the agents module's production files | 30 | **32** |
| the daemon's roster accessor file | 163 | **121** |
| the contract's production lines (non-blank) | 10,406 | **10,413** |
| the app layer / the control plane (production files) | — | **+0 / +0** |

**The daemon grew, and that is the honest result of this slice.** The roster's
*body* shrank by 42 lines (`accessors_roster.go`, 163 → 121) and the module
grew by 256. But the host's *wiring* — the builder field, the publisher
adapter, the private accessor, the composition root's call — costs 43 lines,
and the host is where the wiring lives. The daemon's public surface did not
move: **203 verbs, the same 203**.

The three kinds of line in this slice, then:

- the **decision** (what a roster change says, that a guardian is protected)
  → the module;
- the **wiring** (who builds the service, over which store, publishing on which
  bus) → the host, because the host owns the bus and the file;
- the **name** (`AddProfile` and its four siblings) → the host, because every
  caller above the daemon still calls them.

## 5. What the daemon still keeps — and why

- `Roster`, `RosterGet`, `RosterList` — the **reads**: the host's nil-safe view
  over the contract's port (the shape W3.58 established).
- `AgentImpact` — it answers from the **standing orders**, another domain's
  rows. A module may not name another module's data, so the projection stays
  with the host. It is the same "vocabulary vs. state" line the W3.76l
  evidence drew: the roster module knows the roster; the host knows what
  stands behind it.
- The five write verbs — the **names**. They forward; they no longer decide.

## 6. The ratchets — and the probe

`kernel/runtime/roster_service_ratchet_test.go` (package-scoped) pins the move
in both directions: the daemon's production files must not hold the roster's
event kinds, the guardian rule or a direct store write — and the daemon must
still carry all nine roster names.

**The probe.** Putting the lifecycle back into the daemon made the ratchet
fail:

```
--- FAIL: TestTheDaemonDoesNotSpellOutTheRosterLifecycle (0.01s)
    roster_service_ratchet_test.go:67: zz_probe_roster_tmp.go holds "event.KindRosterCreated" — the roster's lifecycle is the agents module's (W3.76n); the daemon keeps the names and forwards
```

The probe file was removed and the tree scanned for leftovers (none).

**A guard changed honestly.** The control plane's guard reads the daemon's
roster accessor file to learn which verbs the application layer owns — and it
reads **every** `func (k *Kernel) …` in it, private ones included. The private
accessor therefore moved out of `accessors_roster.go` into
`kernel/runtime/roster_service.go`, beside the module's face: that file holds
the roster's **public** verbs, and a private accessor is not a verb the
application layer guards.

## 7. Verification

- **the compiler**: `go build ./...` and `go vet ./...` clean;
- **the module's own tests** (`kernel/modules/agents/roster/service_test.go`,
  9 tests): the five writes, the four event shapes, the guardian refusal, the
  unknown ref, the store-less refusal and the quiet bus;
- **the daemon's tests** (`kernel/runtime/roster_service_test.go`, 4 tests): the
  forward, the refusal without a service, the reads that need no service, and
  `AgentImpact`;
- **the ratchet probe** (§6);
- **the full test suite**: `go test ./...` — green;
- **the thirteen gates**: gofmt, build, vet, archcheck, deadcode, depscheck,
  sdkparity, structure-md, changelog-lint, docclaims, docsindex, staticcheck,
  test.

**No mutation run.** This slice redirects five calls and moves a body; there is
no rule whose reversal a mutation could expose. The evidence is the compiler,
the permanent tests, the ratchet and its probe, and the full suite.

## 8. What is left

The daemon holds **203** public verbs and **6,156** production lines. The
roster family is now split the way the target architecture splits it: the
module keeps the roster, the host keeps the port, the bus and the names.

The rest of the W3 exit is unchanged: the daemon's composition root
(`compose.go`), its `Kernel` (`runtime.go`), the accessors family and the
control plane (`kernel/controlplane`, 7,570 lines). The next roster slice is
the **application** layer's roster (`kernel/app/roster`, 6,665 production lines
in 30 files) — the typed `agent_*` operations the control plane drives — whose
services the module catalogue also names as the agents module's.

Evidence: `architecture/287-w376n-the-rosters-operations-move-into-the-agents-module-evidence.md`
