# W5.5c — the adapter's door closes

## What the slice is

The **forty-fifth** slice of the port programme, and the **third** slice of its
**next phase**: the **adapter's** handle.

The first slice of the phase (W5.5a) made the adapter's handle **unnamed**: the
`Server` held it as `any`. The second (W5.5b) moved the **holding** to the
**composition root**: the daemon builds the **scope** — every family's ports,
filled — and hands the scope to the adapter.

What was left open was the **door itself**. The adapter's constructors still
took an **untyped** value (`any`), the scope's own directory still wore the
kernel's verb's name (`BaseDir`), and the root could still hand the adapter a
handle and let it decide what to do with it.

This slice closes the door: the adapter's constructors take a **scope, by type**,
and nothing in the adapter's own API is untyped any more.

## What the slice found

The adapter's constructors were untyped, and a constructor that takes an `any`
is a constructor that can be handed the kernel:

```go
// kernel/controlplane/server_lifecycle.go (before)
func NewServer(k any, baseDir string) *Server {
	return &Server{k: k, baseDir: baseDir, shutdownCh: make(chan struct{})}
}

// kernel/controlplane/deps.go (before)
func NewServerWithDeps(k any, baseDir string, d Deps) *Server {
```

An `any` here is not a convenience: it is a **door**. Anything can walk through
it, and the compiler will not say which.

The scope's own directory wore the kernel's verb's name:

```go
// kernel/app/binding/ports.go (before)
	// BaseDir is the scope's own directory: the primary daemon's, or the
	// tenant's.
	BaseDir string
```

`BaseDir` is the **kernel's own verb** — `func (k *Kernel) BaseDir() string`. A
scope that carried the verb's name named a door it does not have: the scope's
directory is the **scope's**, and the daemon's root is the **daemon's**.

Two helpers of the previous slice had become unreachable the moment the door
closed: `binding.Resolve` and `binding.ScopeFromHandle` existed only to turn a
handle into a scope, and the **deadcode gate** said so ✗ → they are deleted ✓.

The measurement that moved:

| | before | after |
|---|---|---|
| the adapter's constructors that take an untyped value | **2** | **0** |
| the scope's fields named after a kernel verb | **1** | **0** |
| the call sites that build a server | — | **~160** |
| control-plane production files that name the runtime | **0** | **0** |
| application-owned verbs the guard holds | **170** | **170** |

## What changed

### The constructors take a scope, by type

```go
// kernel/controlplane/server_lifecycle.go
func NewServer(scope binding.Scope) *Server {
	return &Server{
		scope:      scope,
		baseDir:    scope.Directory,
		shutdownCh: make(chan struct{}),
	}
}

// kernel/controlplane/deps.go
func NewServerWithDeps(scope binding.Scope, d Deps) *Server {
```

The server's own directory **is** the scope's directory — one answer, so the
two cannot disagree.

### The root fills the scope and hands it over

```go
// cmd/agezt/main.go
srv := controlplane.NewServerWithDeps(binding.ScopeFromKernel(k, baseDir), controlplane.Deps{…})
```

Every call site — the daemon, the agent, the web UI, the tests — was rewritten
to build the scope at the **root** and pass it in: **~160** call sites across
**134** files.

### The scope's directory is the scope's

```go
// kernel/app/binding/ports.go
	// Directory is the scope's own directory: the primary daemon's, or the
	// tenant's.
	Directory string
```

The kernel's `BaseDir` is the **daemon's** root; the scope's `Directory` is the
scope's own — the primary's, or the tenant's.

### The refusal keeps its directory

```go
// kernel/app/binding/ports.go
func RefusingScope(baseDir string) Scope {
	return Scope{
		CouncilPorts: appcouncil.PortsFromHandle(nil, baseDir),
		…
		Directory: baseDir,
	}
}
```

A scope that refuses still knows where it is: the adapter's own directory comes
from the scope, and a scope that lost its directory would point the adapter at
nothing.

### The scope with no daemon is a refusal, not an absence

