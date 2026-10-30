# W5.5d — the adapter's wiring is one door

## What the slice is

The **forty-sixth** slice of the port programme, and the **fourth** slice of
its **next phase**: the **adapter's** handle.

The phase so far: W5.5a made the adapter's handle **unnamed**, W5.5b moved the
**holding** to the **composition root**, W5.5c closed the **door** (the
constructors take a scope, by type). What was left open was the **wiring
around** that door: the adapter still carried a set of `SetX` back-wiring
methods and a late-binding phase, and the target architecture's P8 says
plainly:

> Wiring is declarative. Modules declare Requires/Provides; the composition
> root resolves them. **No `SetX()` back-wiring, no `Bind(LateDeps)` phases, no
> func-pointer cycle breakers.**

This slice takes that sentence literally.

## What the slice found

The adapter's surface was a back-wiring yard:

```go
// kernel/controlplane/server.go (before)
func (s *Server) SetHTTPBindings(b []HTTPBinding) { s.httpBindings = b }
func (s *Server) SetChannels(c []ChannelInfo) { s.channels = c }
func (s *Server) SetChannelSender(send ChannelSender) { s.channelSend = send }
func (s *Server) SetChatGPTSync(fn ChatGPTSyncFunc) { s.chatgptSync = fn }
func (s *Server) SetCredChain(desc string) { s.credChain = desc }
func (s *Server) SetBoard(st boardapi.Store, notify func(m boardapi.Message, corr string)) { … }
func (s *Server) SetToolbox(host toolboxapi.Host) { s.toolboxHost = host }
func (s *Server) SetRepairSource(src agentapi.RepairSource) { s.repairSource = src }
func (s *Server) SetUpdateService(svc updateapi.Backend) { … }
func (s *Server) SetDiskFree(fn DiskFreeFunc) { s.diskFree = fn }
func (s *Server) SetCancelOnDisconnect(on bool) { s.cancelOnDisconnect = on }
func (s *Server) SetConfigEnvPinned(pinned map[string]bool) { s.configEnvPinned = pinned }
// …and three more, in three other files
```

**16** exported setters, spread over **5** files, and a second door next to
them: the late-binding phase.

```go
// kernel/controlplane/deps.go (before)
type LateDeps struct {
	Pulse        PulseController
	Observers    PulseObservers
	ChannelSend  ChannelSender
	StandingFire func(id string) bool
}

func (s *Server) Bind(d LateDeps) { … }
```

The daemon called it at three points of readiness:

```go
// cmd/agezt/main.go (before)
srv.Bind(controlplane.LateDeps{Pulse: eng, Observers: pulseObserverAdmin{…}})
srv.Bind(controlplane.LateDeps{ChannelSend: channelSend})
srv.Bind(controlplane.LateDeps{StandingFire: fireStandingNow})
```

A `SetX` on the adapter is a door the root can walk through at any time, and
the adapter cannot know what state it is in. The `Bind` phase is the same door
with a schedule attached: the adapter's wiring was a sequence of moments, and
the daemon had to know them.

The measurement that moved:

| | before | after |
|---|---|---|
| the adapter's exported `SetX` methods | **16** | **0** |
| the adapter's binding phases | **1** | **0** |
| the daemon's `Bind` call sites | **3** | **0** |
| the late artifacts that are PROVIDERS | **0** | **4** |
| control-plane production files that name the runtime | **0** | **0** |
| application-owned verbs the guard holds | **170** | **170** |

## What changed

### One door: the constructor and the deps

Every dependency the daemon has when it builds the adapter now travels in
`Deps`, and the constructor applies it — once, at the door:

```go
// kernel/controlplane/deps.go
func NewServerWithDeps(scope binding.Scope, d Deps) *Server {
	s := NewServer(scope)
	s.configEnvPinned = d.ConfigEnvPinned
	s.boardStore = d.Board
	s.boardNotify = d.BoardNotify
	s.toolboxHost = d.Toolbox
	s.updateSvc = d.UpdateSvc
	s.repairSource = d.RepairSource
	s.diskFree = d.DiskFree
	s.tenants = d.Tenants
	s.cancelOnDisconnect = d.CancelOnDisconnect
	s.httpBindings = d.HTTPBindings
	s.credChain = d.CredChain
	s.channels = d.Channels
	s.chatgptSync = d.ChatGPTSync
	s.pulseProvider = d.Pulse
	s.observersProvider = d.Observers
	s.channelSendProvider = d.ChannelSend
	s.standingFireProvider = d.StandingFire
	return s
}
```

### The late artifacts are ASKED for, not pushed

The pulse engine, the live channels and the standing-order runner only exist
**after** the adapter is up. They are not pushed into the adapter later; the
root hands over a way to **ask**, and the answer is nil until the artifact is
there:

