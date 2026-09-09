// SPDX-License-Identifier: MIT

package runexec

import (
	"context"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/agentgw"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/assure"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/datalake"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/intent"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/reflect"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/scheduler"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/kernel/state"
	"github.com/agezt/agezt/kernel/voicetool"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/kernel/worldmodel"
)

// KernelAPI is the surface the run engine needs from its host
// kernel. Day 21a slice: read-side accessors. Day 21b will
// expand with post-run trio, journal re-exports, and the 27
// legacy run-engine methods.
type KernelAPI interface {
	Bus() *bus.Bus
	Journal() *journal.Journal
	Provider() agent.Provider
	Tools() map[string]agent.Tool
	MaxDuration() time.Duration
	Model() string
	Voice() voicetool.Voice
	VisionModel() func() (string, bool)
	MemoryDistill() bool
	MemoryDistillMinTools() int
	SkillForge() bool
	SkillForgeMinTools() int
	ShadowEval() bool
	FoldRunTools(corr string) (int, []string)
	CompleteAux(ctx context.Context, corr, taskType string, req agent.CompletionRequest) (*agent.CompletionResponse, error)
	UpdateProfile(ref string, mutate func(*roster.Profile)) (roster.Profile, bool, error)
	SetProfileRetired(ref string, retired bool, reason ...string) (roster.Profile, error)
	SetupRunState(corr string, parentCtx context.Context) (context.Context, context.CancelFunc, agent.Steerer, error)
	CleanupRunState(corr string) []context.CancelFunc
	DeregisterRunSteer(corr string)
	SystemAgentFromCtx(ctx context.Context) bool
	AgentDailyMcFromCtx(ctx context.Context) int64
	ModelChainFromCtx(ctx context.Context) []string
	WakeContextSource(ctx context.Context) string
	WakeContextReason(ctx context.Context) string
	WakeContextScheduleID(ctx context.Context) string
	WakeContextStandingID(ctx context.Context) string
	WakeContextStandingName(ctx context.Context) string
	WakeContextTriggerSubject(ctx context.Context) string
	WakeContextParentCorrelation(ctx context.Context) string
	ImagesFromCtx(ctx context.Context) []string
	JSONModeFromCtx(ctx context.Context) bool
	MaxCostFromCtx(ctx context.Context) int64
	RunTimeoutFromCtx(ctx context.Context) time.Duration
	ResumeOwnedKindFromCtx(ctx context.Context) (string, bool)
	ResumeSeedFromCtx(ctx context.Context) ([]agent.Message, int, bool)
	DisableHeuristicBypass(ctx context.Context) bool
	BuildRunPrompt(runCtx context.Context, corr, actor, intent string, systemAgent bool, skillDirective skill.ActivationDirective) (string, []string)
	InjectHostEnvironment(system string, tools map[string]agent.Tool) string
	ResumeCheckpointFn(corr string) func(int, []agent.Message)
	ResolveRunModel(ctx context.Context) (string, bool)
	MergeAutoApproveCapabilities(ctx context.Context, from map[string]bool) map[string]bool
	WithActorCorrelation(ctx context.Context, actor, corr string) context.Context
	ActorFromCtx(ctx context.Context) string
	Edict() *edict.Engine
	Warden() warden.Engine
	Approvals() *approval.Registry
	Scheduler() *scheduler.Executor
	Memory() *memory.Manager
	World() *worldmodel.Graph
	Reflect() *reflect.Engine
	SkillStore() *skill.FileStore
	Forge() *skill.Forge
	ArtifactStore() *artifact.Store
	ArtifactIndex() *artifact.Index
	DataLake() *datalake.Lake
	State() *state.FileStore
	Schedules() *cadence.Store
	Roster() *roster.Store
	Standing() *standing.Store
	StandingList() []standing.Order
	ConfigCenter() *configcenter.Center
	Catalog() *catalog.Catalog
	CatalogStore() *catalog.Store
	AgentGateway() *agentgw.Gateway
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
	RunAssured(ctx context.Context, corr, intent string, maxAttempts int) (string, assure.Result, error)
	RunWith(ctx context.Context, corr, intent string) (string, error)
	RunWithRetry(ctx context.Context, corr, intent string, pol roster.RetryPolicy) (string, error)
	NewCorrelation() string
	VerifyCompletion(ctx context.Context, corr, task, answer string) (assure.Verdict, error)
	ClaimResumeTicket(ctx context.Context, corr, intent, kind string, assureBudget int) (context.Context, bool)
	FinalizeResumeTicket(corr string, runErr error)
	AgentSlugFromCtx(ctx context.Context) string
	AgentRetryPolicyFromCtx(ctx context.Context) (roster.RetryPolicy, bool)
	RetryReason(err error) string
	AgentRetryable(reason string, retryOn []string) bool
	RetryDelay(pol roster.RetryPolicy, attempt int) time.Duration
}