```go
// kernel/app/binding/ports.go
func ScopeFromKernel(k *runtime.Kernel, baseDir string) Scope {
	if k == nil {
		return RefusingScope(baseDir)
	}
	…
}
```

There is no way to fill a scope from an untyped handle any more, so "no
daemon" is a **nil kernel**, and the refusal is the same one the adapter asks
for.

## The rule

Two ratchets hold this slice, and one of them is **new**:

- **the adapter's constructors take a scope** — the first parameter is
  `binding.Scope`, **by type**, and no parameter of the adapter's own API is
  `any`. This is the reflection of the signature itself: the rule does not ask
  the helper, it reads the door.
- **the scope's fields never carry a kernel verb** — no field of the scope is
  named like a verb of the kernel (`BaseDir` was). The rule is about the
  **whole name**: a field that carries a family as a prefix (`MemoryPorts`)
  names the family's port, and the verb it resembles is not the field's name.
- **the control plane names the runtime in 0 production files** — unchanged
  since W5.5a, and the other half of the first rule: a handle can only be
  typed by naming the runtime.

The application's own ratchet is unchanged: the application layer names the
handle in **0** files outside a **port file** — and `kernel/app/binding/ports.go`
is a port file ✓.

## The tests

**5 permanent tests** (3 in the application layer, 2 in the adapter):

In the application layer (`kernel/app/binding`):

- `TestTheScopeRefusesWhenThereIsNoDaemon` — a scope with no daemon refuses:
  the families that take a handle refuse, and the scope is not filled.
- `TestTheRefusingScopeKeepsItsDirectory` — a refusing scope still knows where
  it is.
- `TestTheScopesFieldsAreNeverTheKernelsVerbs` — **the new rule**: no field of
  the scope is named like a verb of the kernel.

In the adapter (`kernel/controlplane`):

- `TestTheAdaptersConstructorsTakeAScopeAndNotAHandle` — **the rule**: the
  constructors take a scope as their first argument, by type, and nothing in
  the adapter's API is untyped.
- `TestTheServersDirectoryIsTheScopesDirectory` — the server's directory is
  the scope's directory.

## The mutations

**13 mutations, 13 caught**, in two independent passes with **byte-identical**
logs (SHA-256 `2572E403E605AC0911F484A963D75F8C917ADB03F5026AA7CE9E1315D362837E`):

- **M01** the server's constructor takes a handle — the **signature rule**
  catches it.
- **M02** the deps constructor takes a handle — the **signature rule** catches
  it.
- **M03** the server loses the scope's directory.
- **M04** the refusing scope loses its directory.
- **M05** the scope with no daemon is empty — an absence, not a refusal.
- **M06** the filled scope loses its own directory.
- **M07** the server without a scope does not refuse.
- **M08** the scope's field carries the kernel's verb — the **name rule**
  catches it.
- **M09** the adapter names the runtime again — the **ratchet** catches it.
- **M10** the tenant's scope takes the caller's directory.
- **M11** the request carries a scope that is not the host's.
- **M12** the refusal leaves a family empty.
- **M13** the scope claims it was never filled.

### The rewrites

The first pass caught **12** and left **1** open — a **BUILDERR**, and it was
the mutation's own fault: the mutation replaced the request's scope with an
empty one and left the `host` it had read **unused**, so it did not compile.
The mutation was rewritten to keep the host read and change the **scope**
instead of the **value** (`binding.Scope{Directory: host.scope.Directory}`) ✓.

## What is left

The adapter's door is closed: its constructors take a scope by type, the scope
names ports and never verbs, and the scope's own directory is the scope's.

What remains is the **root's** shape. The daemon still opens the kernel and
fills the scope in one place (`binding.ScopeFromKernel`), and the families'
ports are still filled one family at a time inside that fill. The next slice
can move the fill itself — the scope could be built by the families rather than
by the bridge — and the adapter's `Set*` / `Bind(LateDeps)` wiring is still
there to be rewired onto the scope.
