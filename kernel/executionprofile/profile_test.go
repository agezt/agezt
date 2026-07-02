// SPDX-License-Identifier: MIT

package executionprofile

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/warden"
)

func TestBuildReportsRoutedProfilesAndDowngrades(t *testing.T) {
	inv := Build(Options{
		Tools:    []string{"shell", "code_exec", "coding", "browser", "remote_run"},
		HostOS:   "windows",
		HostArch: "amd64",
		EffectiveProfile: func(p warden.Profile) warden.Profile {
			if p == warden.ProfileNamespace {
				return warden.ProfileNone
			}
			return p
		},
	})
	if inv.HostOS != "windows" || inv.HostArch != "amd64" || inv.Count != 10 {
		t.Fatalf("inventory header = %+v", inv)
	}
	wd, ok := inv.Find("warden")
	if !ok {
		t.Fatal("missing warden profile")
	}
	if wd.Status != StatusDegraded || !wd.Degraded || wd.EffectiveIsolation != string(warden.ProfileNone) {
		t.Fatalf("warden downgrade not reported: %+v", wd)
	}
	if got := join(wd.Tools); got != "code_exec,shell" {
		t.Fatalf("warden tools = %q", got)
	}
	worktree, _ := inv.Find("worktree-coding")
	if !worktree.Routed || worktree.Status != StatusSupported {
		t.Fatalf("worktree profile should be routed/supported: %+v", worktree)
	}
	browser, _ := inv.Find("browser-session")
	if !browser.Routed || browser.PolicyCapability != "browser.read / browser.action" {
		t.Fatalf("browser profile wrong: %+v", browser)
	}
	remote, _ := inv.Find("remote-agezt")
	if !remote.Routed || remote.Status != StatusSupported || remote.PolicyCapability != "remote_run" {
		t.Fatalf("remote peer profile wrong: %+v", remote)
	}
	if inv.RoutedCount < 5 || inv.DegradedCount < 1 {
		t.Fatalf("inventory counts wrong: %+v", inv)
	}
	modal, _ := inv.Find("modal")
	if modal.Status != StatusPlanned || modal.Routed || modal.EffectiveIsolation != "not_routed" {
		t.Fatalf("modal should be a planned non-routable cloud profile: %+v", modal)
	}
}

func TestBuildShowsRemoteAgeztRoutableWithRemoteRunTool(t *testing.T) {
	inv := Build(Options{
		Tools: []string{"remote_run"},
		EffectiveProfile: func(p warden.Profile) warden.Profile {
			return p
		},
	})
	remote, ok := inv.Find("remote-agezt")
	if !ok {
		t.Fatal("missing remote-agezt profile")
	}
	if !remote.Routed || remote.Status != StatusSupported || remote.Degraded {
		t.Fatalf("remote-agezt should be routable/supported with remote_run: %+v", remote)
	}
	if got := join(remote.Tools); got != "remote_run" {
		t.Fatalf("remote tools = %q, want remote_run", got)
	}
	if got := join(RoutableRunProfileIDsFor(inv)); got != "local,warden,remote-agezt" {
		t.Fatalf("routable ids = %q", got)
	}
}

func TestRemoteSecretPolicyIsReportedForRemoteAndCloudProfiles(t *testing.T) {
	t.Setenv(RemoteSecretPolicyEnv, "metadata")
	inv := Build(Options{
		Tools: []string{"remote_run"},
		EffectiveProfile: func(p warden.Profile) warden.Profile {
			return p
		},
	})
	remote, _ := inv.Find("remote-agezt")
	if remote.SecretPolicy == nil || remote.SecretPolicy.Mode != "metadata" || !remote.SecretPolicy.MetadataForwarded || remote.SecretPolicy.ValuesForwarded {
		t.Fatalf("remote secret policy not reported correctly: %+v", remote.SecretPolicy)
	}
	if !strings.Contains(remote.Secrets, "metadata") || !strings.Contains(remote.Secrets, "never exported") {
		t.Fatalf("remote secrets summary missing policy: %q", remote.Secrets)
	}
	modal, _ := inv.Find("modal")
	if modal.SecretPolicy == nil || modal.SecretPolicy.Mode != "metadata" {
		t.Fatalf("cloud secret policy not reported correctly: %+v", modal.SecretPolicy)
	}
}

