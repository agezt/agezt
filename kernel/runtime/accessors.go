// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/datalake"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/kernel/reflect"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime/types"
	"github.com/agezt/agezt/kernel/scheduler"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/kernel/ulid"
	"github.com/agezt/agezt/kernel/worldmodel"
)

// Journal exposes the underlying journal for read-only inspection (used by
// the control plane's `why` and `journal verify`).
// This file is the kernel's read-mostly surface: the public accessors
// and CRUD helpers the rest of the codebase uses to drive the kernel
// without touching its mutex invariants directly. Pulled out of the
// runtime.go god file as the third step of the Day 9 split (after
// lifecycle.go and compose.go).
//
// As of Day 13 the eleven "easy" store getters (Journal, Bus, State,
// Edict, Warden, Approvals, Scheduler, Provider, Memory, AgentGateway,
// Schedules) live in the kernel/runtime/accessors sub-package; their
// legacy *Kernel bodies have become thin delegations to the sub-
// package's Accessor (see runtime.go). The rest of the surface lands
// in subsequent commits.
//
// The live-mutator set (SetModel, SetSystem, SetCouncilMembers,
// SetScheduleEngine, SetMarket) all take configMu; readers that go
// through the same field (Model, System, ScheduleEngine, Catalog) also
// take configMu. Anyone adding a new live-mutator must follow the
// same pattern or risk a data race visible only under `go test -race`.

// SetScheduleEngine records the live cadence resident so status/doctor/UI
// surfaces can observe whether scheduled work is currently running. It is set by
// the daemon, not Open, because cmd/agezt owns the schedule target dispatcher.
func (k *Kernel) SetScheduleEngine(e *cadence.Engine) {
	k.configMu.Lock()
	k.schedEngine = e
	k.configMu.Unlock()
}

// ScheduleEngine returns the live cadence resident when the daemon has started
// it. Nil means schedules can still be managed in the store but no resident is
// currently attached in this process.
func (k *Kernel) ScheduleEngine() *cadence.Engine {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.schedEngine
}

// World returns the world-model graph backing `agt world`, run-time entity
// injection, and the Pulse salience relevance signal. Always non-nil after
// Open.
func (k *Kernel) World() *worldmodel.Graph { return k.world }

// Forge returns the skill manager backing `agt skill`, run-time skill
// activation, and post-run skill proposal. Always non-nil after Open.
func (k *Kernel) Forge() *skill.Forge { return k.forge }

// Market returns the capability marketplace manager (skill/MCP/tool packs). It is
// nil until the daemon wires it via SetMarket (the built-in catalogue is a plugin
// the kernel must not import, so it is injected from cmd/agezt).
func (k *Kernel) Market() *market.Manager { return k.marketMgr }

// SetMarket injects the marketplace manager (from cmd/agezt, with the built-in
// Official library + this kernel's Forge/MCP as the install targets).
func (k *Kernel) SetMarket(m *market.Manager) { k.marketMgr = m }

// Artifacts returns the content-addressed artifact store (SPEC-04 §3.6), where
// the loop offloads oversized tool outputs. Used by retrieval surfaces.
func (k *Kernel) Artifacts() *artifact.Store { return k.artifacts }

// Voice returns the configured voice adapter (STT/TTS), or nil when unset. The
// channel inbound path uses it to auto-transcribe inbound voice notes.
func (k *Kernel) Voice() Voice { return k.cfg.Voice }

// ArtifactIndex returns the metadata index over the blob store (M822) — the
// browsable/deletable per-arrival entries (inbound images, tool outputs) the
// file manager and inbound-image persistence use.
func (k *Kernel) ArtifactIndex() *artifact.Index { return k.artIndex }

// DataLake returns the Personal Data Lake (M834) — the file-based structured
// collections agents build and share, surfaced by the `db` tool and the Web UI.
func (k *Kernel) DataLake() *datalake.Lake { return k.lake }

// Standing returns the standing wake-rule store (SPEC-16 §4), backing `agt
// standing`. Always non-nil after Open.
func (k *Kernel) Standing() *standing.Store { return k.standing }

