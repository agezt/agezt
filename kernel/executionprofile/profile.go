// SPDX-License-Identifier: MIT

// Package executionprofile names the runtime execution surfaces AGEZT can use
// for high-risk work. It does not launch work by itself; it is the shared
// inventory contract for CLI/API/UI so operators can see requested vs effective
// isolation before tool routing grows a selectable profile knob.
package executionprofile

import (
	"runtime"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/warden"
)

type Status string

const (
	StatusSupported Status = "supported"
	StatusDegraded  Status = "degraded"
	StatusPartial   Status = "partial"
	StatusPlanned   Status = "planned"
)

const (
	IDLocal  = "local"
	IDWarden = "warden"
)

type Profile struct {
	ID                 string        `json:"id"`
	Name               string        `json:"name"`
	Summary            string        `json:"summary"`
	Status             Status        `json:"status"`
	Routed             bool          `json:"routed"`
	RequestedIsolation string        `json:"requested_isolation"`
	EffectiveIsolation string        `json:"effective_isolation"`
	Degraded           bool          `json:"degraded"`
	DegradeReason      string        `json:"degrade_reason,omitempty"`
	Tools              []string      `json:"tools"`
	Backends           []string      `json:"backends"`
	FileSystem         string        `json:"filesystem"`
	Network            string        `json:"network"`
	Environment        string        `json:"environment"`
	Secrets            string        `json:"secrets"`
	SecretPolicy       *SecretPolicy `json:"secret_policy,omitempty"`
	Limits             []string      `json:"limits"`
	BrowserAccess      string        `json:"browser_access"`
	Cleanup            string        `json:"cleanup"`
	PolicyCapability   string        `json:"policy_capability,omitempty"`
	Notes              []string      `json:"notes,omitempty"`
}

type Inventory struct {
	HostOS         string    `json:"host_os"`
	HostArch       string    `json:"host_arch"`
	Profiles       []Profile `json:"profiles"`
	Count          int       `json:"count"`
	RoutedCount    int       `json:"routed_count"`
	SupportedCount int       `json:"supported_count"`
	DegradedCount  int       `json:"degraded_count"`
}

type Options struct {
	Tools            []string
	Warden           warden.Engine
	EffectiveProfile func(warden.Profile) warden.Profile
	SSH              SSHConfig
	K8s              K8sConfig
	Modal            ModalConfig
	Daytona          DaytonaConfig
	HostOS           string
	HostArch         string
}

func Build(opts Options) Inventory {
	hostOS := strings.TrimSpace(opts.HostOS)
	if hostOS == "" {
		hostOS = runtime.GOOS
	}
	hostArch := strings.TrimSpace(opts.HostArch)
	if hostArch == "" {
		hostArch = runtime.GOARCH
	}
	tools := toolSet(opts.Tools)
	effective := opts.EffectiveProfile
	if effective == nil && opts.Warden != nil {
		effective = opts.Warden.EffectiveProfile
	}
	if effective == nil {
		w := warden.New(nil)
		effective = w.EffectiveProfile
	}

	profiles := []Profile{
		localProfile(tools),
		wardenProfile(tools, effective),
		worktreeCodingProfile(tools),
		browserSessionProfile(tools),
		dockerProfile(tools, effective),
		sshProfile(tools, opts.SSH),
		remoteAgeztProfile(tools),
		modalProfile(tools, opts.Modal),
		daytonaProfile(tools, opts.Daytona),
		k8sProfile(tools, opts.K8s),
	}
	inv := Inventory{HostOS: hostOS, HostArch: hostArch, Profiles: profiles, Count: len(profiles)}
	for _, p := range profiles {
		if p.Routed {
			inv.RoutedCount++
		}
		if p.Status == StatusSupported {
			inv.SupportedCount++
		}
		if p.Degraded {
			inv.DegradedCount++
		}
	}
	return inv
}

func (i Inventory) Find(id string) (Profile, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, p := range i.Profiles {
		if strings.EqualFold(p.ID, id) {
			return p, true
		}
	}
	return Profile{}, false
}