func TestBuildShowsDockerAsNotRoutedWithoutContainerBackend(t *testing.T) {
	inv := Build(Options{
		Tools: []string{"shell"},
		EffectiveProfile: func(p warden.Profile) warden.Profile {
			if p == warden.ProfileContainer {
				return warden.ProfileNamespace
			}
			return p
		},
	})
	docker, ok := inv.Find("docker")
	if !ok {
		t.Fatal("missing docker profile")
	}
	if docker.Routed || docker.Status != StatusPlanned || !docker.Degraded || docker.EffectiveIsolation != string(warden.ProfileNamespace) {
		t.Fatalf("docker should be planned/degraded without backend: %+v", docker)
	}
	if _, ok := inv.Find("nope"); ok {
		t.Fatal("unexpected unknown profile")
	}
}

func TestBuildShowsDockerRoutedWithContainerBackend(t *testing.T) {
	inv := Build(Options{
		Tools: []string{"shell", "code_exec"},
		EffectiveProfile: func(p warden.Profile) warden.Profile {
			return p
		},
	})
	docker, ok := inv.Find("docker")
	if !ok {
		t.Fatal("missing docker profile")
	}
	if !docker.Routed || docker.Status != StatusSupported || docker.Degraded || docker.EffectiveIsolation != string(warden.ProfileContainer) {
		t.Fatalf("docker should be routed/supported with backend: %+v", docker)
	}
	if got := join(docker.Tools); got != "code_exec,shell" {
		t.Fatalf("docker tools = %q", got)
	}
	if got := join(RoutableRunProfileIDsFor(inv)); got != "local,warden,docker" {
		t.Fatalf("routable ids = %q", got)
	}
}

func TestBuildShowsSSHRoutedWithConfig(t *testing.T) {
	inv := Build(Options{
		Tools: []string{"shell", "code_exec"},
		SSH:   SSHConfig{Enabled: true, Target: "deploy@example.com", WorkDir: "/srv/app"},
		EffectiveProfile: func(p warden.Profile) warden.Profile {
			return p
		},
	})
	ssh, ok := inv.Find("ssh")
	if !ok {
		t.Fatal("missing ssh profile")
	}
	if !ssh.Routed || ssh.Status != StatusPartial || ssh.Degraded || ssh.EffectiveIsolation != "remote-host" {
		t.Fatalf("ssh should be routed/partial with config: %+v", ssh)
	}
	if got := join(ssh.Tools); got != "code_exec,shell" {
		t.Fatalf("ssh tools = %q, want code_exec,shell", got)
	}
	if got := join(RoutableRunProfileIDsFor(inv)); got != "local,warden,docker,ssh" {
		t.Fatalf("routable ids = %q", got)
	}
}

func TestBuildShowsK8sRoutedWithConfig(t *testing.T) {
	inv := Build(Options{
		Tools: []string{"shell", "code_exec"},
		K8s:   K8sConfig{Enabled: true, Namespace: "agents", Pod: "runner-0", Container: "worker", WorkDir: "/workspace"},
		EffectiveProfile: func(p warden.Profile) warden.Profile {
			return p
		},
	})
	k8s, ok := inv.Find("k8s")
	if !ok {
		t.Fatal("missing k8s profile")
	}
	if !k8s.Routed || k8s.Status != StatusPartial || k8s.Degraded || k8s.EffectiveIsolation != "kubernetes-pod" {
		t.Fatalf("k8s should be routed/partial with config: %+v", k8s)
	}
	if got := join(k8s.Tools); got != "code_exec,shell" {
		t.Fatalf("k8s tools = %q, want code_exec,shell", got)
	}
	if got := join(RoutableRunProfileIDsFor(inv)); got != "local,warden,docker,k8s" {
		t.Fatalf("routable ids = %q", got)
	}
}