// AddStanding validates and persists a standing order, journaling
// standing.created so the lifecycle is auditable (SPEC-16 §4).
func (k *Kernel) AddStanding(o standing.Order) (standing.Order, error) {
	saved, err := k.standing.Add(o)
	if err != nil {
		return standing.Order{}, err
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "standing." + saved.ID, Kind: event.KindStandingCreated, Actor: "standing",
		Payload: map[string]any{"id": saved.ID, "name": saved.Name, "triggers": len(saved.Triggers)},
	})
	return saved, nil
}

// SetStandingEnabled pauses/resumes a standing order, journaling standing.updated.
func (k *Kernel) SetStandingEnabled(id string, enabled bool) (standing.Order, error) {
	o, err := k.standing.SetEnabled(id, enabled)
	if err != nil {
		return standing.Order{}, err
	}
	state := "paused"
	if enabled {
		state = "resumed"
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "standing." + id, Kind: event.KindStandingUpdated, Actor: "standing",
		Payload: map[string]any{"id": id, "name": o.Name, "enabled": enabled, "action": state},
	})
	return o, nil
}

// UpdateStanding edits a standing order's mutable fields via mutate, journaling
// standing.updated (action "edited") on success. Identity/lifecycle fields are
// protected by the store. Returns the updated order and whether the id existed
// (false + nil error for an unknown id, mirroring the schedule-edit path).
func (k *Kernel) UpdateStanding(id string, mutate func(*standing.Order)) (standing.Order, bool, error) {
	o, err := k.standing.Update(id, mutate)
	if errors.Is(err, standing.ErrNotFound) {
		return standing.Order{}, false, nil
	}
	if err != nil {
		return standing.Order{}, false, err
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "standing." + id, Kind: event.KindStandingUpdated, Actor: "standing",
		Payload: map[string]any{"id": id, "name": o.Name, "action": "edited"},
	})
	return o, true, nil
}

// RemoveStanding deletes a standing order, journaling standing.removed when it
// existed. Returns whether it existed.
func (k *Kernel) RemoveStanding(id string) (bool, error) {
	o, _ := k.standing.Get(id)
	ok, err := k.standing.Remove(id)
	if err != nil {
		return false, err
	}
	if ok {
		_, _ = k.bus.Publish(event.Spec{
			Subject: "standing." + id, Kind: event.KindStandingRemoved, Actor: "standing",
			Payload: map[string]any{"id": id, "name": o.Name},
		})
	}
	return ok, nil
}

// Roster returns the durable agent-profile store (M783). Always non-nil after Open.
func (k *Kernel) Roster() *roster.Store { return k.roster }

// AddProfile validates and persists a named agent profile, journaling
// roster.created so the agent's birth is auditable.
func (k *Kernel) AddProfile(p roster.Profile) (roster.Profile, error) {
	saved, err := k.roster.Add(p)
	if err != nil {
		return roster.Profile{}, err
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "roster." + saved.Slug, Kind: event.KindRosterCreated, Actor: "roster",
		Payload: map[string]any{"id": saved.ID, "slug": saved.Slug, "name": saved.Name, "model": saved.Model},
	})
	return saved, nil
}

// SetProfileEnabled pauses/resumes an agent profile, journaling roster.updated.
func (k *Kernel) SetProfileEnabled(ref string, enabled bool) (roster.Profile, error) {
	p, err := k.roster.SetEnabled(ref, enabled)
	if err != nil {
		return roster.Profile{}, err
	}
	state := "paused"
	if enabled {
		state = "resumed"
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "roster." + p.Slug, Kind: event.KindRosterUpdated, Actor: "roster",
		Payload: map[string]any{"id": p.ID, "slug": p.Slug, "enabled": enabled, "action": state},
	})
	return p, nil
}

// SetProfileRetired moves an agent to the graveyard (true) or revives it (false)
// by ref, journaling roster.updated. Retiring also pauses the agent so it stops
// firing (M846). A graveyard agent is excluded from delegation (runSubAgent).
func (k *Kernel) SetProfileRetired(ref string, retired bool, reason ...string) (roster.Profile, error) {
	p, err := k.roster.SetRetired(ref, retired, reason...)
	if err != nil {
		return roster.Profile{}, err
	}
	action := "revived"
	if retired {
		action = "retired"
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "roster." + p.Slug, Kind: event.KindRosterUpdated, Actor: "roster",
		Payload: map[string]any{"id": p.ID, "slug": p.Slug, "retired": retired, "reason": p.RetiredReason, "action": action},
	})
	return p, nil
}

