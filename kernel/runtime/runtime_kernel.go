// SPDX-License-Identifier: MIT

// Kernel struct: the composition root that holds every subsystem (journal, bus, state, agent loop, providers, tools).
// Code extracted from runtime.go during the Day-41 god-file split. Public API unchanged.
package runtime


import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/agentgw"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/datalake"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/kernel/mcp"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/okr"
	"github.com/agezt/agezt/kernel/reflect"
	"github.com/agezt/agezt/kernel/resume"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime/accessors"
	"github.com/agezt/agezt/kernel/runtime/lifecycle"
	"github.com/agezt/agezt/kernel/runtime/runexec"
	"github.com/agezt/agezt/kernel/scheduler"
	"github.com/agezt/agezt/kernel/seat"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/kernel/state"
	"github.com/agezt/agezt/kernel/taste"
	"github.com/agezt/agezt/kernel/toolforge"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/kernel/workboard"
	"github.com/agezt/agezt/kernel/workflow"
	"github.com/agezt/agezt/kernel/worldmodel"
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

	memory       *memory.Manager
	memoryDir    *memory.FileStore
	world        *worldmodel.Graph
	worldDir     *worldmodel.FileStore
	forge        *skill.Forge
	skillDir     *skill.FileStore
	marketMgr    *market.Manager
	standing     *standing.Store
	resume       *resume.Store // durable in-flight-run tickets for restart resume (M1002); nil when disabled
	roster       *roster.Store
	toolForge    *toolforge.Store
	mcpStore     *mcp.Store
	workflows    *workflow.Store
	workboard    *workboard.Store
	okr          *okr.Store
	taste        *taste.Store
	seat         *seat.Store
	artifacts    *artifact.Store
	artIndex     *artifact.Index // metadata sidecar over artifacts (M822): browsable/deletable entries
	lake         *datalake.Lake  // Personal Data Lake (M834): agent-built structured collections
	reflect      *reflect.Engine
	schedules    *cadence.Store        // persistent typed schedule store (autonomy)
	schedEngine  *cadence.Engine       // live cadence resident, set by the daemon after Open
	agentGW      *agentgw.Gateway      // agent subprocess gateway (agent SDK)
	configCenter *configcenter.Center  // config center for agent SDK config access
	tools        map[string]agent.Tool // cfg.Tools + the memory/world tools (when enabled)

	// conductorExec is the optional code-execution backend the Conductor's
	// Verifier role uses to actually RUN a worker's code (M997). Injected once
	// after Open by the daemon (SetConductorExec, wired from the code_exec tool)
	// so the kernel never imports the codeexec plugin. nil when the sandbox is
	// off — the Verifier then falls back to LLM critique.
	conductorExec CodeExecutor

	catalogStore *catalog.Store
	catalog      *catalog.Catalog // snapshot — refreshable via ReloadCatalog

	// lifecycle is the run-lifecycle sub-manager extracted as a
	// separate Go sub-package on Day 12. See kernel/runtime/lifecycle
	// for the surface and the KernelAPI interface that wires it back
	// to *Kernel. Initialised in Open(); nil before that, so any
	// method that goes through it is safe to call only post-Open.
	lifecycle *lifecycle.Manager

	// accessors is the read-side sub-manager extracted as a separate
	// Go sub-package on Day 13. See kernel/runtime/accessors. As of
	// this commit only the simple store getters (Journal, Bus,
	// State, Edict, Warden, Approvals, Scheduler, Provider, Memory,
	// AgentGateway, Schedules) are routed through it; the rest of
	// the read surface lands in subsequent commits.
	accessors *accessors.Accessor

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
	mcpConns map[string]mcp.Conn

	startTime time.Time // wall-clock at Open() — powers `agt status` uptime
}

// Lifecycle API accessors (the kernel/runtime/lifecycle sub-package
// speaks to *Kernel through these; see kernel/runtime/lifecycle/api.go).
// They are intentionally narrow — every public method is a new piece
// of coupling between the kernel umbrella and the sub-package, and
// the whole point of the split is to *narrow* that coupling.

// Halted is the read side of the halt flag. Used by lifecycle.Manager