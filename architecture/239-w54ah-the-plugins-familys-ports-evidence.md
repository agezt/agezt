# W5.4ah — the plugins family, and the rule that must survive a name it has not seen

## What the slice is

The **twenty-first** slice of the port programme, and the **twelfth** one of its
last phase: the handle itself.

The plugin manifest is the external-plugin list the daemon was given at Open —
the rows the operator reads with `agt plugin list`. The adapter reached for the
kernel's manifest in **two** places across **two** files.

## What the slice found

| file | sites | whose manifest |
|---|---|---|
| `kernel/controlplane/plugin.go` | 1 | the routed one |
| `kernel/controlplane/config_handler.go` | 1 | the routed one |

| | before | after |
|---|---|---|
| call sites on the handle (the adapter) | **20** | **18** |
| distinct kernel verbs the adapter names | **15** | **14** |
| method values handed over | **9** | **9** |

The **application** layer held no plugin handle either: `kernel/app/plugins`
already took a reader of its own. What the adapter did was wrap the kernel in a
reader of its own — and that wrapper held the handle.

## What changed

The plugins family's ports moved into the application
(`kernel/app/plugins/ports.go`):

```go
type Ports struct {
	manifest func() []Registration
}

func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{manifest: k.Plugins}
}
```

and the adapter keeps the binding (`kernel/controlplane/plugins_ports.go`) —
which does not name the runtime, like every binding since W5.4v:

```go
func requestPluginPorts(ctx context.Context) appplugins.Ports {
	return appplugins.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)
}
```

There is **no daemon-level binding**, and the reason is the shape of the read
path: every surface that reads the manifest — the plugin list and the config
reader's plugin count — reads it on the **request's** path, so the routed
kernel is the one that answers. A binding nobody calls is dead code, and the
dead-code gate says so — the slice wrote one, the gate dropped it, and the
binding went with it.

### The ratchet moved

The adapter's plugin reader of its own was a **production file that named the
kernel runtime**, and deleting it took the control plane's runtime-naming count
from **fourteen** to **thirteen**. The ratchet compares in **both** directions,
so the recorded number and the documents that state it changed in this slice:

> `controlPlaneRuntimeNamingFiles = 13`

### The wrapper is gone

The adapter used to carry a reader of its own, and that reader held the kernel:

```go
type nativePluginReader struct{ kernel *runtime.Kernel }

func (r nativePluginReader) Plugins() []appplugins.Registration {
	return r.kernel.Plugins()
}
```

The port replaced it, and the application's own narrow view — the `Reader`
interface — was renamed to match the daemon's word for what it holds:

```go
type Reader interface{ Manifest() []Registration }
```

So the port **is** the reader: the adapter hands it over as it is, and there is
no wrapper between the application and the kernel's verb. The manifest is the
contract type itself — not a copy of it — so nothing is projected and nothing
can drift.

### The port's names are not the kernel's

The port answers `Manifest` — never `Plugins`. A port that carried the name of
the verb it stands for would let the adapter name the kernel's verb through the
port's door.
`TestThePluginPortNamesAreNotTheKernelVerbs` holds that by reflection over the
port's own method set and the kernel's.

## The rule

The plugins' verbs are the application's, and the rule is **by whole word** —
and the family answers to **two** names:

> a kernel verb whose name carries "Plugin" **or** "Plugins" as a **complete
> word** is the plugins family's business.

The two names are the same trap the artifacts family met: the camel-case
splitter reads `PluginCount` as **"Plugin" + "Count"**, while `Plugins` is
**ONE** word to it. A family that looked for a single name would guard one of
the two forms and leave the other unguarded.

But the rule is written **out**, not taken from the helper:

```go
for _, word := range camelWords(name) {
	if word == "Plugin" || word == "Plugins" {
		verbs = append(verbs, name)
		break
	}
}
```

The helper `kernelVerbsWord` **fails** when a name finds no verb at all — and
"Plugin" finds none today. A rule that cannot be asked about a name it has not
seen yet is a rule that breaks on the day the name arrives: the family would
have to be rewritten to accept the first verb that carries it.

The sweep takes in **one** verb today: `Plugins`.

The guard now holds **135** application-owned verbs.

## The tests

| test | holds |
|---|---|
| `TestThePluginPortsReachTheKernelManifest` | the port reports the daemon's own manifest |
| `TestThePluginPortsSurviveAHostWithNoKernel` | a host with no kernel reports no plugins |
| `TestThePluginPortNamesAreNotTheKernelVerbs` | no method of the port carries the name of a kernel verb |
| `TestTheAdapterBindsThePluginPortsToTheRoutedKernel` | the request's ports follow the **routed** kernel, and the daemon's own request answers for the daemon's own kernel |
| `TestTheAdapterBindsThePluginPortsToAHostWithNoManifest` | a daemon given no manifest hands over none |
| `TestTheConfigReaderCountsThePluginsThroughThePort` | the config reader counts the plugins through the port, not through the handle |
| `TestThePluginServiceReadsTheManifestThroughThePort` | the application reads the manifest through the port |
| `TestTheGuardNamesEveryPluginVerbTheKernelHas` | the guard names every plugin verb the kernel has |
| `TestThePluginFamilyClaimsNoOtherFamilysVerbs` | the plugin's list claims no other family's verb and does not claim the whole kernel |

## The mutations

**13** mutations, **13** caught, in two independent passes with **byte-identical**
logs (SHA-256 `819ADBE76A3BDE21434B417F8276E4FAEAFC1BEC4DF76F9C459B9EA328C70FA0`).

M05 puts the kernel back — the config reader reaches for the kernel's plugin
manifest again — and the **verb guard** catches it. It is not a behavioural
regression: the wiring would work **exactly as well**, it would simply be in
the wrong layer.

M01, M02 and M10 are the ones that bite: the port holds **no** manifest, the
manifest answers with **nothing** for every caller, and the application reads
**nothing**. M03 empties the binding, M04 makes it follow the **daemon's**
kernel — a tenant's plugin list would become the daemon's — M06 gives the port a
method that carries a kernel verb's own name, M07 takes the family out of the
guard, M08 drops the plural half of the rule, M09 hands the family **another**
family's verbs, M11 makes the binding name the runtime, M12 builds the service
from **nothing** — and the plugin list over the socket reports an **empty**
manifest — and M13 makes the config reader count **no** plugins.

Two of the thirteen were caught only after the slice learned something:

- M12 first ran against the **tenant-denial** test, which asserts that a tenant
  cannot read the daemon's manifest — and a denial is a denial whether the
  manifest is there or not. The mutation **survived**: the test was the wrong
  test. The mutation now runs against the test that asserts the manifest's
  **content**, and the empty manifest is caught.
- M13 did not exist until the slice wrote a test for the config reader's plugin
  count — a surface that had no test of its own, which is exactly the gap a
  mutation is supposed to find.

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

The handle stays, and the adapter still calls **18** kernel verbs on it and hands
over **9** method values: the run decorators, the approvals, the market, the
state, the world, the taste, the data lake, the seats, the reflect, the reaper,
the update drain and the rest.

The **config reader** still holds the kernel handle — for the daemon's default
model and its default identity prompt. Those are the daemon's own identity, and
they are the next family's business, not the plugins family's.

The daemon's **overview** (`kernel/app/system`) still holds the handle for
**thirteen** verbs across **eight** families — the journal's head, the schedule
engine, the approvals, the world, the forge, the memory count and the rest.
That surface is not one family but a **roll-up** of many, and it will have to
ask each of them rather than the kernel.
