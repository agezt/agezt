// SPDX-License-Identifier: MIT

package runs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appexecprofile "github.com/agezt/agezt/kernel/app/execprofile"
	approster "github.com/agezt/agezt/kernel/app/roster"
	"github.com/agezt/agezt/kernel/executionprofile"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/warden"
)

// resolvedRun is a run after the request has been resolved against the kernel:
// the model it will use, the agent it runs as, the tools it may call, and the
// context the agent loop will read.
type resolvedRun struct {
	ctx        context.Context
	intent     string
	tenant     string
	agent      *roster.Profile
	model      string // the model this run will actually use
	overridden bool   // a per-run model override was given
	corr       string
	timeout    string // the raw per-run timeout, as validated
	system     string // the per-run system-prompt override, as resolved
	tools      []string
	toolsSet   bool
	maxCost    int64
	profile    string // the requested execution profile id
	warden     string // the warden profile label the run reports
	remote     bool   // the remote-agezt execution profile is in effect
	peer       string
	assure     int64
	dryRun     bool
	images     []string
}

// resolve reads the request against the kernel and prepares the run's context.
// Nothing here executes: a dry run resolves exactly the same way, so the plan
// can never drift from what would run.
func (s *RunService) resolve(ctx context.Context, in RunRequest) (*resolvedRun, error) {
	k := s.ports.Kernel
	if k == nil {
		return nil, errors.New("run kernel unavailable")
	}
	intent := strings.TrimSpace(argText(in.Intent))
	if intent == "" {
		return nil, errors.New("args.intent required")
	}
	tenant, _, err := argString(in.Tenant, "tenant")
	if err != nil {
		return nil, err
	}
	modelRaw, _, err := argString(in.Model, "model")
	if err != nil {
		return nil, err
	}
	modelOverride := strings.TrimSpace(modelRaw)
	agentRaw, _, err := argString(in.Agent, "agent")
	if err != nil {
		return nil, err
	}
	resolved := &resolvedRun{ctx: ctx, intent: intent, tenant: tenant}
	// Run AS a named agent: the profile's soul, model and cost ceiling become
	// this run's defaults. An unknown, retired, paused or managed agent is a
	// usage error — silently running as the default identity would be worse.
	if ref := strings.TrimSpace(agentRaw); ref != "" {
		p, found := k.Roster().Get(ref)
		if !found {
			return nil, errors.New("unknown agent: " + ref)
		}
		if p.Retired {
			return nil, fmt.Errorf("agent %s is retired — revive it first (agt agent revive %s)", p.Slug, p.Slug)
		}
		if !p.Enabled {
			return nil, fmt.Errorf("agent %s is paused (agt agent resume %s)", p.Slug, p.Slug)
		}
		if !p.AllowsDirectCall() {
			return nil, errors.New(approster.ManagedDirectCallError(p, "called"))
		}
		resolved.agent = &p
		// The whole profile binds the run: tool allow/deny, trust ceiling,
		// standing instructions, memory scope, workdir, ledger identity.
		ctx = runtime.WithAgentProfile(ctx, p)
		if modelOverride == "" {
			modelOverride = strings.TrimSpace(p.Model)
		}
		// The fallback chain is rebuilt around the resolved primary: an
		// explicit model, or the daemon default when the profile names none,
		// still takes the front slot.
		if len(p.Fallbacks) > 0 {
			primary := modelOverride
			if primary == "" {
				primary = k.Model()
			}
			ctx = runtime.WithModelChain(ctx, approster.AgentModelChain(primary, p.Fallbacks))
		}
	}
	resolved.ctx = ctx
	resolved.model = k.Model()
	if modelOverride != "" {
		resolved.model = modelOverride
	}
	resolved.overridden = modelOverride != ""

	// The correlation is minted before the vision gate so the sidecar's
	// journaled event links to this run.
	resolved.corr = k.NewCorrelation()

	imageRefs, _, err := argStringList(in.Images, "images")
	if err != nil {
		return nil, err
	}
	admission, err := k.AdmitImages(ctx, resolved.corr, modelOverride, intent, imageRefs)
	if err != nil {
		return nil, err
	}
	resolved.intent = admission.Intent
	resolved.images = admission.Images
	if len(resolved.images) > 0 {
		ctx = runtime.WithImages(ctx, resolved.images)
	}
	// Route this run to the override model; the loop reads it the same way
	// the OpenAI API does.
	if strings.TrimSpace(modelRaw) != "" {
		ctx = runtime.WithModel(ctx, modelOverride)
	}

	// A per-run system-prompt override replaces the base system prompt for
	// this run only; memory/world/skill injection still layers on top.
	sysRaw, _, err := argString(in.System, "system")
	if err != nil {
		return nil, err
	}
	systemOverride := strings.TrimSpace(sysRaw)
	if systemOverride == "" && resolved.agent != nil {
		// The agent's soul, standing instructions and tasks ARE its system
		// prompt.
		systemOverride = strings.TrimSpace(runtime.AgentProfileSystem(*resolved.agent))
	}
	if strings.TrimSpace(sysRaw) != "" {
		ctx = runtime.WithSystem(ctx, systemOverride)
	}
	resolved.system = systemOverride

	// A per-run wall-clock timeout bounds THIS run without a daemon-wide cap.
	timeoutRaw, _, err := argString(in.Timeout, "timeout")
	if err != nil {
		return nil, err
	}
	timeoutRaw = strings.TrimSpace(timeoutRaw)
	if timeoutRaw != "" {
		d, perr := time.ParseDuration(timeoutRaw)
		if perr != nil || d <= 0 {
			return nil, fmt.Errorf("invalid timeout %q (want a positive Go duration like 30s, 2m)", timeoutRaw)
		}
		ctx = runtime.WithRunTimeout(ctx, d)
	}
	resolved.timeout = timeoutRaw

	// A "tools" arg (present, possibly empty) scopes THIS run to the named
	// tools only — an empty list means no tools at all. Absent means
	// unrestricted.
	toolsAllow, toolsSet, err := argStringList(in.Tools, "tools")
	if err != nil {
		return nil, err
	}
	if toolsSet {
		ctx = runtime.WithTools(ctx, toolsAllow)
	}
	resolved.tools, resolved.toolsSet = toolsAllow, toolsSet

	// A per-run cost cap bounds THIS run's cumulative provider spend. A
	// malformed value is a usage error; a non-positive value is uncapped.
	maxCost, _, err := argInt64(in.MaxCost, "max_cost")
	if err != nil {
		return nil, err
	}
	if maxCost <= 0 && resolved.agent != nil {
		maxCost = resolved.agent.MaxCostMc
	}
	if maxCost > 0 {
		ctx = runtime.WithMaxCost(ctx, maxCost)
	}
	resolved.maxCost = maxCost

	// The per-run execution profile selects the requested warden profile for
	// shell/code_exec. Conditional profiles are only accepted when the active
	// backend can satisfy them.
	execProfileRaw, _, err := argString(in.ExecProfile, "execution_profile")
	if err != nil {
		return nil, err
	}
	execProfile := strings.TrimSpace(execProfileRaw)
	// No explicit profile? Fall back to the assigned agent's default isolation
	// surface, so an agent configured to run sandboxed does so on direct runs
	// too — an explicit arg still wins.
	if execProfile == "" && resolved.agent != nil {
		execProfile = strings.TrimSpace(resolved.agent.ExecutionProfile)
	}
	resolved.profile = execProfile
	ctx, label, remote, err := applyExecutionProfile(ctx, k, execProfile)
	if err != nil {
		return nil, err
	}
	resolved.ctx, resolved.warden, resolved.remote = ctx, label, remote

	remotePeerRaw, _, err := argString(in.RemotePeer, "remote_peer")
	if err != nil {
		return nil, err
	}
	remotePeer := strings.TrimSpace(remotePeerRaw)
	if remotePeer != "" && !remote {
		return nil, errors.New(`remote_peer requires execution_profile "remote-agezt"`)
	}
	resolved.peer = remotePeer

	// A session-scoped auto-approve grant lets the loop approve the named
	// capabilities without prompting. It never overrides a hard-deny.
	if caps := parseCapList(in.ApproveCaps); len(caps) > 0 {
		ctx = runtime.WithAutoApproveCapabilities(ctx, caps)
	}
	// A session-scoped "trust this run's content" grant downgrades the
	// prompt-injection guard from blocking to warn FOR THIS RUN.
	if argTruthy(in.Trust) {
		ctx = runtime.WithTrustedObservations(ctx)
	}
	resolved.ctx = ctx

	// An assured run executes, verifies completion and retries with the gap
	// fed back, up to that many attempts.
	assure, _, err := argInt64(in.Assure, "assure")
	if err != nil {
		return nil, err
	}
	resolved.assure = assure

	// A dry run resolves what the run WOULD do without executing it. A
	// mistyped dry_run is rejected rather than silently executing the run.
	dryRun, _, err := argBool(in.DryRun, "dry_run")
	if err != nil {
		return nil, err
	}
	resolved.dryRun = dryRun
	if dryRun {
		return resolved, nil
	}
	if remote && assure > 0 {
		return nil, errors.New(`execution profile "remote-agezt" cannot combine with assure yet; delegate once or run assurance on the peer`)
	}
	return resolved, nil
}

