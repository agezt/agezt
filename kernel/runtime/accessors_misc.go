// SPDX-License-Identifier: MIT

package runtime

// Provenance: Kernel accessors: misc
//             (ShadowEval/VisionModel/StandingList/SkillStore/BaseDir/ConfigCenter/Model/SetModel)
//             + council + catalog + reload + the plan runner's face. Code
//             extracted from accessors.go during the Day-56 god-file split.
//             The AgentSlug and AgentRetryPolicy doors went with W3.76m: the
//             run context's vocabulary is the runs module's, and the run
//             engine — which lives inside that module — reads it there. The
//             plan runner's own verbs (RunPlan, LoopRunner) went with
//             W3.76ak: the plan runner is the runs module's own service, and
//             the daemon hands its face over.

import (
	"errors"
	"fmt"
	"time"

	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/contract/catalogapi"
	"github.com/agezt/agezt/kernel/contract/types"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/modules/runs/api"
)

// VisionModel returns the injected vision-capable model resolver
// (or nil if not configured). Used by DescribeImages (M821) to
// caption images for a run whose active model can't see them.
// Returns nil when the daemon did not wire one — the vision
// sidecar is then disabled.
func (k *Kernel) VisionModel() func() (string, bool) { return k.cfg.VisionModel }

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

// errNoCatalogLibrary is the answer the kernel gives when the host holds no
// catalog library.
//
// The catalog directory is the DAEMON's file (W3.51): the kernel holds the
// contract's store port, and a host that was given none holds a nil port. The
// refusal is the answer — not a panic, and not a silent "nothing to do".
var errNoCatalogLibrary = errors.New("runtime: the catalog library is not available on this daemon")

// Catalog returns the currently-loaded provider/model catalog. The
// returned pointer is the live snapshot; callers should treat it as
// read-only and re-call after ReloadCatalog if they need fresh data.
//
// The row is the CONTRACT's (W3.51): the kernel may not name the catalog
// module. It is never nil — a daemon that has no catalog (or has never
// synced) holds the EMPTY catalog, which is exactly what it knows.
func (k *Kernel) Catalog() *catalogapi.Catalog {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.catalog
}

// CatalogStore returns the on-disk store backing the catalog, so the
// control plane can drive `agt catalog sync` writes.
//
// It is the contract's STORE port (W3.51) and it MAY be nil: the catalog
// directory is the daemon's file, and a host that was given no store holds
// a nil port. The surfaces that write into it refuse instead of writing
// into nothing.
func (k *Kernel) CatalogStore() catalogapi.Store { return k.catalogStore }

// CatalogSyncer returns the port that fetches a remote catalog.
//
// It is the contract's SYNCER port (W3.51) and it MAY be nil: the daemon
// builds the syncer and hands it over. The sync surfaces refuse instead of
// fetching nothing.
func (k *Kernel) CatalogSyncer() catalogapi.Syncer { return k.catalogSync }

// Reload refreshes both the catalog snapshot AND the live provider
// registry. Catalog reload is always performed; provider rebuild runs
// when Config.OnReload is non-nil. This is the operator-facing hot-
// reload entry point invoked by the control plane's `provider.reload`
// command (and, by extension, `agt provider reload`).
//
// Returns (catalog, providersReloaded, err). providersReloaded is
// true when OnReload ran successfully; false when OnReload was nil
// (catalog-only reload) or returned an error.
func (k *Kernel) Reload() (*catalogapi.Catalog, bool, error) {
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
//
// The catalog directory is the DAEMON's file (W3.51): the kernel holds the
// contract's store port, and a host that was given none has nothing to
// reload from — the refusal is the answer, not a panic.
func (k *Kernel) ReloadCatalog() (*catalogapi.Catalog, error) {
	if k.catalogStore == nil {
		return nil, errNoCatalogLibrary
	}
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

// PlanService returns the RUNS MODULE's plan runner (W3.76ak,
// kernel/modules/runs/plan): the plan's execution and its registration as a
// live run.
//
// The daemon used to answer for the family in its own words — `RunPlan`
// executed the plan and `LoopRunner` handed the scheduler the hook that runs
// a node's loop — while the machinery behind both was already the module's:
// the SCHEDULER (the runs module's family, per the architecture gate) and the
// module's own live-run REGISTRY. The daemon held a second handle to the
// module's machinery and spoke for it; the module answers in its own names
// now, and the daemon hands the face over.
func (k *Kernel) PlanService() api.PlanService { return k.plan }
