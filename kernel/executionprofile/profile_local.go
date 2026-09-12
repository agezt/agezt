// SPDX-License-Identifier: MIT

// Execution profile local: wardenProfile + worktreeCodingProfile + browserSessionProfile + dockerProfile.
// Code extracted from profile.go during the Day-71 god-file split. Public API unchanged.
package executionprofile


import (
	"github.com/agezt/agezt/kernel/warden"
)


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
