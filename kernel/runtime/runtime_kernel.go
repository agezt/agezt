// SPDX-License-Identifier: MIT

package runtime

// Provenance: Kernel struct: the composition root that holds every subsystem
//             (journal, bus, state, agent loop, providers, tools). Code extracted
//             from runtime.go during the Day-41 god-file split. Public API
//             unchanged.

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/artifactapi"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/catalogapi"
	"github.com/agezt/agezt/kernel/contract/datalakeapi"
	"github.com/agezt/agezt/kernel/contract/marketapi"
	"github.com/agezt/agezt/kernel/contract/mcpapi"
	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/contract/okrapi"
	"github.com/agezt/agezt/kernel/contract/seatapi"
	"github.com/agezt/agezt/kernel/contract/skillapi"
	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/contract/tasteapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolforgeapi"
	"github.com/agezt/agezt/kernel/contract/toolphaseapi"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/contract/workflowapi"
	"github.com/agezt/agezt/kernel/contract/worldmodelapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/journal"
	agentsapi "github.com/agezt/agezt/kernel/modules/agents/api"
	"github.com/agezt/agezt/kernel/modules/reasoning/api"
	workapi "github.com/agezt/agezt/kernel/modules/work/api"
	"github.com/agezt/agezt/kernel/reflect"
	"github.com/agezt/agezt/kernel/resume"
	"github.com/agezt/agezt/kernel/runtime/lifecycle"
	"github.com/agezt/agezt/kernel/runtime/runexec"
	"github.com/agezt/agezt/kernel/scheduler"
	"github.com/agezt/agezt/kernel/state"
	"github.com/agezt/agezt/kernel/warden"
)

