// SPDX-License-Identifier: MIT

// Execution profile remote: sshProfile + remoteAgeztProfile + modalProfile + daytonaProfile + k8sProfile.
// Code extracted from profile.go during the Day-71 god-file split. Public API unchanged.
package executionprofile





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
