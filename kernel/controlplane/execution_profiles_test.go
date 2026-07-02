// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/shell"
)

func TestExecutionProfilesInventoryAndShow(t *testing.T) {
	_, _, c, _ := startPairWithConfig(t, runtime.Config{
		Provider: mock.New(mock.FinalText("ok")),
		Tools: map[string]agent.Tool{
			"shell":      shell.NewWithWarden(warden.New(nil)),
			"code_exec":  testTool{name: "code_exec"},
			"coding":     testTool{name: "coding"},
			"browser":    testTool{name: "browser"},
			"remote_run": testTool{name: "remote_run"},
		},
	})
	res, err := c.Call(context.Background(), controlplane.CmdExecutionProfiles, nil)
	if err != nil {
		t.Fatalf("execution profiles: %v", err)
	}
	if intOf(res["count"]) != 10 || res["host_os"] == "" || res["host_arch"] == "" {
		t.Fatalf("inventory header wrong: %+v", res)
	}
	rows, _ := res["profiles"].([]any)
	byID := map[string]map[string]any{}
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		byID[row["id"].(string)] = row
	}
	wardenRow := byID["warden"]
	if wardenRow == nil || wardenRow["requested_isolation"] != "namespace" || wardenRow["routed"] != true {
		t.Fatalf("warden profile wrong: %+v", wardenRow)
	}
	if got := strings.Join(anyStrings(wardenRow["tools"]), ","); got != "code_exec,shell" {
		t.Fatalf("warden tools = %q", got)
	}
	if byID["worktree-coding"]["routed"] != true || byID["browser-session"]["routed"] != true {
		t.Fatalf("expected coding/browser routed: %+v", byID)
	}
	if byID["remote-agezt"]["routed"] != true || byID["remote-agezt"]["status"] != "supported" {
		t.Fatalf("expected remote-agezt routed/supported: %+v", byID["remote-agezt"])
	}
	remotePolicy, _ := byID["remote-agezt"]["secret_policy"].(map[string]any)
	if remotePolicy["mode"] != "deny" || remotePolicy["values_forwarded"] != false {
		t.Fatalf("expected deny remote secret policy: %+v", remotePolicy)
	}
	if byID["modal"]["status"] != "planned" || byID["modal"]["routed"] != false {
		t.Fatalf("expected modal planned/not routed: %+v", byID["modal"])
	}

	show, err := c.Call(context.Background(), controlplane.CmdExecutionProfileShow, map[string]any{"id": "warden"})
	if err != nil {
		t.Fatalf("show warden: %v", err)
	}
	profile, _ := show["profile"].(map[string]any)
	if profile["id"] != "warden" || profile["filesystem"] == "" || profile["policy_capability"] == "" {
		t.Fatalf("show profile wrong: %+v", profile)
	}
	if _, err := c.Call(context.Background(), controlplane.CmdExecutionProfileShow, map[string]any{"id": "missing"}); err == nil || !strings.Contains(err.Error(), "unknown execution profile") {
		t.Fatalf("missing profile err = %v", err)
	}

	check, err := c.Call(context.Background(), controlplane.CmdExecutionProfileCheck, nil)
	if err != nil {
		t.Fatalf("check execution profiles: %v", err)
	}
	if intOf(check["count"]) != 12 || intOf(check["warning_count"]) == 0 {
		t.Fatalf("check counts wrong: %+v", check)
	}
	checks, _ := check["checks"].([]any)
	var sawWarden, sawRemoteSecrets, sawRemoteArtifactBytes bool
	for _, raw := range checks {
		row, _ := raw.(map[string]any)
		if row["profile_id"] == "warden" {
			sawWarden = true
			if row["status"] == "" || row["detail"] == "" {
				t.Fatalf("warden check incomplete: %+v", row)
			}
		}
		if row["id"] == "remote-cloud.secret_policy" {
			sawRemoteSecrets = true
			if row["status"] != "ok" || !strings.Contains(row["detail"].(string), "not exported") {
				t.Fatalf("remote secret policy check incomplete: %+v", row)
			}
		}
		if row["id"] == "remote-cloud.artifact_bytes" {
			sawRemoteArtifactBytes = true
			if row["status"] != "ok" || !strings.Contains(row["detail"].(string), "disabled") {
				t.Fatalf("remote artifact bytes check incomplete: %+v", row)
			}
		}
	}
	if !sawWarden {
		t.Fatalf("missing warden check: %+v", check)
	}
	if !sawRemoteSecrets {
		t.Fatalf("missing remote/cloud secret policy check: %+v", check)
	}
	if !sawRemoteArtifactBytes {
		t.Fatalf("missing remote artifact bytes policy check: %+v", check)
	}
}