// WardenProfileForRun resolves execution profiles wired into warden-backed
// tools for a single agent run. Some profiles are conditional: docker maps to
// ProfileContainer, but the control plane must still verify the active warden
// backend can satisfy it before accepting the run.
func WardenProfileForRun(id string) (warden.Profile, bool) {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case IDLocal:
		return warden.ProfileNone, true
	case IDWarden:
		return warden.ProfileNamespace, true
	case "docker":
		return warden.ProfileContainer, true
	default:
		return "", false
	}
}

func RoutableRunProfileIDs() []string {
	return []string{IDLocal, IDWarden}
}

func RoutableRunProfileIDsFor(inv Inventory) []string {
	ids := RoutableRunProfileIDs()
	if p, ok := inv.Find("docker"); ok && p.Routed && !p.Degraded && p.EffectiveIsolation == string(warden.ProfileContainer) {
		ids = append(ids, "docker")
	}
	if p, ok := inv.Find("ssh"); ok && p.Routed && !p.Degraded {
		ids = append(ids, "ssh")
	}
	if p, ok := inv.Find("remote-agezt"); ok && p.Routed && !p.Degraded {
		ids = append(ids, "remote-agezt")
	}
	if p, ok := inv.Find("modal"); ok && p.Routed && !p.Degraded {
		ids = append(ids, "modal")
	}
	if p, ok := inv.Find("daytona"); ok && p.Routed && !p.Degraded {
		ids = append(ids, "daytona")
	}
	if p, ok := inv.Find("k8s"); ok && p.Routed && !p.Degraded {
		ids = append(ids, "k8s")
	}
	return ids
}

func localProfile(tools map[string]bool) Profile {
	return Profile{
		ID:                 IDLocal,
		Name:               "Local host",
		Summary:            "Direct host execution with AGEZT policy/audit but no process isolation.",
		Status:             StatusSupported,
		Routed:             anyTool(tools, "file", "shell", "browser", "http", "websearch"),
		RequestedIsolation: string(warden.ProfileNone),
		EffectiveIsolation: string(warden.ProfileNone),
		Tools:              presentTools(tools, "file", "shell", "browser", "http", "websearch"),
		Backends:           []string{"host process", "workspace filesystem"},
		FileSystem:         "configured workspace and host paths allowed by each tool",
		Network:            "host network, guarded per tool by netguard where applicable",
		Environment:        ProfileEnvironmentSummary(IDLocal, "tool-specific scrubbed environment for child processes"),
		Secrets:            ProfileSecretFileSummary(IDLocal, ProfileSecretsSummary(IDLocal, "vault secrets are not forwarded unless a tool explicitly resolves them")),
		Limits:             []string{"tool timeout", "output caps"},
		BrowserAccess:      "available when browser tools are registered",
		Cleanup:            "caller/tool-specific",
		PolicyCapability:   "tool-specific Edict capability",
	}
}

func wardenProfile(tools map[string]bool, effective func(warden.Profile) warden.Profile) Profile {
	req := warden.ProfileNamespace
	eff := effective(req)
	degraded := eff != req
	status := StatusSupported
	reason := ""
	if degraded {
		status = StatusDegraded
		reason = "requested namespace isolation is downgraded by the active warden backend on this host"
	}
	return Profile{
		ID:                 IDWarden,
		Name:               "Warden namespace",
		Summary:            "Shell/code execution through the warden engine with honest downgrade reporting.",
		Status:             status,
		Routed:             anyTool(tools, "shell", "code_exec", "tool_forge"),
		RequestedIsolation: string(req),
		EffectiveIsolation: string(eff),
		Degraded:           degraded,
		DegradeReason:      reason,
		Tools:              presentTools(tools, "shell", "code_exec", "tool_forge"),
		Backends:           []string{"kernel/warden"},
		FileSystem:         "workspace or per-call sandbox directory",
		Network:            "host network unless the calling tool disables or guards it",
		Environment:        ProfileEnvironmentSummary(IDWarden, "scrubbed child process environment"),
		Secrets:            ProfileSecretFileSummary(IDWarden, ProfileSecretsSummary(IDWarden, "daemon environment is not inherited by default")),
		Limits:             []string{"wall-clock timeout", "output cap", "best-effort CPU/memory/fd/file-size limits on linux namespace"},
		BrowserAccess:      "none",
		Cleanup:            "ephemeral code_exec scratch directories are removed; project dirs persist",
		PolicyCapability:   "shell / code.exec / tool-specific Edict capability",
		Notes:              []string{"warden.executed journals requested and effective profile for every run"},
	}
}