// applyExecutionProfile resolves the requested execution profile: it checks the
// policy, the backend's availability and the tools the profile needs, and
// returns the label the run reports.
func applyExecutionProfile(ctx context.Context, k *runtime.Kernel, execProfile string) (context.Context, string, bool, error) {
	if execProfile == "" {
		return ctx, "", false, nil
	}
	if ok, reason := executionprofile.PolicyFromEnv().Allows(execProfile); !ok {
		return ctx, "", false, fmt.Errorf("execution profile %q is blocked by policy: %s", execProfile, reason)
	}
	switch {
	case strings.EqualFold(execProfile, "ssh"):
		sshCfg := executionprofile.SSHConfigFromEnv()
		if !sshCfg.Active() {
			return ctx, "", false, errors.New(`execution profile "ssh" requires an active SSH backend (set AGEZT_EXEC_SSH=1 and AGEZT_EXEC_SSH_TARGET=user@host)`)
		}
		return executionprofile.WithSSHOverride(ctx, sshCfg), "ssh", false, nil
	case strings.EqualFold(execProfile, "k8s"):
		k8sCfg := executionprofile.K8sConfigFromEnv()
		if !k8sCfg.Active() {
			return ctx, "", false, errors.New(`execution profile "k8s" requires an active Kubernetes backend (set AGEZT_EXEC_K8S=1 and AGEZT_EXEC_K8S_POD=<pod>; optional AGEZT_EXEC_K8S_NAMESPACE/CONTEXT/CONTAINER/WORKDIR)`)
		}
		if err := requireShellTool(k, "k8s"); err != nil {
			return ctx, "", false, err
		}
		return executionprofile.WithK8sOverride(ctx, k8sCfg), "k8s", false, nil
	case strings.EqualFold(execProfile, "modal"):
		modalCfg := executionprofile.ModalConfigFromEnv()
		if !modalCfg.Active() {
			return ctx, "", false, errors.New(`execution profile "modal" requires an active Modal backend (set AGEZT_EXEC_MODAL=1; optional AGEZT_EXEC_MODAL_REF or AGEZT_EXEC_MODAL_IMAGE)`)
		}
		if err := requireShellTool(k, "modal"); err != nil {
			return ctx, "", false, err
		}
		return executionprofile.WithModalOverride(ctx, modalCfg), "modal", false, nil
	case strings.EqualFold(execProfile, "daytona"):
		daytonaCfg := executionprofile.DaytonaConfigFromEnv()
		if !daytonaCfg.Active() {
			return ctx, "", false, errors.New(`execution profile "daytona" requires an active Daytona backend (set AGEZT_EXEC_DAYTONA=1 and AGEZT_EXEC_DAYTONA_SANDBOX=<id-or-name>)`)
		}
		if err := requireShellTool(k, "daytona"); err != nil {
			return ctx, "", false, err
		}
		return executionprofile.WithDaytonaOverride(ctx, daytonaCfg), "daytona", false, nil
	case strings.EqualFold(execProfile, "remote-agezt"):
		if _, ok := k.Tools()["remote_run"]; !ok {
			return ctx, "", false, errors.New(`execution profile "remote-agezt" requires configured AGEZT peers (set AGEZT_PEERS=name=https://host|token so the remote_run tool is registered)`)
		}
		return ctx, "remote-agezt", true, nil
	}
	p, ok := executionprofile.WardenProfileForRun(execProfile)
	if !ok {
		return ctx, "", false, fmt.Errorf("execution profile %q is not routable for run tools yet (supported: %s)",
			execProfile, strings.Join(executionprofile.RoutableRunProfileIDsFor(appexecprofile.Inventory(k)), ", "))
	}
	if p == warden.ProfileContainer && k.Warden().EffectiveProfile(p) != warden.ProfileContainer {
		return ctx, "", false, fmt.Errorf("execution profile %q requires an active container backend (set AGEZT_WARDEN_DOCKER=1 and configure AGEZT_WARDEN_DOCKER_IMAGE/runtime)", execProfile)
	}
	return warden.WithProfileOverride(ctx, p), string(p), false, nil
}

// requireShellTool refuses an execution profile whose backend needs the shell
// or code_exec tool when neither is registered.
func requireShellTool(k *runtime.Kernel, profile string) error {
	if _, ok := k.Tools()["shell"]; !ok {
		if _, ok := k.Tools()["code_exec"]; !ok {
			return fmt.Errorf("execution profile %q requires the shell or code_exec tool to be registered", profile)
		}
	}
	return nil
}
