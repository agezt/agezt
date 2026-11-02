// SPDX-License-Identifier: MIT

package runner

import (
	"context"
	"time"

	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/artifactapi"
	"github.com/agezt/agezt/kernel/contract/assureapi"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/catalogapi"
	"github.com/agezt/agezt/kernel/contract/datalakeapi"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/mediaapi"
	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/contract/skillapi"
	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/worldmodelapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/intent"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/modules/runs/agent"
	"github.com/agezt/agezt/kernel/reflect"
	"github.com/agezt/agezt/kernel/state"
	"github.com/agezt/agezt/kernel/warden"
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
type KernelAPI interface {
	Bus() *bus.Bus
	Journal() *journal.Journal
	Provider() llm.Provider
	Tools() map[string]toolapi.Tool
	MaxDuration() time.Duration
	Model() string
	Voice() mediaapi.Voice
	VisionModel() func() (string, bool)
	MemoryDistill() bool
	MemoryDistillMinTools() int
	SkillForge() bool
	SkillForgeMinTools() int
	ShadowEval() bool
	FoldRunTools(corr string) (int, []string)
	CompleteAux(ctx context.Context, corr, taskType string, req llm.CompletionRequest) (*llm.CompletionResponse, error)
	UpdateProfile(ref string, mutate func(*agentapi.Profile)) (agentapi.Profile, bool, error)
	SetProfileRetired(ref string, retired bool, reason ...string) (agentapi.Profile, error)
	SetupRunState(corr string, parentCtx context.Context) (context.Context, context.CancelFunc, agent.Steerer, error)
	CleanupRunState(corr string) []context.CancelFunc
	DeregisterRunSteer(corr string)
	ResumeOwnedKindFromCtx(ctx context.Context) (string, bool)
	ResumeSeedFromCtx(ctx context.Context) ([]llm.Message, int, bool)
	DisableHeuristicBypass(ctx context.Context) bool
	BuildRunPrompt(runCtx context.Context, corr, actor, intent string, systemAgent bool, skillDirective skillapi.ActivationDirective) (string, []string)
	InjectHostEnvironment(system string, tools map[string]toolapi.Tool) string
	ResumeCheckpointFn(corr string) func(int, []llm.Message)
	ResolveRunModel(ctx context.Context) (string, bool)
	Edict() *edict.Engine
	Warden() warden.Engine
	Approvals() *approval.Registry
	Memory() memoryapi.Manager
	World() worldmodelapi.Graph
	Reflect() *reflect.Engine
	Forge() skillapi.Curator
	ArtifactIndex() artifactapi.HostIndex
	DataLake() datalakeapi.Lake
	State() *state.FileStore
	Schedules() cadenceapi.HostStore
	Roster() agentapi.Store
	// RosterGet is the NIL-SAFE roster read: a kernel with no roster has no
	// agents, not a panic.
	RosterGet(ref string) (agentapi.Profile, bool)
	RosterList() []agentapi.Profile
	Standing() standingapi.Store
	StandingList() []standingapi.Order
	ConfigCenter() *configcenter.Center
	Catalog() *catalogapi.Catalog
	CatalogStore() catalogapi.Store
	PublishIntentInterpreted(corr, actor string, frame intent.Frame)
	PublishContextFailureAnalysis(corr, actor string, err error)
	PublishHeuristicBypass(ctx context.Context, corr, actor, intent, answer string) error
	CompleteAgentLifecycle(ctx context.Context, corr string)
	MaybeDistill(ctx context.Context, corr, intent, answer string)
	MaybeForge(ctx context.Context, corr, intent, answer string)
	MaybeShadowEval(ctx context.Context, corr, intent, answer string)
	BuildLoopConfig(ctx context.Context, corr, model string) agent.LoopConfig

	// Day 22 entry points. Day 23 will move the body of RunWith
	// into Runner; for now the Runner just delegates back so the
	// public surface is consistent.
	RunAssured(ctx context.Context, corr, intent string, maxAttempts int) (string, assureapi.Result, error)
	RunWith(ctx context.Context, corr, intent string) (string, error)
	RunWithRetry(ctx context.Context, corr, intent string, pol agentapi.RetryPolicy) (string, error)
	NewCorrelation() string
	VerifyCompletion(ctx context.Context, corr, task, answer string) (assureapi.Verdict, error)
	ClaimResumeTicket(ctx context.Context, corr, intent, kind string, assureBudget int) (context.Context, bool)
	FinalizeResumeTicket(corr string, runErr error)
	RetryReason(err error) string
	AgentRetryable(reason string, retryOn []string) bool
	RetryDelay(pol agentapi.RetryPolicy, attempt int) time.Duration
}