func worktreeCodingProfile(tools map[string]bool) Profile {
	routed := anyTool(tools, "coding", "acp_agent")
	status := StatusPlanned
	if routed {
		status = StatusSupported
	}
	return Profile{
		ID:                 "worktree-coding",
		Name:               "Worktree coding",
		Summary:            "Delegated coding agents operate in isolated git worktrees with explicit merge/review flow.",
		Status:             status,
		Routed:             routed,
		RequestedIsolation: "worktree",
		EffectiveIsolation: "worktree",
		Tools:              presentTools(tools, "coding", "acp_agent"),
		Backends:           []string{"git worktree", "external coding agent"},
		FileSystem:         "separate worktree rooted under the configured workspace",
		Network:            "external agent host policy",
		Environment:        "external agent environment",
		Secrets:            "no daemon vault export by default",
		Limits:             []string{"task timeout if configured", "git diff review boundary"},
		BrowserAccess:      "external agent dependent",
		Cleanup:            "worktree retained until review/merge/removal",
		PolicyCapability:   "coding / acp_agent",
	}
}

func browserSessionProfile(tools map[string]bool) Profile {
	routed := anyTool(tools, "browser")
	status := StatusPlanned
	if routed {
		status = StatusSupported
	}
	return Profile{
		ID:                 "browser-session",
		Name:               "Browser session",
		Summary:            "Browser automation uses named browser profiles and records screenshots/download artifacts.",
		Status:             status,
		Routed:             routed,
		RequestedIsolation: "browser-profile",
		EffectiveIsolation: "browser-profile",
		Tools:              presentTools(tools, "browser"),
		Backends:           []string{"browseruse driver", "Playwright when available"},
		FileSystem:         "browser profile storage plus artifact store",
		Network:            "browser network, gated by browser profile policy",
		Environment:        "browser process environment",
		Secrets:            "cookies/session storage scoped to selected browser profile",
		Limits:             []string{"action timeout", "artifact capture bounds"},
		BrowserAccess:      "isolated/session/user-attached/remote-cdp profile policy",
		Cleanup:            "profile-specific",
		PolicyCapability:   "browser.read / browser.action",
	}
}

func dockerProfile(tools map[string]bool, effective func(warden.Profile) warden.Profile) Profile {
	eff := effective(warden.ProfileContainer)
	routed := eff == warden.ProfileContainer && anyTool(tools, "shell", "code_exec", "tool_forge")
	status := StatusPlanned
	degraded := true
	reason := "container execution is available only through skills/manual flows until tool routing accepts execution profiles"
	effectiveIsolation := "not_routed"
	profileTools := []string(nil)
	policyCapability := "future execution.profile.docker"
	notes := []string{"set AGEZT_WARDEN_DOCKER=1 to opt into the Docker/Podman warden backend"}
	if routed {
		status = StatusSupported
		degraded = false
		reason = ""
		effectiveIsolation = string(warden.ProfileContainer)
		profileTools = presentTools(tools, "shell", "code_exec", "tool_forge")
		policyCapability = "shell / code.exec / tool-specific Edict capability"
		notes = []string{"default image is python:3.12-slim; set AGEZT_WARDEN_DOCKER_IMAGE for Node/Deno/custom runtimes"}
	} else if eff != "not_routed" && eff != warden.ProfileContainer {
		effectiveIsolation = string(eff)
		reason = "container execution is not enabled; active warden backend would downgrade container requests"
	}
	return Profile{
		ID:                 "docker",
		Name:               "Docker/OCI",
		Summary:            "Container-backed shell/code execution through the optional Docker/Podman warden backend.",
		Status:             status,
		Routed:             routed,
		RequestedIsolation: string(warden.ProfileContainer),
		EffectiveIsolation: effectiveIsolation,
		Degraded:           degraded,
		DegradeReason:      reason,
		Tools:              profileTools,
		Backends:           []string{"Docker or Podman"},
		FileSystem:         "current tool workdir mounted at /workspace",
		Network:            "Docker network mode from AGEZT_WARDEN_DOCKER_NETWORK, default none",
		Environment:        ProfileEnvironmentSummary("docker", "declared env passthrough only"),
		Secrets:            ProfileSecretFileSummary("docker", ProfileSecretsSummary("docker", "declared secret env passthrough only")),
		Limits:             []string{"warden wall-clock timeout", "output cap", "optional Docker --memory from address-space limit"},
		BrowserAccess:      "optional browser sidecar",
		Cleanup:            "container/image cleanup policy",
		PolicyCapability:   policyCapability,
		Notes:              notes,
	}
}

