# W5.4w — the config center family's ports, and the first step of the handle

## What the slice is

The **tenth** slice of the port programme and the **first** one of the
programme's last step: the handle itself.

W5.4n measured the whole remaining programme — *the adapter's kernel use is
almost entirely port filling* — and said the work was "one family at a time,
until the last family is gone and the handle goes with it". Eight families
are gone (council, roster, standing, schedule, workflow, tools, objectives,
workboard). This slice takes the **config center** — the first of the families
that hold a **store** — and it is the first slice in which the adapter stops
reaching for a kernel verb **and asks a family instead**.

## What the slice found

The adapter reached for the kernel's config center in **eleven** places across
**five** files:

| file | sites | what it reached for |
|---|---|---|
| `kernel/controlplane/app_operations.go` | 1 | the roster's permission picture |
| `kernel/controlplane/configcenter_read_ports.go` | 1 | the config center's own reads |
| `kernel/controlplane/configcenter_write_ports.go` | 1 | the config center's own writes |
| `kernel/controlplane/roster_cascade.go` | 2 | the roster's impact cascade |
| `kernel/controlplane/roster_teardown_state.go` | 6 | the roster's teardown |

Three of those are **port filling** — the adapter holding the center so it can
hand it to an application — and two are **families asking the kernel** for a
capability that is not theirs: the roster's teardown and the roster's
permission picture both went to the kernel for the config center's entries,
because the config center family had no ports to ask.

The adapter's handle-verb debt, measured across the whole control plane:

| | before | after |
|---|---|---|
| call sites on the handle | **115** | **104** |
| distinct kernel verbs the adapter names | **31** | **30** |

## What changed

The config center family's ports moved into the application
(`kernel/app/configcenter/ports.go`):

```go
type Ports struct {
	Reader  Reader                                // the center's reads
	Writer  Writer                                // the center's writes
	Entries func() ([]*core.ConfigEntry, bool)    // the entries, and whether a center exists
	Delete  func(key string) error                // delete an entry
	Set     func(entry *core.ConfigEntry) error   // write an entry back
}

func PortsFromKernel(k *runtime.Kernel) Ports { return PortsFromCenter(k.ConfigCenter()) }
```

and the adapter keeps the binding (`kernel/controlplane/configcenter_ports.go`)
— which does not name the runtime, like every binding since W5.4v:

```go
func (s *Server) configCenterPorts() appconfigcenter.Ports {
	return appconfigcenter.PortsFromKernel(s.k)
}
```

The **nil-port discipline** moved with it. The adapter spelled "a daemon with
no config center" three times in one file (the teardown's entries, its delete
and its set) and twice in another (the cascade's entries, the permissions'
entries) — five spellings that could disagree with each other. The
application spells it **once**, in `PortsFromCenter`:

```go
if center == nil {
	return Ports{
		Entries: func() ([]*core.ConfigEntry, bool) { return nil, false },
		Delete:  func(string) error { return nil },
		Set:     func(*core.ConfigEntry) error { return nil },
	}
}
```

The rows are the config center's **own** rows (`core.ConfigEntry` is the
config center module's row, which the application layer may name), so the port
hands them over without a conversion — the same identity W5.4m found.

`PortsFromCenter` exists because the kernel always opens a config center, so
the empty center is not reachable through a handle. The **discipline** still
has to be there and it still has to be **provable**, and this is where it is.

## The rule

The config center's verbs are the application's, and the rule is **by name** —
a kernel verb that mentions the config center is the config center family's
business, and a verb that lands in the kernel is guarded the day it lands.
The guard now holds **93** application-owned verbs.

The name rule is the right one here because the family says its own name: the
kernel's verb is `ConfigCenter`. (The families whose verbs say nothing about
the family — the rollup, the proof — are still read from their files.)

## The tests

| test | holds |
|---|---|
| `TestTheConfigCenterPortsReachTheKernelEntries` | the ports reach the kernel's center |
| `TestTheConfigCenterPortsSurviveADaemonWithNoConfigCenter` | the empty center holds no entries and calls into nothing |
| `TestTheAdapterBindsTheConfigCenterPortsToItsOwnKernel` | the binding answers for the daemon's own kernel |
| `TestTheAdapterDeletesTheAgentsOwnConfigEntries` | the removal deletes the agent's own entries and prunes the shared ones |
| `TestTheGuardNamesEveryConfigCenterVerbTheKernelHas` | the guard names every config center verb the kernel has |

## The mutations

**14** mutations, **14** caught, in two independent passes with **byte-identical**
logs (SHA-256 `19EFE4F1D074D4A1BC93EB7273249587C31BFB393CB29E84138CA94D8CFE445E`).

**M12 survived the first calibration**, and it is worth reading twice: it
breaks the teardown's config **delete** — the agent's own entries stop being
deleted — and nothing caught it. The teardown's config path had **no test at
all**: the cascade test proved the entries were *counted*, and the permissions
test proved the picture was *empty*, but no test ever watched an entry
disappear. The slice wrote that test
(`TestTheAdapterDeletesTheAgentsOwnConfigEntries`), and the mutation is caught
now.

That is the second time this programme a mutation has found a **missing test**
rather than a missing rule (W5.4o's derivation found the roster's own test
checking itself), and it is the reason the harness runs the calibration
before the two clean passes: a mutation that survives the first run is a hole,
and a hole found late is a hole the record has already blessed.

M01 and M08 are the two that matter most, and neither is a behavioural
regression: the ports bind to nothing, and the wiring still works **exactly
as well** — it simply answers for no config center at all. Only the
application's own tests can see that.

### The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go build ./...` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | OK — **2 allowlisted import violations and 0 allowlisted call sites remain** (ratchet) |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean |
| `go run ./tools/changelog-lint` | OK |
| `go run ./tools/docclaimscheck -base origin/main` | OK |
| `go run ./tools/docsindex` | OK |
| `staticcheck ./...` | clean |
| `go test ./... -count=1 -timeout=40m` | green |

## What the slice does not do

The handle stays. The adapter still holds it, and it still calls **104**
kernel verbs on it — the journal (**27**), the event bus (**19**), the skill
forge (**7**), the providers (**7**), the base directory (**6**), the memory
(**5**), the policy engine (**4**) and the rest.

Those families are the ones the roadmap calls **libraries**: they own a store,
and the store is the host's file. The config center is the first of them to
come over, and it came over in the shape the others will take — the
application fills its own ports, the adapter asks the family, and the
**families that consume it stop reaching for the kernel**.

No behaviour changed. The daemon's own config center is the one that was read
before and the one that is read now.