// AgentImpact reports what depends on an agent before it is retired/removed
// (M846) — the standing orders that fire AS it. The operator sees this in the
// retire confirmation so the "etkileri" are explicit, not a surprise. Returns the
// affected orders as "name (id)" strings, or nil when nothing references it.
func (k *Kernel) AgentImpact(slug string) []string {
	slug = strings.TrimSpace(slug)
	if slug == "" || k.standing == nil {
		return nil
	}
	var out []string
	for _, o := range k.standing.List() {
		if strings.EqualFold(strings.TrimSpace(o.Agent), slug) {
			name := o.Name
			if name == "" {
				name = o.ID
			}
			out = append(out, fmt.Sprintf("%s (%s)", name, o.ID))
		}
	}
	return out
}

// UpdateProfile edits a profile's mutable fields via mutate, journaling
// roster.updated (action "edited"). Identity/lifecycle fields are protected by
// the store. Returns false + nil error for an unknown ref (standing pattern).
func (k *Kernel) UpdateProfile(ref string, mutate func(*roster.Profile)) (roster.Profile, bool, error) {
	p, err := k.roster.Update(ref, mutate)
	if errors.Is(err, roster.ErrNotFound) {
		return roster.Profile{}, false, nil
	}
	if err != nil {
		return roster.Profile{}, false, err
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "roster." + p.Slug, Kind: event.KindRosterUpdated, Actor: "roster",
		Payload: map[string]any{"id": p.ID, "slug": p.Slug, "action": "edited"},
	})
	return p, true, nil
}

// RemoveProfile deletes an agent profile, journaling roster.removed when it
// existed. Returns whether it existed.
func (k *Kernel) RemoveProfile(ref string) (bool, error) {
	// Shipped guardians (System) are protected from hard delete (M961): they are
	// the daemon's own self-healing fleet. They can still be paused or retired.
	if p, ok := k.roster.Get(ref); ok && p.System {
		return false, fmt.Errorf("agent %q is a protected system guardian — pause or retire it instead of removing", p.Slug)
	}
	gone, ok, err := k.roster.Remove(ref)
	if err != nil {
		return false, err
	}
	if ok {
		_, _ = k.bus.Publish(event.Spec{
			Subject: "roster." + gone.Slug, Kind: event.KindRosterRemoved, Actor: "roster",
			Payload: map[string]any{"id": gone.ID, "slug": gone.Slug, "name": gone.Name},
		})
	}
	return ok, nil
}

// Reflect returns the reflection engine backing `agt reflect` and the optional
// periodic reflection trigger. Always non-nil after Open.
func (k *Kernel) Reflect() *reflect.Engine { return k.reflect }

// StartTime returns the wall-clock time Open() returned. Used by
// `agt status` to compute uptime; not adjusted by Reload or any
// in-process state change, so it reflects "since this process
// started" rather than "since the kernel was last reconfigured".
func (k *Kernel) StartTime() time.Time { return k.startTime }

// Plugins returns the external-plugin manifest the daemon
// supplied at Open(). Read-only — callers must not mutate the
// slice. Used by the control plane to power `agt plugin list`;
// returns nil when no external plugins are configured.
func (k *Kernel) Plugins() []PluginInfo { return k.cfg.Plugins }

// MemoryDistill reports whether post-run memory distillation is
// enabled. The run engine reads this via KernelAPI to decide
// whether to spend one best-effort LLM call folding the journal
// after a multi-tool run (see MaybeDistill).
func (k *Kernel) MemoryDistill() bool { return k.cfg.MemoryDistill }

// MemoryDistillMinTools returns the tool-call threshold that
// triggers a post-run distillation. 0 / unset = the run engine
// uses its own default of 4. Wraps the config field so the runexec
// sub-package can read it through KernelAPI (Day 32).
func (k *Kernel) MemoryDistillMinTools() int { return k.cfg.MemoryDistillMinTools }

// SkillForge reports whether post-run skill-forging is enabled.
// The run engine reads this via KernelAPI to decide whether to
// spend one best-effort LLM call proposing a DRAFT skill from
// the journal after a multi-tool run (see MaybeForge).
func (k *Kernel) SkillForge() bool { return k.cfg.SkillForge }

// SkillForgeMinTools returns the tool-call threshold that
// triggers a post-run skill proposal. 0 / unset = the run engine
// uses its own default of 4. Wraps the config field so the
// runexec sub-package can read it through KernelAPI (Day 32).
func (k *Kernel) SkillForgeMinTools() int { return k.cfg.SkillForgeMinTools }