func sshProfile(tools map[string]bool, cfg SSHConfig) Profile {
	routed := cfg.Active() && anyTool(tools, "shell", "code_exec")
	status := StatusPlanned
	degraded := true
	reason := "SSH exists as a skill/manual integration, not yet as a tool execution profile"
	effectiveIsolation := "not_routed"
	profileTools := []string(nil)
	policyCapability := "future execution.profile.ssh"
	notes := []string{"set AGEZT_EXEC_SSH=1 and AGEZT_EXEC_SSH_TARGET=user@host to opt into SSH shell/code_exec routing"}
	if routed {
		status = StatusPartial
		degraded = false
		reason = ""
		effectiveIsolation = "remote-host"
		profileTools = presentTools(tools, "shell", "code_exec")
		policyCapability = "shell / code.exec"
		notes = []string{"shell routes through ssh; code_exec syncs its workspace with scp before running on the remote host"}
	}
	return Profile{
		ID:                 "ssh",
		Name:               "SSH remote",
		Summary:            "Shell-only remote execution over the system ssh client.",
		Status:             status,
		Routed:             routed,
		RequestedIsolation: "remote-host",
		EffectiveIsolation: effectiveIsolation,
		Degraded:           degraded,
		DegradeReason:      reason,
		Tools:              profileTools,
		Backends:           []string{"ssh"},
		FileSystem:         "remote working directory from AGEZT_EXEC_SSH_WORKDIR, or the remote login directory",
		Network:            "remote host network",
		Environment:        "no daemon env forwarding; remote shell environment only",
		Secrets:            "SSH key/agent handled by the local ssh client; daemon secrets are not forwarded",
		Limits:             []string{"local ssh process timeout", "output cap"},
		BrowserAccess:      "remote dependent",
		Cleanup:            "remote dependent",
		PolicyCapability:   policyCapability,
		Notes:              notes,
	}
}

func remoteAgeztProfile(tools map[string]bool) Profile {
	routed := anyTool(tools, "remote_run", "peer")
	status := StatusPlanned
	if routed {
		status = StatusSupported
	}
	secretPolicy := RemoteSecretPolicyFromEnv()
	return Profile{
		ID:                 "remote-agezt",
		Name:               "Remote AGEZT peer",
		Summary:            "Whole-run delegation to another AGEZT daemon through the remote_run peer tool.",
		Status:             status,
		Routed:             routed,
		RequestedIsolation: "remote-agezt",
		EffectiveIsolation: "remote-agezt",
		Tools:              presentTools(tools, "remote_run", "peer"),
		Backends:           []string{"AGEZT peer mesh"},
		FileSystem:         "remote daemon workspace",
		Network:            "peer route policy",
		Environment:        "remote daemon configuration",
		Secrets:            RemoteSecretPolicySummary("remote daemon vault"),
		SecretPolicy:       &secretPolicy,
		Limits:             []string{"peer timeout", "remote daemon policy"},
		BrowserAccess:      "remote daemon dependent",
		Cleanup:            "remote daemon dependent",
		PolicyCapability:   "remote_run",
		Notes:              []string{"the delegating node policy-gates remote_run; the peer enforces its own run policy and journal"},
	}
}

