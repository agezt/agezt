package executionprofile

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

const RemoteArtifactBytesEnv = "AGEZT_REMOTE_ARTIFACT_BYTES"

type CheckStatus string

const (
	CheckOK      CheckStatus = "ok"
	CheckWarning CheckStatus = "warning"
	CheckFail    CheckStatus = "fail"
)

type HealthCheck struct {
	ID               string      `json:"id"`
	ProfileID        string      `json:"profile_id"`
	Status           CheckStatus `json:"status"`
	Title            string      `json:"title"`
	Detail           string      `json:"detail"`
	Next             string      `json:"next,omitempty"`
	Routed           bool        `json:"routed"`
	Degraded         bool        `json:"degraded"`
	BackendAvailable bool        `json:"backend_available,omitempty"`
	Backend          string      `json:"backend,omitempty"`
}

type HealthReport struct {
	HostOS              string        `json:"host_os"`
	HostArch            string        `json:"host_arch"`
	Checks              []HealthCheck `json:"checks"`
	Count               int           `json:"count"`
	OKCount             int           `json:"ok_count"`
	WarningCount        int           `json:"warning_count"`
	FailCount           int           `json:"fail_count"`
	RoutableRunProfiles []string      `json:"routable_run_profiles"`
}

type HealthOptions struct {
	LookPath           func(string) (string, error)
	Policy             ProfilePolicy
	RemoteSecretPolicy SecretPolicy
}

func Diagnose(inv Inventory, opts HealthOptions) HealthReport {
	lookPath := opts.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	report := HealthReport{
		HostOS:              inv.HostOS,
		HostArch:            inv.HostArch,
		RoutableRunProfiles: RoutableRunProfileIDsForPolicy(inv, opts.Policy),
	}
	profiles := map[string]Profile{}
	for _, p := range inv.Profiles {
		profiles[p.ID] = p
	}

	report.add(applyPolicyCheck(profileCheck(profiles, IDLocal, "local runtime profile", "direct host execution tools are routable", "register file/shell/browser/http/websearch tools for local profile coverage"), opts.Policy))
	report.add(applyPolicyCheck(profileCheck(profiles, IDWarden, "warden runtime profile", "shell/code execution can request the warden profile", "register shell/code_exec/tool_forge or fix the warden backend downgrade"), opts.Policy))
	report.add(profileCheck(profiles, "worktree-coding", "worktree coding profile", "coding agents can use isolated worktrees", "register coding or acp_agent before relying on this profile"))
	report.add(profileCheck(profiles, "browser-session", "browser session profile", "browser automation has a named session profile", "register browser tooling and verify browser profile policy"))
	report.add(applyPolicyCheck(commandProfileCheck(profiles, "docker", "Docker/OCI profile", []string{"docker", "podman"}, lookPath), opts.Policy))
	report.add(applyPolicyCheck(commandProfileCheck(profiles, "ssh", "SSH remote profile", []string{"ssh"}, lookPath), opts.Policy))
	report.add(profileCheck(profiles, "remote-agezt", "remote AGEZT profile", "peer routing is available for remote AGEZT work", "register/configure peer routing before using remote AGEZT as an execution profile"))
	report.add(remoteSecretPolicyCheck(remoteSecretPolicyForHealth(opts)))
	report.add(remoteArtifactBytesPolicyCheck(os.Getenv(RemoteArtifactBytesEnv)))
	report.add(commandProfileCheck(profiles, "modal", "Modal cloud profile", []string{"modal"}, lookPath))
	report.add(commandProfileCheck(profiles, "daytona", "Daytona cloud profile", []string{"daytona"}, lookPath))
	report.add(commandProfileCheck(profiles, "k8s", "Kubernetes cloud profile", []string{"kubectl"}, lookPath))
	return report
}

func remoteSecretPolicyForHealth(opts HealthOptions) SecretPolicy {
	if opts.RemoteSecretPolicy.Mode != "" || opts.RemoteSecretPolicy.Detail != "" || opts.RemoteSecretPolicy.Valid {
		return opts.RemoteSecretPolicy
	}
	return RemoteSecretPolicyFromEnv()
}

func applyPolicyCheck(c HealthCheck, policy ProfilePolicy) HealthCheck {
	if policy.Empty() || c.ProfileID == "" {
		return c
	}
	if ok, reason := policy.Allows(c.ProfileID); !ok {
		c.Status = CheckWarning
		c.Detail = "profile is blocked by execution-profile policy: " + reason
		c.Next = "update AGEZT_EXEC_PROFILE_ALLOW / AGEZT_EXEC_PROFILE_DENY or choose another execution profile"
	}
	return c
}

func (r *HealthReport) add(c HealthCheck) {
	r.Checks = append(r.Checks, c)
	r.Count++
	switch c.Status {
	case CheckOK:
		r.OKCount++
	case CheckFail:
		r.FailCount++
	default:
		r.WarningCount++
	}
}

