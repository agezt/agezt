# W5.4s — the tools family's ports, and the registry the other families ask

## What the slice is

The **sixth** family of the port programme, and the first one the family map
calls **verbs without libraries**: there is no store the daemon opens and
hands over, there is a *registry* the kernel holds and a *forge* and a set of
*MCP servers* beside it.

The tools family is the busiest reader in the adapter. Before this slice the
control plane named the registry **five** times — the tools inventory, the
roster's permission picture, the config's tool count, the schedule's tool
targets (twice) and the nodes' remote-run check — and it handed the **whole
handle** to the tools application three times, because the application's own
ports were shaped like the kernel's method set:

```go
apptools.NewInventory(host.kernel)                        // the handle as the registry reader
apptools.NewForgeLifecycle(host.kernel)                    // the handle as the forge writer
apptools.NewMCPLifecycle(host.kernel, host.kernel)         // the handle as the MCP writer
```

So the tools family's ports moved into the application:

```go
// kernel/app/tools/ports.go
type Ports struct {
	Registry     RegistryReader        // the registered tools, by registration name
	Forge        ForgeReader           // the script-tool forge; NIL when the daemon holds no forge
	ForgeWriter  ForgeWriter           // draft, edit, test, promote, quarantine, remove
	MCP          MCPRegistrationReader // the MCP registrations
	Attachment   MCPAttachmentReader   // the MCP attachment status
	MCPLifecycle MCPLifecycleWriter     // add, attach, detach, enable, remove
}

func PortsFromKernel(k *runtime.Kernel) Ports
```

The adapter keeps the bindings (`kernel/controlplane/tools_ports.go`):

```go
func toolsPortsFor(k *runtime.Kernel) apptools.Ports { return apptools.PortsFromKernel(k) }
func (s *Server) toolsPorts() apptools.Ports         { return toolsPortsFor(s.k) }
func requestToolsPorts(ctx context.Context) apptools.Ports { /* the routed kernel */ }
```

## What moved

| | Before | After |
|---|---|---|
| the tools inventory | `apptools.NewInventory(ctx.Value(appHostKey{}).(appHost).kernel)` — the **whole handle** | `apptools.NewInventory(requestToolsPorts(ctx).Registry)` |
| the forge catalogue | `apptools.NewForgeCatalog(ctx.Value(appHostKey{}).(appHost).kernel.ToolForge())` | `apptools.NewForgeCatalog(requestToolsPorts(ctx).Forge)` |
| the forge lifecycle | `apptools.NewForgeLifecycle(ctx.Value(appHostKey{}).(appHost).kernel)` — the **whole handle** | `apptools.NewForgeLifecycle(requestToolsPorts(ctx).ForgeWriter)` |
| the MCP catalogue | `apptools.NewMCPCatalog(host.kernel.MCPReader(), host.kernel)` | `apptools.NewMCPCatalog(ports.MCP, ports.Attachment)` |
| the MCP lifecycle | `apptools.NewMCPLifecycle(host.kernel, host.kernel)` — the **whole handle** | `apptools.NewMCPLifecycle(ports.MCPLifecycle, ports.Attachment)` |
| the roster's permission picture | `Tools: s.k.Tools` | `Tools: s.toolsPorts().Registry.Registered` |
| the config's tool count | `len(r.kernel.Tools())` | `r.tools.Count()` |
| the schedule's tool targets | `func(name string) bool { _, ok := s.k.Tools()[name]; return ok }` — **twice** | `s.toolsPorts().Has` |
| the nodes' remote-run check | `t, ok := s.k.Tools()["remote_run"]` | `s.toolsPorts().RemoteRun` |

**Nine** kernel verb sites left the adapter's production code, and the adapter
no longer hands the handle to a tools-application constructor at all.

Three things are worth naming:

- **The port has the application's name, not the kernel's.** The registry's
  read port says `Registered()`, not `Tools()`: the kernel's verb stays on the
  kernel's side of the port, and the adapter never names it. The inventory's
  own reader interface changed with it (`InventoryReader` → `RegistryReader`),
  because a port that is shaped like the kernel's method set is a handle in
  disguise.