func modalProfile(tools map[string]bool, cfg ModalConfig) Profile {
	secretPolicy := RemoteSecretPolicyFromEnv()
	routed := cfg.Active() && anyTool(tools, "shell", "code_exec")
	status := StatusPlanned
	degraded := true
	reason := "Modal exists as a planned cloud profile until AGEZT_EXEC_MODAL is enabled"
	effectiveIsolation := "not_routed"
	profileTools := []string(nil)
	policyCapability := "future execution.profile.modal"
	notes := []string{"set AGEZT_EXEC_MODAL=1 to opt into modal shell/code_exec routing; optional REF/IMAGE/ENVIRONMENT select the Modal runtime"}
	if routed {
		status = StatusPartial
		degraded = false
		reason = ""
		effectiveIsolation = "modal-shell"
		profileTools = presentTools(tools, "shell", "code_exec")
		policyCapability = "shell / code.exec"
		notes = []string{"shell routes through `modal shell --cmd`; code_exec mounts its generated workspace with `modal shell --add-local` and copies back .agezt-artifacts through a bounded archive"}
	}
	return Profile{
		ID:                 "modal",
		Name:               "Modal cloud sandbox",
		Summary:            "Modal-backed shell/code execution through the Modal CLI.",
		Status:             status,
		Routed:             routed,
		RequestedIsolation: "modal",
		EffectiveIsolation: effectiveIsolation,
		Degraded:           degraded,
		DegradeReason:      reason,
		Tools:              profileTools,
		Backends:           []string{"modal"},
		FileSystem:         "Modal container filesystem; code_exec workspace is mounted with --add-local and .agezt-artifacts is copied back through stdout archive",
		Network:            "Modal cloud egress policy",
		Environment:        "local daemon env is not forwarded; modal CLI uses local profile/config",
		Secrets:            RemoteSecretPolicySummary("Modal-managed secrets"),
		SecretPolicy:       &secretPolicy,
		Limits:             []string{"local modal process timeout", "output cap", "bounded artifact archive cap", "Modal runtime quotas"},
		BrowserAccess:      "adapter dependent",
		Cleanup:            "Modal shell lifecycle",
		PolicyCapability:   policyCapability,
		Notes:              notes,
	}
}

func daytonaProfile(tools map[string]bool, cfg DaytonaConfig) Profile {
	secretPolicy := RemoteSecretPolicyFromEnv()
	routed := cfg.Active() && anyTool(tools, "shell", "code_exec")
	status := StatusPlanned
	degraded := true
	reason := "Daytona exists as a planned cloud profile until AGEZT_EXEC_DAYTONA and AGEZT_EXEC_DAYTONA_SANDBOX are configured"
	effectiveIsolation := "not_routed"
	profileTools := []string(nil)
	policyCapability := "future execution.profile.daytona"
	notes := []string{"set AGEZT_EXEC_DAYTONA=1 and AGEZT_EXEC_DAYTONA_SANDBOX=<id-or-name> to opt into Daytona exec shell/code_exec routing"}
	if routed {
		status = StatusPartial
		degraded = false
		reason = ""
		effectiveIsolation = "daytona-sandbox"
		profileTools = presentTools(tools, "shell", "code_exec")
		policyCapability = "shell / code.exec"
		notes = []string{"shell routes through `daytona exec`; code_exec materializes its workspace through bounded `daytona exec` writes and copies back .agezt-artifacts through a bounded archive"}
	}
	return Profile{
		ID:                 "daytona",
		Name:               "Daytona workspace",
		Summary:            "Daytona sandbox shell/code execution through the Daytona CLI.",
		Status:             status,
		Routed:             routed,
		RequestedIsolation: "daytona",
		EffectiveIsolation: effectiveIsolation,
		Degraded:           degraded,
		DegradeReason:      reason,
		Tools:              profileTools,
		Backends:           []string{"daytona"},
		FileSystem:         "existing Daytona sandbox filesystem; optional working directory from AGEZT_EXEC_DAYTONA_WORKDIR",
		Network:            "Daytona sandbox network policy",
		Environment:        "local daemon env is not forwarded; daytona CLI uses local login/config",
		Secrets:            RemoteSecretPolicySummary("Daytona-managed secrets"),
		SecretPolicy:       &secretPolicy,
		Limits:             []string{"local daytona process timeout", "output cap", "Daytona sandbox quotas"},
		BrowserAccess:      "adapter dependent",
		Cleanup:            "existing sandbox lifecycle remains operator controlled",
		PolicyCapability:   policyCapability,
		Notes:              notes,
	}
}

