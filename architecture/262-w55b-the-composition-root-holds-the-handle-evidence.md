# W5.5b — the composition root holds the handle

## What the slice is

The **forty-fourth** slice of the port programme, and the **second** slice of
its **next phase**: the **adapter's** handle.

The first slice of the phase (W5.5a) made the adapter's handle **unnamed**:
the `Server` held it as `any`, the `DispatchCtx` carried it as `any`, the
request's app host carried it as `any`. The adapter could not **name** the
runtime — but it still **held** the daemon's kernel, and every one of the
**forty-five** binding files filled a family's ports from it on the spot.

This slice moves the holding to the **composition root**. The daemon is the
root: it holds the kernel, it builds the **scope**, and what it hands the
adapter is the scope — the families' capabilities, already filled — and never
the kernel itself.

## What the slice found

The adapter held the handle in **three** places:

```go
type Server struct {
	k             any
```

```go
type DispatchCtx struct {
	K             any
```

```go
type appHost struct {
	kernel      any
```

and **sixty-eight** binding call sites across **forty-five** files filled a
family's ports from it, one family at a time:

```go
func (s *Server) toolsPorts() apptools.Ports { return binding.Fill(s.k, apptools.PortsFromKernel) }
```

Every one of those calls is the adapter deciding *when* a capability is
filled. The composition root is the place that decides that: it holds the
kernel, it knows which daemon the adapter serves, and it can hand over a
finished scope instead of a promise to fill one later.

The measurement that moved:

| | before | after |
|---|---|---|
| the adapter's types that hold a handle | **3** | **0** |
| the adapter's binding call sites that fill a family's ports | **68** | **0** |
| control-plane production files that name the runtime | **0** | **0** |
| application-owned verbs the guard holds | **170** | **170** |
| the families the scope carries | — | **40** |

## What changed

### The scope is a bundle of ports, not a handle

The application layer grew the **scope** — one field per family, filled from
one kernel:

```go
// kernel/app/binding/ports.go
type Scope struct {
	ApprovalPorts  appapprovals.Ports
	ArtifactPorts  appartifacts.Ports
	…
	ToolsPorts     apptools.Ports
	WorldPorts     appworld.Ports

	// BaseDir is the scope's own directory: the primary daemon's, or the
	// tenant's.
	BaseDir string

	// filled records that this scope was filled from the daemon's own kernel.
	filled bool
}
```

`ScopeFromKernel(k, baseDir)` fills it; `ScopeFromHandle(h, baseDir)` refuses
a handle that is not the daemon's own; `RefusingScope()` is what an adapter
that was built without a scope asks for, so that a family is **asked** rather
than called into nothing.

### The root fills the scope

```go
// cmd/agezt/main.go
srv := controlplane.NewServerWithDeps(binding.ScopeFromKernel(k, baseDir), baseDir, controlplane.Deps{…})
```

The daemon holds the kernel; the adapter holds the scope.

### The adapter holds the scope

```go
// kernel/controlplane/server.go
type Server struct {
	// scope is the daemon's SCOPE: every family's ports, filled by the
	// composition root. The server holds no handle.
	scope   binding.Scope
	baseDir string
```

The `DispatchCtx` and the request's app host carry the scope too, and the
adapter's forty-five binding files became one-line reads:

```go
func (s *Server) toolsPorts() apptools.Ports { return s.scoped().ToolsPorts }
```

### The tenant's scope is the tenant's

The tenant path used to acquire the tenant's kernel and hand the handle to
the families. The **application** now opens the tenant's kernel and fills the
tenant's **scope**:

```go
// kernel/app/binding/ports.go
func ScopeForTenant(reg TenantAcquirer, id string, now time.Time, baseDir string) (Scope, error)
```

The tenant's scope carries the **tenant's own** directory, so the families
that keep state on disk read the tenant's state — not the primary's. The
adapter asks for a scope and gets one; the tenant's kernel never reaches it.

### The names do not carry the verbs

The first draft of the scope named its fields after the families
(`Council`, `Journal`, `Tools`…), and the **guard caught it**: the adapter's
own verb rule ("the adapter calls no application-owned verb on the kernel")
reads `scope.Council` as a call to the kernel's `Council` verb — and it is
right to be suspicious, because the rule cannot tell a port field from a
verb call. The fields were renamed to carry their **port** (`CouncilPorts`,
`JournalPorts`, `ToolsPorts`…), and two of them (`ReaperPorts`,
`WorkflowPorts`) collided with the kernel's **own** verbs of the same name,
so they became `ReaperScanPorts` and `WorkflowEnginePorts`.

The rule did not have to be weakened for the new shape — it held, and it
caught the new code ✗ → the new code was renamed ✓.

## The rule

Two ratchets hold this slice:

