// SPDX-License-Identifier: MIT

package runner

import (
	"context"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/catalogapi"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/contract/skillapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/worldmodelapi"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/modules/runs/agent"
	"github.com/agezt/agezt/kernel/state"
)

// KernelAPI is the surface the run engine needs from its host
// kernel. Day 21a slice: read-side accessors. Day 21b will
// expand with post-run trio, journal re-exports, and the 27
// legacy run-engine methods.
//
// The run's CONTEXT is not part of it (W3.76l): the run context's vocabulary
// belongs to the module that owns it (kernel/modules/runs/runctx), and the
// run engine is INSIDE that module — asking the host to read the run's own
// decoration was a hop that bought nothing. What stays here is the host's
// own state: the effective config, the resume ticket and the seed, the
// profile lookups.
//
// The ROSTER's WRITES are not part of it either (W3.76o): they are the agents
// module's operations, and the run engine — which retires an agent whose run
// completed and advances its cycle counter — asks the MODULE for them. The
// port kept the roster's READ (RosterGet), because a read over the contract's
// port is the host's own state, and the two roster entries nobody in this
// module ever opened (Roster, RosterList) are gone with them.
//
// The RESUME HANDSHAKE is not part of it either (W3.76q): the owned marker
// and the seeded conversation are the resume module's vocabulary, and the run
// engine — which lives next to that module — reads them from there.
//
// The run's LIVE STATE is not part of it either (W3.76ae): the steering
// registry and the per-run registration are the RUNS MODULE's own
// (kernel/modules/runs/state), and the run engine — which lives inside that
// module — asks the module for them. The host held three verbs for them
// (SetupRunState, CleanupRunState, DeregisterRunSteer) and the module asked
// the host: a round trip inside one module.
//
// The PROMPT ASSEMBLY is not part of it either (W3.76af): the layer that
// decides what a run knows before it starts is the RUNS MODULE's own
// (kernel/modules/runs/prompt), and the run engine holds the very same
// service the delegation engine holds. The host held two verbs over it
// (BuildRunPrompt, InjectHostEnvironment) and the module asked the host: a
// round trip inside one module, twice.
//
// The run's PUBLISH SURFACE is not part of it either (W3.76ag): what a run
// says about itself — the reading it made of the user's utterance and the
// analysis of what its context selection left out — is the RUNS MODULE's own
// (kernel/modules/runs/publish). The host held two verbs over them
// (PublishIntentInterpreted, PublishContextFailureAnalysis) and the module
// asked the host: a round trip inside one module, twice more.
//
// The TICKET LIFECYCLE is not part of it either (W3.76ah): who owns the
// durable resume ticket, when it is refreshed and when it is deleted is the
// RUNS MODULE's own (kernel/modules/runs/resume). The host held three verbs
// over it (ClaimResumeTicket, FinalizeResumeTicket, ResumeCheckpointFn) and
// the module asked the host: a round trip inside one module, three times
// more.
type KernelAPI interface {
	Bus() *bus.Bus
	Journal() *journal.Journal
	Provider() llm.Provider
	Tools() map[string]toolapi.Tool
	MaxDuration() time.Duration
	Model() string
	VisionModel() func() (string, bool)
	MemoryDistill() bool
	MemoryDistillMinTools() int
	SkillForge() bool
	SkillForgeMinTools() int
	ShadowEval() bool
	FoldRunTools(corr string) (int, []string)
	CompleteAux(ctx context.Context, corr, taskType string, req llm.CompletionRequest) (*llm.CompletionResponse, error)
	DisableHeuristicBypass(ctx context.Context) bool
	ResolveRunModel(ctx context.Context) (string, bool)
	Memory() memoryapi.Manager
	World() worldmodelapi.Graph
	Forge() skillapi.Curator
	State() *state.FileStore
	// RosterGet is the NIL-SAFE roster read: a kernel with no roster has no
	// agents, not a panic.
	//
	// It is the host's own state (a read over the contract's port), so it
	// stays. The roster's WRITES left this port in W3.76o: they are the
	// agents module's operations, and the run engine asks the module for them
	// (see the Runner's roster field) instead of asking the host to ask the
	// module.
	RosterGet(ref string) (agentapi.Profile, bool)
	Catalog() *catalogapi.Catalog
	BuildLoopConfig(ctx context.Context, corr, model string) agent.LoopConfig

	// The run engine's OWN operations are not in this port (W3.76ab).
	//
	// PublishHeuristicBypass, MaybeDistill, MaybeForge, MaybeShadowEval,
	// RunWith, RunWithRetry and CompleteAgentLifecycle used to sit here, and
	// the run engine asked the HOST for them — and the host asked the run
	// engine back. A port entry that is a round trip is a promise the host
	// never had to keep: the module has the body, so the module calls it.
	//
	// What stayed in the port is what the HOST owns: the run's setup and
	// cleanup, the prompt, the loop configuration, the retry policy and the
	// host's own state.
	NewCorrelation() string
	RetryReason(err error) string
	AgentRetryable(reason string, retryOn []string) bool
	RetryDelay(pol agentapi.RetryPolicy, attempt int) time.Duration
}