func k8sProfile(tools map[string]bool, cfg K8sConfig) Profile {
	secretPolicy := RemoteSecretPolicyFromEnv()
	routed := cfg.Active() && anyTool(tools, "shell", "code_exec")
	status := StatusPlanned
	degraded := true
	reason := "Kubernetes exists as a planned cloud profile until AGEZT_EXEC_K8S and AGEZT_EXEC_K8S_POD are configured"
	effectiveIsolation := "not_routed"
	profileTools := []string(nil)
	policyCapability := "future execution.profile.k8s"
	notes := []string{"set AGEZT_EXEC_K8S=1 and AGEZT_EXEC_K8S_POD=<pod> to opt into kubectl exec shell/code_exec routing"}
	if routed {
		status = StatusPartial
		degraded = false
		reason = ""
		effectiveIsolation = "kubernetes-pod"
		profileTools = presentTools(tools, "shell", "code_exec")
		policyCapability = "shell / code.exec"
		notes = []string{"shell routes through kubectl exec; code_exec copies its workspace into an existing pod before running there and copies back .agezt-artifacts; job lifecycle is still planned"}
	}
	return Profile{
		ID:                 "k8s",
		Name:               "Kubernetes pod",
		Summary:            "Kubernetes-backed shell/code execution through kubectl exec against a configured existing pod.",
		Status:             status,
		Routed:             routed,
		RequestedIsolation: "k8s",
		EffectiveIsolation: effectiveIsolation,
		Degraded:           degraded,
		DegradeReason:      reason,
		Tools:              profileTools,
		Backends:           []string{"kubectl", "Kubernetes"},
		FileSystem:         "existing pod filesystem; optional working directory from AGEZT_EXEC_K8S_WORKDIR",
		Network:            "pod namespace/network-policy dependent",
		Environment:        "local daemon env is not forwarded; kubectl uses local kubeconfig/context only",
		Secrets:            RemoteSecretPolicySummary("cluster/pod managed secrets"),
		SecretPolicy:       &secretPolicy,
		Limits:             []string{"local kubectl process timeout", "output cap", "cluster pod quotas"},
		BrowserAccess:      "pod/cluster dependent",
		Cleanup:            "existing pod lifecycle remains operator controlled",
		PolicyCapability:   policyCapability,
		Notes:              notes,
	}
}

func plannedCloudProfile(id, name, summary, toolName, backend, filesystem, network, policyCapability string, notes []string, tools map[string]bool) Profile {
	secretPolicy := RemoteSecretPolicyFromEnv()
	return Profile{
		ID:                 id,
		Name:               name,
		Summary:            summary,
		Status:             StatusPlanned,
		Routed:             false,
		RequestedIsolation: id,
		EffectiveIsolation: "not_routed",
		Tools:              presentTools(tools, toolName),
		Backends:           []string{backend},
		FileSystem:         filesystem,
		Network:            network,
		Environment:        "cloud adapter environment; local daemon env is not forwarded by default",
		Secrets:            RemoteSecretPolicySummary("cloud adapter secrets"),
		SecretPolicy:       &secretPolicy,
		Limits:             []string{"cloud provider timeout/quotas", "AGEZT run timeout once adapter is wired"},
		BrowserAccess:      "adapter dependent",
		Cleanup:            "adapter dependent",
		PolicyCapability:   policyCapability,
		Notes:              notes,
	}
}

func toolSet(tools []string) map[string]bool {
	out := make(map[string]bool, len(tools))
	for _, t := range tools {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			out[t] = true
		}
	}
	return out
}

func anyTool(tools map[string]bool, names ...string) bool {
	for _, n := range names {
		if tools[strings.ToLower(n)] {
			return true
		}
	}
	return false
}

func presentTools(tools map[string]bool, names ...string) []string {
	out := []string{}
	for _, n := range names {
		if tools[strings.ToLower(n)] {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}