// SystemAgentFromCtx reports whether the current run is operating
// in system-agent mode (bypasses post-run distillation / forge /
// shadow-eval). Wraps the package-level systemAgentFromCtx so
// the runexec sub-package can read it through KernelAPI (Day 34).
func (k *Kernel) SystemAgentFromCtx(ctx context.Context) bool { return systemAgentFromCtx(ctx) }

// AgentDailyMcFromCtx returns the named agent's daily USD-microcent
// ceiling attached to a run context by WithAgentProfile. Wraps the
// package-level agentDailyMcFromCtx (Day 34).
func (k *Kernel) AgentDailyMcFromCtx(ctx context.Context) int64 { return agentDailyMcFromCtx(ctx) }

// ModelChainFromCtx returns the explicit per-request model chain
// attached to a run context by WithModelChain. Wraps the
// package-level modelChainFromCtx (Day 34).
func (k *Kernel) ModelChainFromCtx(ctx context.Context) []string { return modelChainFromCtx(ctx) }

// WakeContextSource returns the run provenance "source" field
// (e.g. "schedule", "channel", "standing"). Wraps the
// package-level wakeContextFromCtx so the runexec sub-package
// can read individual fields through KernelAPI (Day 34) without
// importing the WakeContext type across the package boundary.
func (k *Kernel) WakeContextSource(ctx context.Context) string { return wakeContextFromCtx(ctx).Source }

// WakeContextReason returns the run provenance "reason" field
// (e.g. "tick", "command", "briefing"). Wraps wakeContextFromCtx.
func (k *Kernel) WakeContextReason(ctx context.Context) string { return wakeContextFromCtx(ctx).Reason }

// WakeContextScheduleID returns the schedule ID that triggered
// the run ("" when the run did not originate from a schedule).
// Wraps wakeContextFromCtx.
func (k *Kernel) WakeContextScheduleID(ctx context.Context) string { return wakeContextFromCtx(ctx).ScheduleID }

// WakeContextStandingID returns the standing order ID that
// triggered the run. Wraps wakeContextFromCtx.
func (k *Kernel) WakeContextStandingID(ctx context.Context) string { return wakeContextFromCtx(ctx).StandingID }

// WakeContextStandingName returns the standing order name that
// triggered the run. Wraps wakeContextFromCtx.
func (k *Kernel) WakeContextStandingName(ctx context.Context) string { return wakeContextFromCtx(ctx).StandingName }

// WakeContextTriggerSubject returns the bus subject that
// triggered the run (e.g. "channel.telegram.message"). Wraps
// wakeContextFromCtx.
func (k *Kernel) WakeContextTriggerSubject(ctx context.Context) string { return wakeContextFromCtx(ctx).TriggerSubject }

// WakeContextParentCorrelation returns the parent run's
// correlation ID (the lead run that delegated this child). Wraps
// wakeContextFromCtx.
func (k *Kernel) WakeContextParentCorrelation(ctx context.Context) string { return wakeContextFromCtx(ctx).ParentCorrelation }

// ImagesFromCtx returns the images attached to a run context by
// WithImages (vision-gated upstream by M91). Wraps the
// package-level imagesFromCtx (Day 34).
func (k *Kernel) ImagesFromCtx(ctx context.Context) []string { return imagesFromCtx(ctx) }

// JSONModeFromCtx reports whether the current run was requested
// with structured-output (response_format=json_object by OpenAI,
// tool_choice="any" by Anthropic). Wraps the package-level
// jsonModeFromCtx (Day 34).
func (k *Kernel) JSONModeFromCtx(ctx context.Context) bool { return jsonModeFromCtx(ctx) }

// MaxCostFromCtx returns the per-run USD-microcent ceiling attached
// to a run context by WithMaxCost (--max-cost). Wraps the
// package-level maxCostFromCtx (Day 34).
func (k *Kernel) MaxCostFromCtx(ctx context.Context) int64 { return maxCostFromCtx(ctx) }

// RunTimeoutFromCtx returns the per-run timeout attached by
// WithRunTimeout (`agt run --timeout`). Wraps the package-level
// runTimeoutFromCtx (Day 34).
func (k *Kernel) RunTimeoutFromCtx(ctx context.Context) time.Duration { return runTimeoutFromCtx(ctx) }