- **The port's field names must not collide with kernel verbs.** The first
  draft called the MCP attachment port `MCPAttached` — the kernel's verb — and
  the guard caught the adapter the moment it did
  (`app_operations.go reaches the kernel's MCPAttached`). The field is
  `Attachment` now: the port's vocabulary is the application's.
- **The registry is read by four other families, and it is asked once.** The
  roster's permission picture, the config's tool count, the schedule's tool
  targets and the nodes' remote-run check all ask the tools family now. The
  adapter asks the tools family, not the kernel.

### The routed kernel stays the routed kernel

The target architecture says the tenant routing is a **pipeline** step and that
*handlers never pick a kernel* (`architecture/20-target-architecture.md`, §4).
So the tools services are built from the **request's** kernel — a tenant's
tools are the tenant's tools — while the daemon's own ports (`s.toolsPorts()`)
answer for the root kernel. The two bindings are separate on purpose, and the
test says so.

## The guard's fifth rule — the family

The tools family is found **by name**: every kernel method whose name mentions
a tool or an MCP server. The family is **29** verbs — the registry, the forge's
seven, the runner's three, the MCP registry's fourteen and `FoldRunTools` — and
a verb that lands in the kernel is guarded the day it lands.

The rule also sweeps the two threshold verbs — `MemoryDistillMinTools` and
`SkillForgeMinTools`. They are the memory and skill families' numbers rather
than the tools family's, but the adapter must not name them either, and until
those families are wired they have no rule of their own. When they are, the
verb is named twice, and **a verb guarded twice is still guarded**.

The guard now holds **67** application-owned verbs (the council's four, the
roster's nine, the standing family's eight, the schedule family's five, the
workflow family's twelve and the tools family's **29**).

## The guard's sixth rule — the HANDLE

The verb rule cannot see a handle. `apptools.NewForgeLifecycle(k)` names **no
verb at all** — the kernel is handed over as a value — so the guard would read
that line as clean, and the handle *is* the tools family's whole capability
surface. Handing it over is handing over the family.

So this family needed a second rule: **the tools application's services must
receive PORTS, never the handle they were filled from.** The rule reads the
file's own import to learn the alias the tools application is under, finds
every `New…(…)` call it makes, and refuses an argument that *ends* with the
handle's spelling. An argument that reaches **through** the handle for
something else — the journal, say — is not the handle and is left alone.

The rule is held by the rule: a matcher that could not see the handle would
let the family walk back in through the door the verb rule cannot see.

## The proof

### Permanent tests — 11 new

| Test | What it holds |
|---|---|
| `TestTheToolsPortsReachTheKernelTools` (`kernel/app/tools`) | the ports are **filled** (the registry, the MCP ports, the forge writer) and the tools the kernel holds are the tools the application reads, counts and answers about |
| `TestTheToolsPortsAnswerTheRemoteRunQuestion` (`kernel/app/tools`) | the remote-run question is the **tools family's**: the application knows which tool the node family means |
| `TestTheToolsPortsSurviveADaemonWithNoRegistry` (`kernel/app/tools`) | the **nil-port discipline**: with no registry the application sees no tools rather than calling into nothing |
| `TestTheToolsPortsRefuseADaemonWithNoForge` (`kernel/app/tools`) | a daemon with no forge holds a nil forge port, and the forge catalogue refuses with a plain answer |
| `TestTheToolsPortsReachTheKernelForge` (`kernel/app/tools`) | the forge ports reach the kernel's forge: the lifecycle port writes and the catalogue port reads the same script tool back |
| `TestTheToolsPortsReachTheKernelMCPServers` (`kernel/app/tools`) | the MCP ports reach the kernel's registry: the lifecycle port writes and the catalogue port reads the same server back |
| `TestTheAdapterBindsTheToolsPortsToItsOwnKernel` (`kernel/controlplane`) | the binding, end to end: the daemon's **tool inventory**, the **config's tool count**, the **roster's permission picture** and the **schedule's tool targets** all answer from the kernel's registry |
| `TestTheRequestToolsPortsFollowTheRoutedKernel` (`kernel/controlplane`) | the routed kernel: the request's ports follow the tenant's kernel, the daemon's own ports answer for the root |
| `TestTheGuardNamesEveryToolsVerbTheKernelHas` (`kernel/controlplane`) | the derivation — read from the kernel's own method set, independently of the helper the guard uses |
| `TestTheAdapterHandsTheToolsApplicationPortsAndNotTheHandle` (`kernel/controlplane`) | the handle rule — the tools application takes ports, not the handle |
| `TestTheHandleRuleSeesTheHandleAndLeavesTheReachedThingAlone` (`kernel/controlplane`) | the rule is held by the rule: it sees the handle, and it does not take the journal for it |

