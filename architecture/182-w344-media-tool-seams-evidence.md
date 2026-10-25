# W3.44 — the media tools are the daemon's

## What the slice is

The kernel still reached **17** modules after W3.43. This slice takes **four**
of those edges at once, and they are one thing: the **voice, image and rerank
tools**.

| Edge | Why it existed |
|---|---|
| `kernel/runtime -> kernel/voicetool` | the kernel built the `voice` tool |
| `kernel/runtime -> kernel/imagetool` | the kernel built the `image_generate` tool |
| `kernel/runtime -> kernel/reranktool` | the kernel built the `rerank` tool |
| `kernel/runtime/runexec -> kernel/voicetool` | the run engine's API declared `Voice() voicetool.Voice` |

The kernel built the three tools because the tools live in their own packages
and the kernel held the adapters they were built over. Naming the tool package
meant naming the seam with it — so the seam had to be nameable from the kernel
too.

The ratchet went **59 → 55**.

## What the kernel used to do

```go
// kernel/runtime/compose.go
var vTool *voicetool.Tool
if cfg.Voice != nil {
    vTool = voicetool.New(cfg.Voice)
    effTools["voice"] = vTool
}
var imgTool *imagetool.Tool
if cfg.ImageGenerator != nil {
    imgTool = imagetool.New(cfg.ImageGenerator)
    effTools["image_generate"] = imgTool
}
if cfg.Reranker != nil {
    effTools["rerank"] = reranktool.New(cfg.Reranker)
}
// … and, after the kernel was built:
if imgTool != nil && k.artifacts != nil {
    imgTool.SaveArtifact = k.artifacts.Put
}
if vTool != nil && k.artifacts != nil {
    vTool.SaveArtifact = k.artifacts.Put
}
```

and the seams were **aliases of the tool packages**:

```go
// kernel/runtime/toolseams.go
type Voice = voicetool.Voice
type ImageGen = imagetool.ImageGen
type Reranker = reranktool.Reranker
```

## What it does now

### The seams are contract rows

`kernel/contract/mediaapi` is new — the three adapter seams the daemon
injects:

```go
type Voice interface { Transcribe; Speak; HasSTT; HasTTS }
type ImageGen interface { GenerateImage; HasImage }
type Reranker interface { Rerank; HasRerank }
```

The tool packages keep their names as **aliases**, so every existing importer
compiles unchanged:

```go
// kernel/voicetool/voice.go
type Voice = mediaapi.Voice
```

and the kernel names the **contract's row**, not the tool package:

```go
// kernel/runtime/toolseams.go
type Voice = mediaapi.Voice
```

The seams are **structural** — every method set speaks only in standard
library types — so a provider plugin satisfies the seam without importing
anything from the kernel. That is what lets the plugin stay a plugin, and the
contract now pins it.

### The tools are the daemon's

The kernel may not name a module's implementation, so it may not name a tool
package. The composition root builds the three tools and hands them over as
ordinary tools:

```go
// cmd/agezt/media_tools.go
func mediaTools(tools map[string]toolapi.Tool, cfg kernelruntime.Config) map[string]toolapi.Tool {
    out := make(map[string]toolapi.Tool, len(tools)+3)
    maps.Copy(out, tools)
    if cfg.Voice != nil {
        vt := voicetool.New(cfg.Voice)
        if cfg.Artifacts != nil {
            vt.SaveArtifact = cfg.Artifacts.Put
        }
        out["voice"] = vt
    }
    …
}
```

and `openAppKernel` calls it, after the artifact store is open — because the
two tools that persist bytes bind it:

```go
// cmd/agezt/main_toolinvoker.go
cfg.Tools = mediaTools(cfg.Tools, cfg)
k, err := kernelruntime.Open(cfg)
```

The tool map is **copied, never mutated**: the primary kernel and every tenant
share one map, and each kernel must see *its own* tools bound to *its own*
artifact store. A map written in place would hand the last kernel's tools to
all of them.

### The store is the same store

