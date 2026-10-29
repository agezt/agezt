# W5.4am — the persona family, and the identity the config reader no longer holds

## What the slice is

The **twenty-sixth** slice of the port programme, and the **seventeenth** one of
its last phase: the handle itself.

The daemon's **default identity** — the model the runs fall back to and the
system instructions they fall back to — was the last thing the config reader
held a kernel handle for.

## What the slice found

| file | sites | whose identity |
|---|---|---|
| `kernel/controlplane/config_handler.go` | 2 calls | the routed one |
| `kernel/controlplane/app_operations.go` | 4 method values | the daemon's own |

| | before | after |
|---|---|---|
| call sites on the handle (the adapter) | **12** | **10** |
| distinct kernel verbs the adapter names | **10** | **8** |
| method values handed over | **8** | **4** |
| control-plane files naming the runtime | **13** | **12** |

The ratchet moved with the slice: the config reader no longer names the runtime
at all, so the control plane's runtime-naming count went from **thirteen**
production files to **twelve**, and the recorded number changed in the same
slice — as the ratchet requires.

The **application** layer held no identity handle either: `kernel/app/persona`
already took the system prompt and the setter as **fields** of its own
`Ports` — filled by the adapter, with the kernel's method values handed over
(`System: s.k.System, SetSystem: s.k.SetSystem`).

## What changed

The persona family's identity became a **port of its own**
(`kernel/app/persona/ports.go`):

```go
type Identity struct {
	defaultModel func() string
	prompt       func() string
	setPrompt    func(string)
}

func IdentityFromKernel(k *runtime.Kernel) Identity { ... }

func (i Identity) DefaultModel() string { ... }
func (i Identity) Prompt() string       { ... }
func (i Identity) PromptSet() bool      { ... }
func (i Identity) SetPrompt(string)     { ... }
```

and the adapter keeps the bindings (`kernel/controlplane/persona_ports.go`) —
which do not name the runtime, like every binding since W5.4v:

```go
func (s *Server) personaPorts() apppersona.Ports { ... }
func requestPersonaPorts(ctx context.Context) apppersona.Ports { ... }
```

There are **two** bindings, and they answer for **two different kernels**:

- The persona **surface** — the owner's chat defaults, the identity the owner
  edits — is **PRIMARY only**, so it is the daemon's own.
- The **config surface** reports the identity — the model and whether a prompt
  is set — and it runs on the **request's** path, so it asks the **routed**
  kernel.

### The config reader holds no kernel handle at all

The reader's every field belongs to a family, and it asks each of them: the
tools' count, the policy's ask-policy, the provider's routing, the plugins'
count, the base dir's path — and now the **persona's** model and prompt.

So the reader's `kernel` field is gone. That is the whole point of the slice:
the adapter's config reader no longer reaches for the kernel at all.

### The port's names are not the kernel's

The port answers `DefaultModel`, `Prompt`, `PromptSet` and `SetPrompt` — never
`Model` and never `System`. A port that carried the name of the verb it stands
for would let the adapter name the kernel's verb through the port's door — and
the guard would read that call as the adapter reaching for the kernel.

### Two other families ask the identity, not the kernel

The **chat summarizer** and the **nodes** family needed the daemon's default
model too, and they asked the **kernel** for it (`k.Model`). They now ask the
**identity** family for it: `s.personaPorts().Identity.DefaultModel`.

## The rule

The identity's verbs are the application's, and the rule is by **two whole
words**:

> a kernel verb whose name carries "System" **or** "Model" as a complete word is
> the identity's business.

The sweep takes in **four** verbs today: `Model`, `SetModel`, `System`,
`SetSystem`.

Four verbs carry those words and are **not** the identity's:
`ResolveRunModel` and `currentTaskModelChain` are the **routing's** business —
which model a run resolves — `SystemTaskPorts` is the **system task** family's,
and `VisionModel` is the daemon's **vision sidecar** — the model that captions
images for a run whose active model cannot see them. A rule that swept them in
would make the persona family claim four other families' verbs — and a family
that claims everything claims nothing.

The **context helpers** are not the identity's either: the system agent from
the context is the roster's business, and the model chain from the context is
the routing's.

The guard now holds **144** application-owned verbs.

## The tests

| test | holds |
|---|---|
| `TestThePersonaIdentityReachesTheKernel` | the identity hands over the daemon's model and prompt, and the prompt changes **live** |
| `TestThePersonaIdentitySurvivesAHostWithNoKernel` | a host with no kernel holds no identity, and the empty answers are the honest ones |
| `TestThePersonaPortNamesAreNotTheKernelVerbs` | no method of the port carries the name of a kernel verb |
| `TestTheAdapterBindsThePersonaPortsToTheRoutedKernel` | the request's ports follow the **routed** kernel, and the daemon's own request answers for the daemon's own |
| `TestTheConfigReaderReportsTheIdentityThroughThePort` | the reader reports the model and the prompt's presence **both ways** — a daemon with a prompt says so, a daemon without one says the opposite |
| `TestThePersonaPortsEditTheDaemonsIdentity` | the daemon-level binding edits the identity live |
| `TestThePersonaSurfaceEditsTheDaemonsIdentity` | the persona **surface**, driven over the socket, edits the identity live |
| `TestTheGuardNamesEveryPersonaVerbTheKernelHas` | the guard names every identity verb the kernel has |
| `TestThePersonaFamilyClaimsNoOtherFamilysVerbs` | the persona's list claims no other family's verb and does not claim the whole kernel |

**9** permanent tests.

## The mutations

**15** mutations, **15** caught, in two independent passes with **byte-identical**
logs (SHA-256 `9426CAAE16232BAD089448CB5323D283309F7099DFAA56417E24AFC14A83897A`).

M01, M02, M03 and M04 are the ones that bite: the identity holds **no** model, a
host with no kernel asks the kernel **anyway**, the identity hands over **no**
prompt, and the identity says **no** prompt is set.

M05 gives the port a method that carries a kernel verb's own name, M06 makes
the config reader report the prompt as the model, M07 makes it say a prompt is
**always** set, M08 empties the request's binding, M09 makes it follow the
**daemon's** kernel, M10 empties the daemon's binding, M11 makes the persona
surface edit **nothing**, M12 puts the kernel back in the chat summarizer and
the **verb guard** catches it, M13 takes the family out of the guard, M14 hands
the family **another** family's verbs, and M15 makes the binding name the
runtime.

### Two of the fifteen were caught only after the slice learned something

M07 and M11 first ran against tests that could not see them. The config reader
test only held the **"a prompt IS set"** answer, so a reader that always said
"set" was not wrong in front of it. And the persona test asked the **binding**
directly, so a mutation in the adapter's operation wiring — where the surface
is really built — was invisible to it.

The slice then wrote the missing halves: the reader test holds **both**
answers (a daemon with a prompt and a daemon without one), and the persona
surface test drives the **socket**, the way the operator does, so the whole
path is held. A test that asks a port directly holds the port; a test that
drives the surface holds the wiring.

## What the slice does not do

The handle stays, and the adapter still calls **10** kernel verbs on it and
hands over **4** method values: the run decorators, the state, the data lake,
the seats, the reflect, the reaper, the update drain and the rest.

The **runs family** still holds the kernel handle — for the run decorators, the
roster, the warden and the catalog. It is the next family, and it is the
largest one left.

The daemon's **overview** (`kernel/app/system`) still holds the handle for
**thirteen** verbs across **eight** families. That surface is not one family but a
**roll-up** of many, and it will have to ask each of them rather than the kernel.