func TestExecutionProfilesReportsConfiguredSSH(t *testing.T) {
	t.Setenv("AGEZT_EXEC_SSH", "1")
	t.Setenv("AGEZT_EXEC_SSH_TARGET", "deploy@example.com")
	_, _, c, _ := startPairWithConfig(t, runtime.Config{
		Provider: mock.New(mock.FinalText("ok")),
		Tools: map[string]agent.Tool{
			"shell": shell.NewWithWarden(warden.New(nil)),
		},
	})
	res, err := c.Call(context.Background(), controlplane.CmdExecutionProfiles, nil)
	if err != nil {
		t.Fatalf("execution profiles: %v", err)
	}
	rows, _ := res["profiles"].([]any)
	byID := map[string]map[string]any{}
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		byID[row["id"].(string)] = row
	}
	if byID["ssh"]["routed"] != true || byID["ssh"]["status"] != "partial" {
		t.Fatalf("ssh profile wrong: %+v", byID["ssh"])
	}
	check, err := c.Call(context.Background(), controlplane.CmdExecutionProfileCheck, nil)
	if err != nil {
		t.Fatalf("check execution profiles: %v", err)
	}
	if got := strings.Join(anyStrings(check["routable_run_profiles"]), ","); !strings.Contains(got, "ssh") {
		t.Fatalf("routable_run_profiles missing ssh: %q", got)
	}
}

func TestExecutionProfilesReportsConfiguredK8s(t *testing.T) {
	t.Setenv("AGEZT_EXEC_K8S", "1")
	t.Setenv("AGEZT_EXEC_K8S_NAMESPACE", "agents")
	t.Setenv("AGEZT_EXEC_K8S_POD", "runner-0")
	_, _, c, _ := startPairWithConfig(t, runtime.Config{
		Provider: mock.New(mock.FinalText("ok")),
		Tools: map[string]agent.Tool{
			"shell": shell.NewWithWarden(warden.New(nil)),
		},
	})
	res, err := c.Call(context.Background(), controlplane.CmdExecutionProfiles, nil)
	if err != nil {
		t.Fatalf("execution profiles: %v", err)
	}
	rows, _ := res["profiles"].([]any)
	byID := map[string]map[string]any{}
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		byID[row["id"].(string)] = row
	}
	if byID["k8s"]["routed"] != true || byID["k8s"]["status"] != "partial" {
		t.Fatalf("k8s profile wrong: %+v", byID["k8s"])
	}
	check, err := c.Call(context.Background(), controlplane.CmdExecutionProfileCheck, nil)
	if err != nil {
		t.Fatalf("check execution profiles: %v", err)
	}
	if got := strings.Join(anyStrings(check["routable_run_profiles"]), ","); !strings.Contains(got, "k8s") {
		t.Fatalf("routable_run_profiles missing k8s: %q", got)
	}
}

func TestExecutionProfilesReportsConfiguredModalAndDaytona(t *testing.T) {
	t.Setenv("AGEZT_EXEC_MODAL", "1")
	t.Setenv("AGEZT_EXEC_MODAL_REF", "app.py::main")
	t.Setenv("AGEZT_EXEC_DAYTONA", "1")
	t.Setenv("AGEZT_EXEC_DAYTONA_SANDBOX", "sandbox-1")
	_, _, c, _ := startPairWithConfig(t, runtime.Config{
		Provider: mock.New(mock.FinalText("ok")),
		Tools: map[string]agent.Tool{
			"shell": shell.NewWithWarden(warden.New(nil)),
		},
	})
	res, err := c.Call(context.Background(), controlplane.CmdExecutionProfiles, nil)
	if err != nil {
		t.Fatalf("execution profiles: %v", err)
	}
	rows, _ := res["profiles"].([]any)
	byID := map[string]map[string]any{}
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		byID[row["id"].(string)] = row
	}
	if byID["modal"]["routed"] != true || byID["modal"]["status"] != "partial" {
		t.Fatalf("modal profile wrong: %+v", byID["modal"])
	}
	if byID["daytona"]["routed"] != true || byID["daytona"]["status"] != "partial" {
		t.Fatalf("daytona profile wrong: %+v", byID["daytona"])
	}
	check, err := c.Call(context.Background(), controlplane.CmdExecutionProfileCheck, nil)
	if err != nil {
		t.Fatalf("check execution profiles: %v", err)
	}
	got := strings.Join(anyStrings(check["routable_run_profiles"]), ",")
	if !strings.Contains(got, "modal") || !strings.Contains(got, "daytona") {
		t.Fatalf("routable_run_profiles missing modal/daytona: %q", got)
	}
}