Before the slice, the tools wrote into `k.artifacts` — the kernel's blob port.
They write into `cfg.Artifacts` now — the daemon's store, which **is** the
kernel's store, because the daemon opened it and injected it (W3.43). The
bytes land in the same place they always did.

### The kernel keeps the one accessor it needs

`k.Voice()` stays: the daemon's channel paths and the web UI ask the kernel
"is speech configured?" on the daemon's behalf. It answers with the contract's
row now, and the run engine's `KernelAPI` declares the same row.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `kernel/contract/mediaapi`: `TestTheSeamsCarryExactlyTheirVerbs` | the three seams carry exactly the verbs the three tools use — a new verb has to be earned |
| `kernel/contract/mediaapi`: `TestTheSeamsAreStructural` | every seam method speaks only in standard library types, so a plugin satisfies it without importing the kernel |
| `cmd/agezt`: `TestOpenAppKernelWiresTheMediaTools` | the daemon end: the three tools are live in the kernel's tool set, the voice tool's bytes land in the **daemon's** store (the ref resolves through the kernel's own blob port), and the caller's tool map comes back **untouched** |
| `kernel/runtime`: `TestRuntimeTools_ResolveToGovernedCapabilities` and friends | the capability guard still covers the whole tool set — the three tools are now the host's, and the host hands them over as tools |

### The mutation run

**8 mutations: 8 caught, 0 survived, 0 skipped, 0 build errors.**

| # | Mutation | Caught by |
|---|---|---|
| 1 | the kernel's seam alias points back at the voice package | **archcheck** — `cross-module kernel/runtime -> kernel/voicetool` |
| 2 | the run engine's API names the concrete voice type | **archcheck** — `cross-module kernel/runtime/runexec -> kernel/voicetool` |
| 3 | the kernel builds the voice tool again | **archcheck** — `cross-module kernel/runtime -> kernel/voicetool` |
| 4 | the daemon does not build the voice tool | `TestOpenAppKernelWiresTheMediaTools` |
| 5 | the daemon builds the voice tool but forgets its artifact sink | `TestOpenAppKernelWiresTheMediaTools` |
| 6 | the daemon writes into the **caller's** tool map | `TestOpenAppKernelWiresTheMediaTools` |
| 7 | the voice seam grows a verb | `TestTheSeamsCarryExactlyTheirVerbs` |
| 8 | a stale allowlist entry comes back | **archcheck** — a listed edge that no longer occurs |

**Four of the eight are gate-measured.** "The kernel's reach is smaller" is
not something a test can see: a test would happily accept a kernel that builds
the voice tool itself, as long as the tool works.

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 311 packages placed; 55 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — **down from 59** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `staticcheck ./...` | clean |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** |

## The rule the slice is really about

**A tool that lives in its own package cannot be built by a module.** The
kernel is a module; the three tools live in platform packages; so the kernel
could only build them by naming them — and that naming was the edge. The way
out is not a narrower import but a different owner: the composition root
builds the tools, binds their sinks and hands them over as tools, and the
kernel reaches no tool package at all.

**A seam is a row, and a row lives in the contract.** The adapter seam the
daemon injects is not the tool's business — it is the host's. Moving it to the
contract is what let the kernel keep its one accessor (`k.Voice()`) without
keeping the package that owns the tool.

**A shared map is a shared bug.** The primary kernel and every tenant share
one tool map. Copying it before adding the tools is not tidiness — it is what
keeps each kernel's tools bound to its own artifact store.

## What remains

The kernel still reaches **17** modules. The next candidates, in the order the
measurement suggests:

- **`toolforge`** — the rows are aliases, but the kernel calls **eight** store
  methods that the contract's two-verb port does not cover.
- **`workflow` and `workboard`** — both need wide new ports: the kernel names
  `workflow.Engine`, `workflow.Ports`, `workflow.Provenance` and
  `workflow.Trigger` (no contract home), and it calls **seventeen** workboard
  store methods.
- **`market`** — the manager's contract port does not exist yet; the tool has
  to move to the daemon with it.
- **`cadence`, `standing`, `mcp`** — many callers each; the shape is known
  (a contract port the host opens), the work is in the port's width.