- **the adapter holds no handle** — `Server`, `DispatchCtx` and `appHost` have
  no untyped field, so there is nowhere for a kernel to be parked. The other
  half is the ratchet that already existed: the control plane names the
  runtime in **0** production files, so a handle cannot be held **by type**
  either.
- **the scope is filled or it refuses** — `Scope.Filled()` is the difference
  between a scope that reaches the daemon and one that asks the families to
  refuse.

The application's own ratchet is unchanged: the application layer names the
handle in **0** files outside a **port file** — and `kernel/app/binding/ports.go`
is a port file ✓.

## The tests

**11 permanent tests**:

In the application layer (`kernel/app/binding`):

- `TestTheScopeIsFilledFromTheDaemonsOwnKernel` — the scope carries the
  families' ports and its own directory.
- `TestTheScopeRefusesAHandleThatIsNotTheDaemons` — a foreign handle fills
  nothing, and the families that take a handle still refuse.
- `TestTheRefusingScopeRefusesEveryFamilyThatTakesAHandle` — the refusing
  scope is a refusal, not an absence.
- `TestResolveKeepsAScopeAndFillsAHandle` — a scope stays the scope it is; a
  handle is filled into one; anything else refuses.

In the adapter (`kernel/controlplane`):

- `TestTheAdapterHoldsNoHandle` — **the rule**: the adapter's own types have
  no untyped field.
- `TestTheServerReadsTheScopeItWasGiven` — the scope the root built is the
  scope the adapter reads.
- `TestTheServerWithoutAScopeRefuses` — a server built by hand asks the
  refusing scope.
- `TestTheTenantsScopeIsTheTenantsOwn` — the tenant's scope is the tenant's
  directory, not the primary's.
- `TestThePrimaryScopeIsThePrimarysOwn` — the primary's scope is the
  primary's.
- `TestTheScopeForATenantWithoutARegistryRefuses` — the daemon's own words:
  "multi-tenancy is disabled (no tenant registry configured)".
- `TestTheRequestCarriesTheScopeAndNotTheHandle` — the request carries the
  scope.

## The mutations

**17 mutations, 17 caught**, in two independent passes with **byte-identical**
logs (SHA-256 `A272DBBA43B9073D31D4A4B268E3A42383C055735AA788DA396A352FD73C2908`):

- **M01** the scope has no journal.
- **M02** the scope has no tools.
- **M03** the scope has no council.
- **M04** the refusing scope loses the plan refusal.
- **M05** the refusing scope loses the files refusal.
- **M06** the refusing scope loses the steer refusal.
- **M07** a foreign handle fills an empty scope.
- **M08** resolve refuses a scope it was given.
- **M09** the tenant's scope takes the primary's directory.
- **M10** the tenant's scope refuses.
- **M11** the scope claims it was never filled.
- **M12** the server always refuses.
- **M13** the request reads another scope.
- **M14** the primary's scope refuses.
- **M15** the scope has no bus.
- **M16** the adapter names the runtime again — the **ratchet** catches it.
- **M17** a tenant's typed operation is routed to the primary.

### The rewrites

The first pass caught **6** and left **10** open — **7 BUILDERR** and **3**
survivors — and every one of them was the mutation's own fault or the test's
own blind spot, not a defect in the slice:

- **four** left a binding unused (`Plan`, `Files`, `Steer` and the resolved
  scope) and did not compile — the mutation's fault;
- **one** left the tenant's kernel unused — the mutation's fault;
- **one** replaced a whole line with half of it — the mutation's fault;
- **one** read the wrong context key and did not compile — the mutation's
  fault;
- **one** survived because the test asked whether the council's **store
  function** was there, not whether it **answered** — a test's blind spot,
  and the test now asks the store ✓;
- **two** survived because the mutation hit the wrong occurrence of its
  anchor, or the wrong path: the tenant routing happens in **two** places —
  the native dispatch and the typed operation router — and a mutation in one
  of them does not touch the other ✗ → the mutation was aimed at the path
  the test actually walks ✓.

All **ten** were rewritten. Two of them walked a path that nothing reads: the
native dispatch's audit scope — `dc.Scope` is read by the native audit, and
**every** tenant-routed command today is **AppOwned** (the app dispatcher
audits it with the request's own scope ✓), so the native audit never runs
and the boundary's tenant half is **mechanism, not behaviour**. The mutation
was aimed at a live line instead — the scope's **bus**, which every audit
publishes through ✗ → it is caught now ✓, and the finding stayed in the
code: the boundary's comment says what its tenant half is for, and what it
is not for yet.

## What is left

The adapter holds no handle and the root fills the scope. What remains is
the root's own shape: the daemon still **opens** the kernel and still hands
it to the application in one place (`binding.ScopeFromKernel`), and the
families' ports are still filled one family at a time inside the scope's
fill. The next slice can move the fill itself — the scope could be built by
the families rather than by the bridge — but the **door is closed**: there is
no handle left in the adapter to walk through.