### The mutation run — 23 mutations, 23 caught

Two independent passes with **byte-identical** logs (SHA-256
`E6F662F1248B4B02C9D7FD1FB8EAF2697B42DAA3A8242C2266ABC4643F935887`; the elapsed times, ULIDs, epoch stamps, addresses, goroutine ids and
temporary paths are normalized, because they are not the mutation's
fingerprint).

| Mutation | What it breaks | Caught by |
|---|---|---|
| M01 | the registry port is nil — the application cannot read the tools | the ports test |
| M02 | the registry answers nothing — the inventory lists no tools | the ports test |
| M03 | the count answers zero | the ports test |
| M04 | the "is this tool registered" answer looks up the wrong name | the ports test |
| M05 | the remote-run question looks up the wrong name | the ports test |
| M06 | the forge port is nil — the application cannot read the forge | the ports test |
| M07 | the forge writer port is nil — the application cannot drive the forge | the ports test |
| M08 | the MCP port is nil — the application cannot read the servers | the ports test |
| M09 | the attachment port is nil | the ports test |
| M10 | the MCP lifecycle port is nil — the application cannot drive the servers | the ports test |
| M11 | the inventory reads nothing even when the port is filled | the ports test |
| M12 | the adapter's binding fills no ports at all | the binding test |
| M13 | the request's binding fills no ports — the tenant's tools are gone | the binding test |
| M14 | the adapter names the registry verb again | the **verb guard** |
| M15 | the node family's remote-run answer lies | the node registry test (W5.4a's) |
| M16 | the daemon's inventory gets no reader | the binding test |
| M17 | the forge lifecycle gets the **handle** | the **handle rule** |
| M18 | the MCP catalogue names the MCP verb again | the **verb guard** |
| M19 | the schedule's tool answer lies — every tool schedule is refused | the binding test |
| M20 | the config's tool count answers zero | the binding test |
| M21 | the derivation loses the MCP half — the family falls out of the guard | the guard test |
| M22 | the derivation loses its case — the same, spelled differently | the guard test |
| M23 | the handle rule goes blind | the rule's own test |

M14, M17 and M18 are the mutations that matter most, and none of them is a
behavioural regression: the wiring would work **exactly as well**, it would
simply be in the wrong layer. M14 and M18 name a kernel verb in the adapter;
M17 hands the handle to the application. No behaviour test can see those —
only the two rules can.

M15 is the one that was already covered: the node family's remote-run answer
was pinned by an earlier slice's test, and the port did not change what the
adapter asks for — only **who** answers.

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

The **ratchet does not move yet**: the control plane still imports the runtime
in **15** files.

The application's own **invoker** (`apptools.NewInvoker`) is untouched: it
binds the tool *pipeline*, not the kernel's verbs, and the pipeline is the
platform's business.

Six families are done — council, roster, standing, schedule, workflow, tools —
and the next one (the workboard and OKR surface) is the last of the family
map's **verbs without libraries**: the adapter asks the kernel a question and
the application decides what the answer means. That is the same shape as the
council's, and the council's slice is the template.

The two blockers the family map named for this family are now ports rather
than wirings: the tools family holds the handle **narrowed** (six interfaces
and the registry), and the application's own services are built from the
routed kernel rather than from the server's.