type Kernel struct {
	cfg Config

	journal   *journal.Journal
	state     *state.FileStore
	bus       *bus.Bus
	edict     *edict.Engine
	warden    warden.Engine
	approvals *approval.Registry
	scheduler *scheduler.Executor

	memory       memoryapi.Manager   // the memory manager — the contract's port (W3.57)
	memoryStore  memoryapi.Store     // the memory's file — the contract's store port (W3.57)
	world        worldmodelapi.Graph // the world-model graph — the contract's port (W3.54)
	worldStore   worldmodelapi.Store // the world-model file — the contract's store port (W3.54)
	forge        skillapi.Curator    // the skill curator — the contract's port (W3.55)
	skillStore   skillapi.Store      // the skill library's file — the contract's store port (W3.55)
	marketMgr    marketapi.Manager
	standing     standingapi.Store // the standing-order library — the contract's store port (W3.50)
	resume       *resume.Store     // durable in-flight-run tickets for restart resume (M1002); nil when disabled
	roster       agentapi.Store
	toolForge    toolforgeapi.HostStore
	mcpStore     mcpapi.Registry
	workflows    workflowapi.Store
	wfEngine     workflowapi.Engine
	workboard    workboardapi.HostStore
	okr          okrapi.Store   // the objective library — the contract's store port (W3.53)
	taste        tasteapi.Store // the curated exemplar library — the contract's store port (W3.52)
	seat         seatapi.Store
	artifacts    artifactapi.Blob
	artIndex     artifactapi.HostIndex // metadata sidecar over artifacts (M822): browsable/deletable entries
	lake         datalakeapi.Lake      // Personal Data Lake (M834): agent-built structured collections
	reflect      *reflect.Engine
	schedules    cadenceapi.HostStore    // persistent typed schedule store (autonomy) — the contract's host port (W3.49)
	schedEngine  cadenceapi.Engine       // live cadence resident, set by the daemon after Open
	configCenter *configcenter.Center    // config center for agent SDK config access
	toolInvoker  toolapi.Invoker         // immutable invocation port, constructed per Open
	toolPhases   toolphaseapi.Phases     // same service's batch-aware port for root/delegated loops
	tools        map[string]toolapi.Tool // cfg.Tools + the memory/world tools (when enabled)

	// conductorExec is the optional code-execution backend the Conductor's
	// Verifier role uses to actually RUN a worker's code (M997). Injected once
	// after Open by the daemon (SetConductorExec, wired from the code_exec tool)
	// so the kernel never imports the codeexec plugin. nil when the sandbox is
	// off — the Verifier then falls back to LLM critique.
	conductorExec CodeExecutor

	// reasoning is the reasoning module (the Council, the Conductor and the
	// deep-research harness). The module lives in kernel/modules/reasoning; the
	// runtime cannot import it (a module may import another module's api
	// package and nothing else — archcheck's principle P5), so the composition
	// root builds the service and injects it here with SetReasoning. nil until
	// it is wired.
	reasoning api.Service

	// board is the WORK MODULE's task board — the lifecycle over the store the
	// host opened (W5.6). The module lives in kernel/modules/work; the runtime
	// cannot import it (a module may import another module's api package and
	// nothing else — archcheck's principle P5), so the composition root builds
	// the service over the store and injects it here with SetWorkboard. nil
	// until it is wired, and every board verb refuses until then.
	board workapi.Service

	// reaperScanner is the agents module's reaper (the read-only fleet health
	// scan). The module lives in kernel/modules/agents/reaper; the runtime
	// cannot import it (a module may import another module's api package and
	// nothing else — archcheck's principle P5), so the composition root builds
	// the scanner and injects it here with SetReaperScanner. nil until it is
	// wired.
	reaperScanner agentsapi.ReaperScanner

	// The catalog's library is the DAEMON's file (W3.51): the kernel holds
	// the contract's ports, and the daemon opens the directory and hands
	// them over.
	catalogStore catalogapi.Store    // the on-disk catalog — the contract's store port
	catalog      *catalogapi.Catalog // snapshot — refreshable via ReloadCatalog
	catalogSync  catalogapi.Syncer   // the remote fetcher — the contract's syncer port

	// lifecycle is the run-lifecycle sub-manager extracted as a
	// separate Go sub-package on Day 12. See kernel/runtime/lifecycle
	// for the surface and the KernelAPI interface that wires it back
	// to *Kernel. Initialised in Open(); nil before that, so any
	// method that goes through it is safe to call only post-Open.
	lifecycle *lifecycle.Manager

	// runexec is the run-engine sub-manager extracted as a separate
	// Go sub-package on Day 21. See kernel/runtime/runexec. As of
	// Day 22 only the entry points (Run / RunAssured / RunWith /
	// NewCorrelation / Why) are routed through the sub-package; the
	// 260-line RunWith body migrates in Day 23.
	runexec *runexec.Runner

	// Fine-grained mutexes to reduce lock contention. Lock ordering to prevent
	// deadlocks (always acquire in this order):
	//   configMu (light config) < runsMu < fanoutMu < treeMu < steersMu < spawnsMu < mcpMu
	configMu sync.Mutex // guards: system, model, cfg, catalog, schedEngine
	runsMu   sync.Mutex // guards: halted, runs
	fanoutMu sync.Mutex
	treeMu   sync.Mutex
	steersMu sync.Mutex
	spawnsMu sync.Mutex
	mcpMu    sync.Mutex // guards: mcpConns

	halted bool
	// suspending latches true when Suspend begins tearing the daemon down for a
	// restart (M1002). A run cancelled while this is set is treated as INTERRUPTED
	// (its ticket is kept for resume) rather than operator-cancelled (ticket
	// deleted). Never cleared — the process is on its way out.
	suspending atomic.Bool
	system     string                        // live daemon default identity / system prompt (M710); seeded from cfg.System, editable at runtime
	model      string                        // live default model id (M816); seeded from cfg.Model, hot-swapped on provider reload
	runs       map[string]context.CancelFunc // correlation_id → cancel
	fanout     map[string]int                // spawning correlation_id → sub-agents spawned (M46 fan-out bound)
	tree       map[string]int                // root correlation_id → total sub-agents in the tree (M629 total bound)
	steers     map[string]*runControl        // correlation_id → live-steering control surface (M608)
	spawns     map[string]*spawnHandle       // child correlation_id → pending/finished async delegation (M881)
	runWG      sync.WaitGroup                // in-flight runs + async spawn goroutines; Close drains it bounded (M883)

	// toolCaps is the validated declared-capability overlay (M900): tool name
	// → Edict capability, consulted by policyHook before the built-in
	// classification. Built once at Open from cfg.ToolCapabilities (known
	// capabilities only); read-only afterwards, so no lock needed.
	toolCaps map[string]edict.Capability
	// mcpConns are the LIVE MCP attachments (M796): server name → connection.
	// Merged into every run's tool map (mergeMCPTools); detach removes.
	mcpConns map[string]mcpapi.Conn

	startTime time.Time // wall-clock at Open() — powers `agt status` uptime
}

// Lifecycle API accessors (the kernel/runtime/lifecycle sub-package
// speaks to *Kernel through these; see kernel/runtime/lifecycle/api.go).
// They are intentionally narrow — every public method is a new piece
// of coupling between the kernel umbrella and the sub-package, and
// the whole point of the split is to *narrow* that coupling.

// Halted is the read side of the halt flag. Used by lifecycle.Manager