// ResumeOwnedKindFromCtx returns (kind, true) if the run context
// carries a resume-ticket owned by an outer wrapper (assure /
// retry / resumer). Wraps the package-level resumeOwnedKind (Day 34).
func (k *Kernel) ResumeOwnedKindFromCtx(ctx context.Context) (string, bool) { return resumeOwnedKind(ctx) }

// ResumeSeedFromCtx returns (prior messages, start iter, true) if
// the run context carries a seeded conversation snapshot (resumed
// run). Wraps the package-level resumeSeedFromCtx (Day 34).
func (k *Kernel) ResumeSeedFromCtx(ctx context.Context) ([]agent.Message, int, bool) {
	return resumeSeedFromCtx(ctx)
}

// DisableHeuristicBypass reports whether THIS run's effective
// config disables the deterministic heuristic bypass (time/date
// fast paths). Wraps the effectiveConfig field the Runner needs
// without leaking the full Config type across the package
// boundary (Day 34).
func (k *Kernel) DisableHeuristicBypass(ctx context.Context) bool {
	return k.effectiveConfig(ctx).DisableHeuristicBypass
}

// BuildRunPrompt assembles the system prompt (live default +
// profile/taste/memory/world/skill injection) for a run. Day 34
// wrapper for the runexec sub-package.
func (k *Kernel) BuildRunPrompt(runCtx context.Context, corr, actor, intent string, systemAgent bool, skillDirective skill.ActivationDirective) (string, []string) {
	return k.buildRunPrompt(runCtx, corr, actor, intent, systemAgent, skillDirective)
}

// InjectHostEnvironment prepends the host-environment preamble to
// the system prompt when Config.EnvironmentInject is set. Day 34
// wrapper for the runexec sub-package.
func (k *Kernel) InjectHostEnvironment(system string, tools map[string]agent.Tool) string {
	return k.injectHostEnvironment(system, tools)
}

// ResumeCheckpointFn returns the per-iteration checkpoint callback
// for a resumed run (M1002). Wraps the package-level resume.go
// helper so the runexec sub-package can drive it through KernelAPI.
func (k *Kernel) ResumeCheckpointFn(corr string) func(int, []agent.Message) {
	return k.resumeCheckpointFn(corr)
}

// ResolveRunModel picks the per-request model from the run ctx
// (explicit > daemon default with agent-profile overrides applied).
// Runs k.effectiveConfig internally so live daemon-default swaps
// (M816) + per-agent overrides (M783) are reflected without the
// Runner needing the Config type across the package boundary
// (Day 34).
func (k *Kernel) ResolveRunModel(ctx context.Context) (string, bool) {
	if m := modelFromCtx(ctx); m != "" {
		return m, true
	}
	return k.effectiveConfig(ctx).Model, false
}

// MergeAutoApproveCapabilities merges a per-run auto-approve list
// with the existing ctx-stashed one. Wraps the package-level helper.
func (k *Kernel) MergeAutoApproveCapabilities(ctx context.Context, from map[string]bool) map[string]bool {
	return mergeAutoApproveCapabilities(ctx, from)
}

// WithActorCorrelation stamps the actor + correlation IDs onto a
// context for downstream consumers (agent.PolicyHook, the
// in-process memory tool). Wraps the ctxKeyActor + ctxKeyCorrelation
// writes so the runexec sub-package can decorate a runCtx
// without importing the runtime ctx key types (Day 34).
func (k *Kernel) WithActorCorrelation(ctx context.Context, actor, corr string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyActor, actor)
	ctx = context.WithValue(ctx, ctxKeyCorrelation, corr)
	return ctx
}

// ActorFromCtx returns the actor name stamped by WithActorCorrelation.
func (k *Kernel) ActorFromCtx(ctx context.Context) string {
	s, _ := ctx.Value(ctxKeyActor).(string)
	return s
}

// ShadowEval reports whether post-run shadow-skill judgement is
// enabled (SPEC-05 §5.2). When true, the run engine evaluates
// the shadow skills relevant to a completed run against what
// actually happened — opt-in, best-effort, no side effects (see
// MaybeShadowEval).
func (k *Kernel) ShadowEval() bool { return k.cfg.ShadowEval }