func TestBuildShowsModalAndDaytonaRoutedWithConfig(t *testing.T) {
	inv := Build(Options{
		Tools:   []string{"shell", "code_exec"},
		Modal:   ModalConfig{Enabled: true, Ref: "app.py::main", Environment: "prod"},
		Daytona: DaytonaConfig{Enabled: true, Sandbox: "sandbox-1", WorkDir: "/workspace"},
		EffectiveProfile: func(p warden.Profile) warden.Profile {
			return p
		},
	})
	modal, ok := inv.Find("modal")
	if !ok {
		t.Fatal("missing modal profile")
	}
	if !modal.Routed || modal.Status != StatusPartial || modal.Degraded || modal.EffectiveIsolation != "modal-shell" {
		t.Fatalf("modal should be routed/partial with config: %+v", modal)
	}
	if got := join(modal.Tools); got != "code_exec,shell" {
		t.Fatalf("modal tools = %q, want code_exec,shell", got)
	}
	daytona, ok := inv.Find("daytona")
	if !ok {
		t.Fatal("missing daytona profile")
	}
	if !daytona.Routed || daytona.Status != StatusPartial || daytona.Degraded || daytona.EffectiveIsolation != "daytona-sandbox" {
		t.Fatalf("daytona should be routed/partial with config: %+v", daytona)
	}
	if got := join(daytona.Tools); got != "code_exec,shell" {
		t.Fatalf("daytona tools = %q, want code_exec,shell", got)
	}
	if got := join(RoutableRunProfileIDsFor(inv)); got != "local,warden,docker,modal,daytona" {
		t.Fatalf("routable ids = %q", got)
	}
}

func TestProfilePolicyAllowsAndDenies(t *testing.T) {
	policy := ParseProfilePolicy("local, ssh", "docker")
	if ok, reason := policy.Allows("LOCAL"); !ok || reason != "" {
		t.Fatalf("local should be allowed, got ok=%v reason=%q", ok, reason)
	}
	if ok, reason := policy.Allows("docker"); ok || reason != "denied by AGEZT_EXEC_PROFILE_DENY" {
		t.Fatalf("docker should be denied, got ok=%v reason=%q", ok, reason)
	}
	if ok, reason := policy.Allows("warden"); ok || reason != "not listed in AGEZT_EXEC_PROFILE_ALLOW" {
		t.Fatalf("warden should be excluded by allowlist, got ok=%v reason=%q", ok, reason)
	}
	denyWins := ParseProfilePolicy("all", "ssh")
	if ok, reason := denyWins.Allows("ssh"); ok || reason != "denied by AGEZT_EXEC_PROFILE_DENY" {
		t.Fatalf("deny should override allow-all, got ok=%v reason=%q", ok, reason)
	}
}

func TestSSHConfigContextRoundTrips(t *testing.T) {
	if _, ok := SSHOverrideFrom(context.Background()); ok {
		t.Fatal("empty context should not have ssh override")
	}
	cfg := SSHConfig{Enabled: true, Target: "deploy@example.com"}
	got, ok := SSHOverrideFrom(WithSSHOverride(context.Background(), cfg))
	if !ok || got.Target != cfg.Target {
		t.Fatalf("SSHOverrideFrom = %+v/%v", got, ok)
	}
}

func TestK8sConfigContextRoundTripsAndArgv(t *testing.T) {
	if _, ok := K8sOverrideFrom(context.Background()); ok {
		t.Fatal("empty context should not have k8s override")
	}
	cfg := K8sConfig{
		Enabled:   true,
		Context:   "prod",
		Namespace: "agents",
		Pod:       "runner-0",
		Container: "worker",
		WorkDir:   "/srv/app dir",
	}
	got, ok := K8sOverrideFrom(WithK8sOverride(context.Background(), cfg))
	if !ok || got.Pod != cfg.Pod {
		t.Fatalf("K8sOverrideFrom = %+v/%v", got, ok)
	}
	argv := cfg.ShellCommandArgv("echo ok")
	want := []string{"kubectl", "--context", "prod", "-n", "agents", "exec", "runner-0", "-c", "worker", "--", "sh", "-lc", "cd '/srv/app dir' && echo ok"}
	if join(argv) != join(want) {
		t.Fatalf("ShellCommandArgv = %#v, want %#v", argv, want)
	}
}