```go
	// Pulse provides the resident proactive engine. The provider answers nil
	// until the engine is up — the daemon's own readiness, not the adapter's
	// business.
	Pulse func() PulseController
```

The adapter asks when it needs one, and the root decides when the answer
changes:

```go
// kernel/controlplane/deps.go
func (s *Server) pulseController() PulseController {
	if s.pulseProvider == nil {
		return nil
	}
	return s.pulseProvider()
}
```

The daemon publishes on its own goroutine, so the holder is synchronized
(`cmd/agezt/late_wiring.go`):

```go
type lateHolder[T any] struct {
	mu sync.RWMutex
	v  T
}

func (h *lateHolder[T]) get() T { … }
func (h *lateHolder[T]) set(v T) { … }
```

and the root hands the adapter the way to ask:

```go
// cmd/agezt/main.go
srv := controlplane.NewServerWithDeps(binding.ScopeFromKernel(k, baseDir), controlplane.Deps{
	…
	Pulse:        pulseEngine.get,
	Observers:    pulseWatch.get,
	ChannelSend:  liveSender.get,
	StandingFire: standingRunner.get,
})
```

The **window** is unchanged: before the engine is up, the adapter sees nil and
the pulse handlers report "pulse is disabled" — the same answer the late
phase gave. What is gone is the phase.

### The tests had to say what they mean

The adapter has no setter, so a test that needs a dependency hands it over
**when the adapter is built**. That is the whole refactor of the test suite:
the tenant registry, the board, the pulse engine, the channel sender, the
standing fire — each is now a **construction** dependency, and the tests that
observe the "not wired" state observe it on a **second** adapter (the wiring
is one door: an adapter is built one way or the other).

## The rule

Two rules hold this slice, and both are the reflection of the adapter's own
API — they do not ask a helper what the adapter should be:

- **the adapter's wiring is one door** — the adapter exports no `SetX` method
  and has no `Bind` phase. The rule reads the adapter's exported methods and
  says what they are not.
- **the late artifacts are asked for, not pushed** — the four late deps are
  PROVIDERS: a function that takes nothing and answers the artifact (or nil,
  until the root publishes it). The rule reads the deps' field types.

A third rule came with the slice and holds the **nil window**: the provider's
answer is nil until the root publishes the artifact, and the adapter asks
**every time** (it does not cache).

## The tests

**5 permanent tests** (4 in the adapter, 1 in the daemon):

In the adapter (`kernel/controlplane`):

- `TestTheAdaptersWiringIsOneDoor` — the adapter exports no `SetX` and has no
  `Bind`.
- `TestTheLateArtifactsAreAskedForNotPushed` — the four late deps are
  providers.
- `TestTheLateArtifactIsNilUntilTheRootPublishesIt` — the nil window.
- `TestTheProviderIsAskedEveryTime` — the adapter asks, it does not cache.

In the daemon (`cmd/agezt`):

- `TestTheLateHolderAnswersNilUntilTheRootPublishesIt` — the holder answers the
  zero value until the root publishes, and the zero value again after.

Three **port** tests changed their rule with the slice: the board's, the
toolbox's and the update engine's port used to assert that the adapter HAS a
setter for the port; they now assert that the port arrives through `Deps` and
that the setter is **gone**.

## The mutations

**17 mutations, 17 caught**, in two independent passes with **byte-identical**
logs (SHA-256 `07C3225A85B605B9D9C9AC9404E0AAE73FD3EE4B1690DED91E63CED1AB1412D9`):

- **M01** the adapter exports a setter again — the **door rule** catches it.
- **M02** the adapter has a `Bind` phase again — the **door rule** catches it.
- **M03** a late artifact is a value, not a provider — the **provider rule**
  catches it.
- **M04** the adapter never asks the provider — the **ask rule** catches it.
- **M05** the channel sender ignores the provider.
- **M06** … **M17** — each dependency of the constructor stops being applied,
  one at a time: the pulse provider, the standing fire, the board, the repair
  source, the toolbox, the tenant registry, the disk probe, the ChatGPT sync,
  the credential chain, the cancel-on-disconnect, the channels and the update
  port. Each one is caught by the test that lives with it.

## What is left

The adapter's wiring is one door: the constructor, the deps, and the four
providers the root publishes into.

What is left is the **root's** own shape. The daemon still opens the kernel
and fills the scope in one place (`binding.ScopeFromKernel`), and the
families' ports are still filled one family at a time inside that fill — the
scope could be built by the families rather than by the bridge. And the
`kernel/modules/agents/selfrepair` → `kernel/runtime` edge is still in the
allowlist: the self-repair module's own runtime accessors are the parallel
session's work, and the edge stays until it lands.