func profileCheck(profiles map[string]Profile, id, title, okDetail, notRoutedNext string) HealthCheck {
	p, ok := profiles[id]
	if !ok {
		return HealthCheck{
			ID:        id + ".missing",
			ProfileID: id,
			Status:    CheckFail,
			Title:     title,
			Detail:    "profile is missing from the execution-profile inventory",
			Next:      "restore the inventory entry before exposing this profile",
		}
	}
	c := HealthCheck{
		ID:        id + ".profile",
		ProfileID: id,
		Title:     title,
		Routed:    p.Routed,
		Degraded:  p.Degraded,
	}
	switch {
	case !p.Routed:
		c.Status = CheckWarning
		c.Detail = "profile exists but no registered tool currently routes through it"
		c.Next = notRoutedNext
	case p.Degraded:
		c.Status = CheckWarning
		c.Detail = p.DegradeReason
		if c.Detail == "" {
			c.Detail = "profile is routed but effective isolation is weaker than requested"
		}
		c.Next = "use dry-run and warden events to verify requested vs effective isolation before high-risk work"
	default:
		c.Status = CheckOK
		c.Detail = okDetail
	}
	if id == "remote-agezt" && p.Routed {
		c.Status = CheckOK
		c.Detail = "whole-run peer delegation is routable through remote_run; the peer enforces its own policy and journal"
		c.Next = ""
	}
	return c
}

func remoteSecretPolicyCheck(policy SecretPolicy) HealthCheck {
	c := HealthCheck{
		ID:        "remote-cloud.secret_policy",
		ProfileID: "remote-cloud-secrets",
		Title:     "remote/cloud secret policy",
		Status:    CheckOK,
		Detail:    policy.Detail,
	}
	if policy.Mode == "" {
		policy = ParseRemoteSecretPolicy("")
		c.Detail = policy.Detail
	}
	if !policy.Valid {
		c.Status = CheckWarning
		c.Detail = policy.Detail
		c.Next = "set " + RemoteSecretPolicyEnv + " to deny or metadata"
	}
	return c
}

func remoteArtifactBytesPolicyCheck(raw string) HealthCheck {
	mode := strings.ToLower(strings.TrimSpace(raw))
	c := HealthCheck{
		ID:        "remote-cloud.artifact_bytes",
		ProfileID: "remote-artifacts",
		Title:     "remote artifact bytes policy",
		Status:    CheckOK,
	}
	switch mode {
	case "", "off", "deny":
		c.Detail = "remote artifact byte transfer is disabled by default; metadata-only mirroring can still list artifact refs"
	case "allow", "1", "true", "yes", "on":
		c.Detail = "remote artifact byte transfer is enabled for authenticated REST clients up to the daemon limit"
	default:
		c.Status = CheckWarning
		c.Detail = "invalid " + RemoteArtifactBytesEnv + " value; remote artifact byte transfer is denied"
		c.Next = "set " + RemoteArtifactBytesEnv + " to off or allow"
	}
	return c
}

func commandProfileCheck(profiles map[string]Profile, id, title string, commands []string, lookPath func(string) (string, error)) HealthCheck {
	p, ok := profiles[id]
	if !ok {
		return HealthCheck{
			ID:        id + ".missing",
			ProfileID: id,
			Status:    CheckFail,
			Title:     title,
			Detail:    "profile is missing from the execution-profile inventory",
			Next:      "restore the inventory entry before exposing this profile",
		}
	}
	backend, available := firstAvailable(commands, lookPath)
	c := HealthCheck{
		ID:               id + ".backend",
		ProfileID:        id,
		Status:           CheckWarning,
		Title:            title,
		Routed:           p.Routed,
		Degraded:         p.Degraded,
		BackendAvailable: available,
		Backend:          backend,
	}
	switch {
	case p.Routed && !p.Degraded && available:
		c.Status = CheckOK
		c.Detail = "profile is routed and backend routing is available"
	case p.Routed && !p.Degraded && !available:
		c.Status = CheckFail
		c.Detail = strings.Join(commands, "/") + " is not on PATH, but AGEZT is configured to route this execution profile"
		c.Next = "install/configure the backend or disable the execution profile until the runtime is available"
	case available:
		c.Detail = backend + " is on PATH, but AGEZT does not route this execution profile yet"
		c.Next = "wire this backend into the tool execution path before advertising it as selectable"
	default:
		c.Detail = strings.Join(commands, "/") + " is not on PATH and AGEZT does not route this execution profile yet"
		c.Next = "install/configure the backend, then wire it into execution-profile routing"
	}
	return c
}

func firstAvailable(commands []string, lookPath func(string) (string, error)) (string, bool) {
	for _, name := range commands {
		if name == "" {
			continue
		}
		if _, err := lookPath(name); err == nil {
			return name, true
		} else if !errors.Is(err, exec.ErrNotFound) {
			continue
		}
	}
	return "", false
}