func TestModalAndDaytonaConfigContextRoundTripsAndArgv(t *testing.T) {
	if _, ok := ModalOverrideFrom(context.Background()); ok {
		t.Fatal("empty context should not have modal override")
	}
	modal := ModalConfig{Enabled: true, Ref: "app.py::main", Environment: "prod", WorkDir: "/srv/app"}
	gotModal, ok := ModalOverrideFrom(WithModalOverride(context.Background(), modal))
	if !ok || gotModal.Ref != modal.Ref {
		t.Fatalf("ModalOverrideFrom = %+v/%v", gotModal, ok)
	}
	modalArgv := modal.ShellCommandArgv("echo ok")
	if join(modalArgv) != join([]string{"modal", "shell", "--env", "prod", "app.py::main", "--cmd", "cd '/srv/app' && echo ok", "--no-pty"}) {
		t.Fatalf("modal argv = %#v", modalArgv)
	}
	modalCodeArgv := modal.CodeExecArgv("/tmp/run-123", "python3 main.py")
	if join(modalCodeArgv) != join([]string{"modal", "shell", "--env", "prod", "--add-local", "/tmp/run-123", "--cmd", "python3 main.py", "--no-pty"}) {
		t.Fatalf("modal code argv = %#v", modalCodeArgv)
	}

	if _, ok := DaytonaOverrideFrom(context.Background()); ok {
		t.Fatal("empty context should not have daytona override")
	}
	daytona := DaytonaConfig{Enabled: true, Sandbox: "sandbox-1", WorkDir: "/workspace"}
	gotDaytona, ok := DaytonaOverrideFrom(WithDaytonaOverride(context.Background(), daytona))
	if !ok || gotDaytona.Sandbox != daytona.Sandbox {
		t.Fatalf("DaytonaOverrideFrom = %+v/%v", gotDaytona, ok)
	}
	daytonaArgv := daytona.ShellCommandArgv("echo ok", 30)
	if join(daytonaArgv) != join([]string{"daytona", "exec", "sandbox-1", "--cwd", "/workspace", "--timeout", "30", "--", "sh", "-lc", "echo ok"}) {
		t.Fatalf("daytona argv = %#v", daytonaArgv)
	}
	daytonaCodeArgv := daytona.CommandArgv("python3 main.py", "/workspace/runs/run-123", 30)
	if join(daytonaCodeArgv) != join([]string{"daytona", "exec", "sandbox-1", "--cwd", "/workspace/runs/run-123", "--timeout", "30", "--", "sh", "-lc", "python3 main.py"}) {
		t.Fatalf("daytona code argv = %#v", daytonaCodeArgv)
	}
}

