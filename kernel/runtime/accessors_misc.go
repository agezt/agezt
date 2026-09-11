// SPDX-License-Identifier: MIT

// Kernel accessors: misc (ShadowEval/VisionModel/StandingList/SkillStore/AgentSlug/AgentRetryPolicy/BaseDir/ConfigCenter/Model/SetModel) + council + catalog + reload + loop/plan.
// Code extracted from accessors.go during the Day-56 god-file split. Public API unchanged.
package runtime


import (
	"context"
	"fmt"
	"time"

	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime/types"
	"github.com/agezt/agezt/kernel/scheduler"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/kernel/ulid"
)



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