// VisionModel returns the injected vision-capable model resolver
// (or nil if not configured). Used by DescribeImages (M821) to
// caption images for a run whose active model can't see them.
// Returns nil when the daemon did not wire one — the vision
// sidecar is then disabled.
func (k *Kernel) VisionModel() func() (string, bool) { return k.cfg.VisionModel }

// StandingList returns the current standing orders (read-only
// snapshot). Read-only — callers must not mutate the slice. The
// run engine and the control plane's `agt standing list` both
// use this; nil is fine when no standing store is configured.
func (k *Kernel) StandingList() []standing.Order { return k.standing.List() }

// SkillStore returns the on-disk skill repository. The run
// engine reads skills from here for injection; the skill forge
// writes DRAFT skills here. Nil when skills are not enabled.
func (k *Kernel) SkillStore() *skill.FileStore { return k.skillDir }

// AgentSlugFromCtx returns the named agent's slug attached to a
// run context by WithAgentProfile (or "" when unset). Wraps the
// package-level agentSlugFromCtx so the run engine can reach it
// through the runexec.KernelAPI interface (Day 23).
func (k *Kernel) AgentSlugFromCtx(ctx context.Context) string { return agentSlugFromCtx(ctx) }

// AgentRetryPolicyFromCtx returns the named agent's retry policy
// attached to a run context by WithAgentProfile (or zero + false
// when unset). Wraps the package-level agentRetryPolicyFromCtx
// for the same reason as AgentSlugFromCtx (Day 23).
func (k *Kernel) AgentRetryPolicyFromCtx(ctx context.Context) (roster.RetryPolicy, bool) { return agentRetryPolicyFromCtx(ctx) }

// BaseDir returns the kernel's base directory — the root under
// which journal/, state/, runtime/, catalog/, and vault data
// live. Used by `agt config show` to surface the resolved data
// directory to operators (which can differ from $AGEZT_HOME
// when the daemon was launched with a custom path).
func (k *Kernel) BaseDir() string { return k.cfg.BaseDir }

// ConfigCenter returns the Config Center instance, or nil if not configured.
func (k *Kernel) ConfigCenter() *configcenter.Center { return k.configCenter }

// Model returns the live default model name. Empty when the daemon uses
// provider defaults rather than an override. Seeded from cfg.Model at Open and
// hot-swapped via SetModel when the provider is reloaded (M816), so it must be
// mu-guarded like the default identity. Used by `agt config show` and every run that
// builds a CompletionRequest without an explicit per-run/per-task model.
func (k *Kernel) Model() string {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.model
}

// SetModel replaces the live default model id. The next run picks it up — no
// restart. Paired with SetSystem-style persistence: the daemon's provider
// reload calls this after AGEZT_MODEL changes so a wizard/Config-Center edit
// takes effect in place instead of waiting for the next boot (M816).
func (k *Kernel) SetModel(m string) {
	k.configMu.Lock()
	k.model = m
	k.configMu.Unlock()
}

// SetCouncilMembers replaces the live default Council of Elders membership
// (M839). The next council convening picks it up — no restart. Paired with
// persistence: handleCouncilSet writes AGEZT_COUNCIL_MEMBERS to the settings
// store, then calls this so the kernel picks up the new membership immediately.
func (k *Kernel) SetCouncilMembers(members func() []CouncilMember) {
	k.configMu.Lock()
	k.cfg.CouncilMembers = members
	k.configMu.Unlock()
}

// CouncilMembers is the read side of the Council of Elders membership
// (M839). Exposed so the Accessor sub-package can mirror the lock
// order on SetCouncilMembers. Day 18a.
func (k *Kernel) CouncilMembers() func() []CouncilMember {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.cfg.CouncilMembers
}

// MaxDuration is the daemon-wide per-run wall-clock budget (M31), 0 if disabled.
// Exposed so the control plane can report the effective timeout in `agt run
// --dry-run` (M159) without reaching into the config.
func (k *Kernel) MaxDuration() time.Duration { return k.cfg.MaxDuration }

// SubAgentLimits is a type alias for the shared value type (Day 17).
type SubAgentLimits = types.SubAgentLimits