func TestWardenProfileForRun(t *testing.T) {
	cases := []struct {
		id   string
		want warden.Profile
		ok   bool
	}{
		{"local", warden.ProfileNone, true},
		{" LOCAL ", warden.ProfileNone, true},
		{"warden", warden.ProfileNamespace, true},
		{"docker", warden.ProfileContainer, true},
		{"ssh", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := WardenProfileForRun(tc.id)
		if ok != tc.ok || got != tc.want {
			t.Errorf("WardenProfileForRun(%q) = (%q,%v), want (%q,%v)", tc.id, got, ok, tc.want, tc.ok)
		}
	}
}

func TestDiagnoseReportsRoutableAndPlannedProfiles(t *testing.T) {
	inv := Build(Options{
		Tools:  []string{"shell", "browser"},
		HostOS: "windows", HostArch: "amd64",
		EffectiveProfile: func(p warden.Profile) warden.Profile {
			if p == warden.ProfileNamespace {
				return warden.ProfileNone
			}
			if p == warden.ProfileContainer {
				return warden.ProfileNone
			}
			return p
		},
	})
	report := Diagnose(inv, HealthOptions{
		LookPath: func(name string) (string, error) {
			if name == "docker" {
				return "/usr/bin/docker", nil
			}
			return "", errors.New("not found")
		},
	})
	if report.Count != 12 || report.OKCount == 0 || report.WarningCount == 0 || report.FailCount != 0 {
		t.Fatalf("report counts wrong: %+v", report)
	}
	byID := map[string]HealthCheck{}
	for _, c := range report.Checks {
		byID[c.ProfileID] = c
	}
	if byID[IDLocal].Status != CheckOK {
		t.Fatalf("local check = %+v", byID[IDLocal])
	}
	if byID[IDWarden].Status != CheckWarning || !byID[IDWarden].Degraded {
		t.Fatalf("warden check should warn on downgrade: %+v", byID[IDWarden])
	}
	if byID["docker"].Backend != "docker" || !byID["docker"].BackendAvailable || byID["docker"].Status != CheckWarning {
		t.Fatalf("docker check wrong: %+v", byID["docker"])
	}
	if byID["ssh"].BackendAvailable || byID["ssh"].Status != CheckWarning {
		t.Fatalf("ssh check wrong: %+v", byID["ssh"])
	}
	if byID["modal"].Status != CheckWarning || byID["modal"].BackendAvailable {
		t.Fatalf("modal check wrong: %+v", byID["modal"])
	}
}

func TestDiagnoseWarnsOnInvalidRemoteSecretPolicy(t *testing.T) {
	inv := Build(Options{
		Tools: []string{"remote_run"},
		EffectiveProfile: func(p warden.Profile) warden.Profile {
			return p
		},
	})
	report := Diagnose(inv, HealthOptions{
		RemoteSecretPolicy: ParseRemoteSecretPolicy("export-values"),
		LookPath:           func(string) (string, error) { return "", errors.New("not found") },
	})
	var policyCheck HealthCheck
	for _, c := range report.Checks {
		if c.ID == "remote-cloud.secret_policy" {
			policyCheck = c
			break
		}
	}
	if policyCheck.Status != CheckWarning || !strings.Contains(policyCheck.Detail, "invalid") || policyCheck.Next == "" {
		t.Fatalf("invalid remote secret policy check = %+v", policyCheck)
	}
}

func TestDiagnoseReportsRemoteArtifactBytesPolicy(t *testing.T) {
	inv := Build(Options{Tools: []string{"remote_run"}})

	t.Setenv(RemoteArtifactBytesEnv, "")
	report := Diagnose(inv, HealthOptions{})
	byID := map[string]HealthCheck{}
	for _, c := range report.Checks {
		byID[c.ID] = c
	}
	if byID["remote-cloud.artifact_bytes"].Status != CheckOK || !strings.Contains(byID["remote-cloud.artifact_bytes"].Detail, "disabled") {
		t.Fatalf("default remote artifact bytes check = %+v", byID["remote-cloud.artifact_bytes"])
	}

	t.Setenv(RemoteArtifactBytesEnv, "allow")
	report = Diagnose(inv, HealthOptions{})
	byID = map[string]HealthCheck{}
	for _, c := range report.Checks {
		byID[c.ID] = c
	}
	if byID["remote-cloud.artifact_bytes"].Status != CheckOK || !strings.Contains(byID["remote-cloud.artifact_bytes"].Detail, "enabled") {
		t.Fatalf("enabled remote artifact bytes check = %+v", byID["remote-cloud.artifact_bytes"])
	}

	t.Setenv(RemoteArtifactBytesEnv, "export-everything")
	report = Diagnose(inv, HealthOptions{})
	byID = map[string]HealthCheck{}
	for _, c := range report.Checks {
		byID[c.ID] = c
	}
	if byID["remote-cloud.artifact_bytes"].Status != CheckWarning || byID["remote-cloud.artifact_bytes"].Next == "" {
		t.Fatalf("invalid remote artifact bytes check = %+v", byID["remote-cloud.artifact_bytes"])
	}
}

func TestDiagnoseReportsDockerOKOnlyWhenRoutedAndRuntimeAvailable(t *testing.T) {
	inv := Build(Options{
		Tools: []string{"shell"},
		EffectiveProfile: func(p warden.Profile) warden.Profile {
			return p
		},
	})
	report := Diagnose(inv, HealthOptions{
		LookPath: func(name string) (string, error) {
			if name == "podman" {
				return "/usr/bin/podman", nil
			}
			return "", errors.New("not found")
		},
	})
	byID := map[string]HealthCheck{}
	for _, c := range report.Checks {
		byID[c.ProfileID] = c
	}
	if byID["docker"].Status != CheckOK || byID["docker"].Backend != "podman" {
		t.Fatalf("docker check should be OK with routed backend: %+v", byID["docker"])
	}
	if got := join(report.RoutableRunProfiles); got != "local,warden,docker" {
		t.Fatalf("routable profiles = %q", got)
	}

	blocked := Diagnose(inv, HealthOptions{
		Policy: ParseProfilePolicy("", "docker"),
		LookPath: func(name string) (string, error) {
			if name == "podman" {
				return "/usr/bin/podman", nil
			}
			return "", errors.New("not found")
		},
	})
	byID = map[string]HealthCheck{}
	for _, c := range blocked.Checks {
		byID[c.ProfileID] = c
	}
	if got := join(blocked.RoutableRunProfiles); got != "local,warden" {
		t.Fatalf("policy-filtered routable profiles = %q", got)
	}
	if byID["docker"].Status != CheckWarning || byID["docker"].Detail != "profile is blocked by execution-profile policy: denied by AGEZT_EXEC_PROFILE_DENY" {
		t.Fatalf("docker policy check wrong: %+v", byID["docker"])
	}
}

func TestDiagnoseReportsSSHOKWhenConfiguredAndRuntimeAvailable(t *testing.T) {
	inv := Build(Options{
		Tools: []string{"shell"},
		SSH:   SSHConfig{Enabled: true, Target: "deploy@example.com"},
		EffectiveProfile: func(p warden.Profile) warden.Profile {
			return p
		},
	})
	report := Diagnose(inv, HealthOptions{
		LookPath: func(name string) (string, error) {
			if name == "ssh" {
				return "/usr/bin/ssh", nil
			}
			return "", errors.New("not found")
		},
	})
	byID := map[string]HealthCheck{}
	for _, c := range report.Checks {
		byID[c.ProfileID] = c
	}
	if byID["ssh"].Status != CheckOK || byID["ssh"].Backend != "ssh" {
		t.Fatalf("ssh check should be OK with routed backend: %+v", byID["ssh"])
	}
	if got := join(report.RoutableRunProfiles); got != "local,warden,docker,ssh" {
		t.Fatalf("routable profiles = %q", got)
	}

	filtered := Diagnose(inv, HealthOptions{
		Policy: ParseProfilePolicy("ssh", ""),
		LookPath: func(name string) (string, error) {
			if name == "ssh" {
				return "/usr/bin/ssh", nil
			}
			return "", errors.New("not found")
		},
	})
	if got := join(filtered.RoutableRunProfiles); got != "ssh" {
		t.Fatalf("allowlist-filtered routable profiles = %q", got)
	}
}

func TestDiagnoseReportsK8sOKWhenConfiguredAndRuntimeAvailable(t *testing.T) {
	inv := Build(Options{
		Tools: []string{"shell"},
		K8s:   K8sConfig{Enabled: true, Namespace: "agents", Pod: "runner-0"},
		EffectiveProfile: func(p warden.Profile) warden.Profile {
			return p
		},
	})
	report := Diagnose(inv, HealthOptions{
		LookPath: func(name string) (string, error) {
			if name == "kubectl" {
				return "/usr/bin/kubectl", nil
			}
			return "", errors.New("not found")
		},
	})
	byID := map[string]HealthCheck{}
	for _, c := range report.Checks {
		byID[c.ProfileID] = c
	}
	if byID["k8s"].Status != CheckOK || byID["k8s"].Backend != "kubectl" {
		t.Fatalf("k8s check should be OK with routed backend: %+v", byID["k8s"])
	}
	if got := join(report.RoutableRunProfiles); got != "local,warden,docker,k8s" {
		t.Fatalf("routable profiles = %q", got)
	}
}

func TestDiagnoseReportsModalAndDaytonaOKWhenConfiguredAndRuntimeAvailable(t *testing.T) {
	inv := Build(Options{
		Tools:   []string{"shell"},
		Modal:   ModalConfig{Enabled: true, Ref: "app.py::main"},
		Daytona: DaytonaConfig{Enabled: true, Sandbox: "sandbox-1"},
		EffectiveProfile: func(p warden.Profile) warden.Profile {
			return p
		},
	})
	report := Diagnose(inv, HealthOptions{
		LookPath: func(name string) (string, error) {
			if name == "modal" || name == "daytona" {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("not found")
		},
	})
	byID := map[string]HealthCheck{}
	for _, c := range report.Checks {
		byID[c.ProfileID] = c
	}
	if byID["modal"].Status != CheckOK || byID["modal"].Backend != "modal" {
		t.Fatalf("modal check should be OK with routed backend: %+v", byID["modal"])
	}
	if byID["daytona"].Status != CheckOK || byID["daytona"].Backend != "daytona" {
		t.Fatalf("daytona check should be OK with routed backend: %+v", byID["daytona"])
	}
	if got := join(report.RoutableRunProfiles); got != "local,warden,docker,modal,daytona" {
		t.Fatalf("routable profiles = %q", got)
	}
}

func TestDiagnoseReportsRemoteAgeztOKWhenRemoteRunRegistered(t *testing.T) {
	inv := Build(Options{
		Tools: []string{"remote_run"},
		EffectiveProfile: func(p warden.Profile) warden.Profile {
			return p
		},
	})
	report := Diagnose(inv, HealthOptions{
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
	})
	byID := map[string]HealthCheck{}
	for _, c := range report.Checks {
		byID[c.ProfileID] = c
	}
	if byID["remote-agezt"].Status != CheckOK {
		t.Fatalf("remote-agezt check should be OK: %+v", byID["remote-agezt"])
	}
	if got := join(report.RoutableRunProfiles); got != "local,warden,remote-agezt" {
		t.Fatalf("routable profiles = %q", got)
	}
}

func TestDiagnoseReportsCloudBackendAvailabilityWithoutRouting(t *testing.T) {
	inv := Build(Options{
		Tools: []string{"shell"},
		EffectiveProfile: func(p warden.Profile) warden.Profile {
			return p
		},
	})
	report := Diagnose(inv, HealthOptions{
		LookPath: func(name string) (string, error) {
			if name == "modal" || name == "kubectl" {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("not found")
		},
	})
	byID := map[string]HealthCheck{}
	for _, c := range report.Checks {
		byID[c.ProfileID] = c
	}
	if byID["modal"].Status != CheckWarning || !byID["modal"].BackendAvailable || byID["modal"].Backend != "modal" {
		t.Fatalf("modal check should report backend present but not routed: %+v", byID["modal"])
	}
	if byID["k8s"].Status != CheckWarning || !byID["k8s"].BackendAvailable || byID["k8s"].Backend != "kubectl" {
		t.Fatalf("k8s check should report backend present but not routed: %+v", byID["k8s"])
	}
	if byID["daytona"].Status != CheckWarning || byID["daytona"].BackendAvailable {
		t.Fatalf("daytona check should warn missing backend/not routed: %+v", byID["daytona"])
	}
	if got := join(report.RoutableRunProfiles); got != "local,warden,docker" {
		t.Fatalf("cloud profiles should not be selectable yet, got %q", got)
	}
}

func TestDiagnoseFailsWhenCoreProfileMissing(t *testing.T) {
	report := Diagnose(Inventory{HostOS: "test", HostArch: "test"}, HealthOptions{
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
	})
	if report.FailCount == 0 {
		t.Fatalf("missing inventory should fail: %+v", report)
	}
}

func join(v []string) string {
	out := ""
	for i, s := range v {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}