// SubAgentLimits returns the effective delegation ceilings (M49).
func (k *Kernel) SubAgentLimits() SubAgentLimits {
	l := SubAgentLimits{
		Enabled:            k.cfg.SubAgentTool,
		MaxDepth:           k.cfg.SubAgentMaxDepth,
		MaxFanout:          k.cfg.SubAgentMaxFanout,
		MaxSpendMicrocents: k.cfg.SubAgentMaxSpendMicrocents,
		MaxTotal:           k.cfg.SubAgentMaxTotal,
	}
	if l.Enabled && l.MaxDepth <= 0 {
		l.MaxDepth = 1 // effective default, matching runSubAgent
	}
	return l
}

// System returns the live daemon default identity prompt. Empty when none is set.
// Seeded from cfg.System at Open and editable at runtime via SetSystem (M710).
// `agt config show` uses it only to report PRESENCE, not content (which could
// carry proprietary instructions); the dedicated default-identity surface returns
// the content for the owner to edit.
func (k *Kernel) System() string {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.system
}

// SetSystem replaces the live daemon default identity prompt. The next default
// run picks it up — no restart. Persistence (so it survives a restart) is the
// control plane's job: it writes AGEZT_SYSTEM_PROMPT to the config store
// alongside this.
func (k *Kernel) SetSystem(s string) {
	k.configMu.Lock()
	k.system = s
	k.configMu.Unlock()
}

// Catalog returns the currently-loaded provider/model catalog. The
// returned pointer is the live snapshot; callers should treat it as
// read-only and re-call after ReloadCatalog if they need fresh data.
func (k *Kernel) Catalog() *catalog.Catalog {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.catalog
}

// CatalogStore returns the on-disk store backing the catalog, so the
// control plane can drive `agt catalog sync` writes.
func (k *Kernel) CatalogStore() *catalog.Store { return k.catalogStore }

// Reload refreshes both the catalog snapshot AND the live provider
// registry. Catalog reload is always performed; provider rebuild runs
// when Config.OnReload is non-nil. This is the operator-facing hot-
// reload entry point invoked by the control plane's `provider.reload`
// command (and, by extension, `agt provider reload`).
//
// Returns (catalog, providersReloaded, err). providersReloaded is
// true when OnReload ran successfully; false when OnReload was nil
// (catalog-only reload) or returned an error.
func (k *Kernel) Reload() (*catalog.Catalog, bool, error) {
	cat, err := k.ReloadCatalog()
	if err != nil {
		return nil, false, err
	}
	if k.cfg.OnReload == nil {
		return cat, false, nil
	}
	if err := k.cfg.OnReload(); err != nil {
		return cat, false, fmt.Errorf("runtime: provider reload: %w", err)
	}
	return cat, true, nil
}

// ReloadCatalog re-reads catalog files from disk and re-installs the
// snapshot into the Governor. Called after `agt catalog sync` and
// after Ollama discovery completes so live pricing reflects the new
// data immediately.
func (k *Kernel) ReloadCatalog() (*catalog.Catalog, error) {
	cat, err := k.catalogStore.Load()
	if err != nil {
		return nil, err
	}
	k.configMu.Lock()
	k.catalog = cat
	k.configMu.Unlock()
	governor.SetCatalog(cat)
	return cat, nil
}

// LoopRunner returns a closure suitable for scheduler.LoopNode.Runner.
// The closure drives one agent.Run end-to-end via the kernel's
// configured Provider/Tools/Policy hook, using the plan-derived
// correlation ID so events stay linked under `agt why`.
func (k *Kernel) LoopRunner() scheduler.LoopRunner {
	return func(ctx context.Context, intent, corr string) (string, error) {
		return k.RunWith(ctx, corr, intent)
	}
}

// RunPlan executes a pre-built Plan through the kernel's scheduler.
// Honors Halt: refuses to start when halted; in-flight nodes are
// cancelled when Halt is called mid-plan. PlanID is the correlation
// ID for the whole plan; if empty, the scheduler mints one.
func (k *Kernel) RunPlan(ctx context.Context, plan scheduler.Plan, planID string) (*scheduler.PlanResult, error) {
	k.runsMu.Lock()
	if k.halted {
		k.runsMu.Unlock()
		return nil, ErrHalted
	}
	if planID == "" {
		planID = "plan-" + ulid.New()
	}
	runCtx, cancel := context.WithCancel(ctx)
	k.runs[planID] = cancel
	k.runsMu.Unlock()

	defer func() {
		k.runsMu.Lock()
		delete(k.runs, planID)
		k.runsMu.Unlock()
		cancel()
	}()

	return k.scheduler.Run(runCtx, plan, planID)
}
