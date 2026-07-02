// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/shell"
)

// TestRun_RejectsWrongTypedArgs — a per-run override arg sent with the wrong JSON
// type is reported as a usage error rather than silently mis-handled (M161). The
// two dangerous cases this guards: a mistyped `dry_run` that would otherwise fall
// through to false and EXECUTE a run the operator meant to preview (spending
// tokens), and a mistyped `tools` that would otherwise scope the run to NO tools.
func TestRun_RejectsWrongTypedArgs(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("hi")))

	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"dry_run as string", map[string]any{"intent": "x", "dry_run": "true"}, "dry_run must be a boolean"},
		{"tools as string", map[string]any{"intent": "x", "tools": "shell"}, "tools must be an array"},
		{"tools element non-string", map[string]any{"intent": "x", "tools": []any{"shell", 3.0}}, "tools[1] must be a string"},
		{"model as number", map[string]any{"intent": "x", "model": 5.0}, "model must be a string"},
		{"remote peer as number", map[string]any{"intent": "x", "remote_peer": 5.0}, "remote_peer must be a string"},
		{"remote peer without remote profile", map[string]any{"intent": "x", "remote_peer": "nodeB"}, `remote_peer requires execution_profile "remote-agezt"`},
		{"timeout as number", map[string]any{"intent": "x", "timeout": 30.0}, "timeout must be a string"},
		{"system as number", map[string]any{"intent": "x", "system": 1.0}, "system must be a string"},
		{"execution profile as number", map[string]any{"intent": "x", "execution_profile": 1.0}, "execution_profile must be a string"},
		{"execution profile docker backend inactive", map[string]any{"intent": "x", "execution_profile": "docker"}, "requires an active container backend"},
		{"execution profile ssh backend inactive", map[string]any{"intent": "x", "execution_profile": "ssh"}, "requires an active SSH backend"},
		{"execution profile k8s backend inactive", map[string]any{"intent": "x", "execution_profile": "k8s"}, "requires an active Kubernetes backend"},
		{"execution profile modal backend inactive", map[string]any{"intent": "x", "execution_profile": "modal"}, "requires an active Modal backend"},
		{"execution profile daytona backend inactive", map[string]any{"intent": "x", "execution_profile": "daytona"}, "requires an active Daytona backend"},
		{"execution profile remote backend inactive", map[string]any{"intent": "x", "execution_profile": "remote-agezt"}, "requires configured AGEZT peers"},
		{"execution profile unsupported", map[string]any{"intent": "x", "execution_profile": "cloud"}, "not routable"},
		{"images as string", map[string]any{"intent": "x", "images": "photo.png"}, "images must be an array"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.Stream(context.Background(), controlplane.CmdRun, tc.args, func(*event.Event) {})
			if err == nil {
				t.Fatalf("expected usage error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q; want it to contain %q", err.Error(), tc.want)
			}
		})
	}
}

// TestRun_WellTypedArgsStillRun — the correctly-typed forms the CLI actually
// sends are unaffected: a dry_run:true returns a plan (no execution), and an
// ordinary run completes.
func TestRun_WellTypedArgsStillRun(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// dry_run:true → a plan, run not executed.
	plan, err := c.Call(context.Background(), controlplane.CmdRun,
		map[string]any{"intent": "x", "dry_run": true, "tools": []any{}})
	if err != nil {
		t.Fatalf("dry-run errored: %v", err)
	}
	if plan["dry_run"] != true {
		t.Errorf("plan dry_run = %v, want true", plan["dry_run"])
	}
	if plan["tools_mode"] != "none (--no-tools)" {
		t.Errorf("tools_mode = %v, want none (--no-tools)", plan["tools_mode"])
	}

	plan, err = c.Call(context.Background(), controlplane.CmdRun,
		map[string]any{"intent": "x", "dry_run": true, "execution_profile": "local"})
	if err != nil {
		t.Fatalf("dry-run with execution profile errored: %v", err)
	}
	if plan["execution_profile"] != "local" || plan["warden_profile"] != "none" {
		t.Errorf("execution profile plan = %v / %v, want local / none", plan["execution_profile"], plan["warden_profile"])
	}

	dockerWarden := warden.NewWithOptions(nil, warden.Options{
		Container: warden.ContainerOptions{Enabled: true, Runtime: "docker", Image: "python:3.12-slim"},
	})
	_, _, c2, _ := startPairWithConfig(t, runtime.Config{
		Provider: mock.New(mock.FinalText("ok")),
		Warden:   dockerWarden,
		Tools:    map[string]agent.Tool{"shell": shell.NewWithWarden(dockerWarden)},
	})
	plan, err = c2.Call(context.Background(), controlplane.CmdRun,
		map[string]any{"intent": "x", "dry_run": true, "execution_profile": "docker"})
	if err != nil {
		t.Fatalf("dry-run with docker execution profile errored: %v", err)
	}
	if plan["execution_profile"] != "docker" || plan["warden_profile"] != "container" {
		t.Errorf("docker execution profile plan = %v / %v, want docker / container", plan["execution_profile"], plan["warden_profile"])
	}

	t.Setenv("AGEZT_EXEC_SSH", "1")
	t.Setenv("AGEZT_EXEC_SSH_TARGET", "deploy@example.com")
	plan, err = c.Call(context.Background(), controlplane.CmdRun,
		map[string]any{"intent": "x", "dry_run": true, "execution_profile": "ssh"})
	if err != nil {
		t.Fatalf("dry-run with ssh execution profile errored: %v", err)
	}
	if plan["execution_profile"] != "ssh" || plan["warden_profile"] != "ssh" {
		t.Errorf("ssh execution profile plan = %v / %v, want ssh / ssh", plan["execution_profile"], plan["warden_profile"])
	}

	t.Setenv("AGEZT_EXEC_K8S", "1")
	t.Setenv("AGEZT_EXEC_K8S_NAMESPACE", "agents")
	t.Setenv("AGEZT_EXEC_K8S_POD", "runner-0")
	plan, err = c.Call(context.Background(), controlplane.CmdRun,
		map[string]any{"intent": "x", "dry_run": true, "execution_profile": "k8s"})
	if err != nil {
		t.Fatalf("dry-run with k8s execution profile errored: %v", err)
	}
	if plan["execution_profile"] != "k8s" || plan["warden_profile"] != "k8s" {
		t.Errorf("k8s execution profile plan = %v / %v, want k8s / k8s", plan["execution_profile"], plan["warden_profile"])
	}

	t.Setenv("AGEZT_EXEC_MODAL", "1")
	t.Setenv("AGEZT_EXEC_MODAL_REF", "app.py::main")
	plan, err = c.Call(context.Background(), controlplane.CmdRun,
		map[string]any{"intent": "x", "dry_run": true, "execution_profile": "modal"})
	if err != nil {
		t.Fatalf("dry-run with modal execution profile errored: %v", err)
	}
	if plan["execution_profile"] != "modal" || plan["warden_profile"] != "modal" {
		t.Errorf("modal execution profile plan = %v / %v, want modal / modal", plan["execution_profile"], plan["warden_profile"])
	}

	t.Setenv("AGEZT_EXEC_DAYTONA", "1")
	t.Setenv("AGEZT_EXEC_DAYTONA_SANDBOX", "sandbox-1")
	plan, err = c.Call(context.Background(), controlplane.CmdRun,
		map[string]any{"intent": "x", "dry_run": true, "execution_profile": "daytona"})
	if err != nil {
		t.Fatalf("dry-run with daytona execution profile errored: %v", err)
	}
	if plan["execution_profile"] != "daytona" || plan["warden_profile"] != "daytona" {
		t.Errorf("daytona execution profile plan = %v / %v, want daytona / daytona", plan["execution_profile"], plan["warden_profile"])
	}

	_, _, c3, _ := startPairWithConfig(t, runtime.Config{
		Provider: mock.New(mock.FinalText("ok")),
		Tools:    map[string]agent.Tool{"remote_run": remoteRunFooterTool{wantPeer: "nodeC"}},
	})
	plan, err = c3.Call(context.Background(), controlplane.CmdRun,
		map[string]any{"intent": "x", "dry_run": true, "execution_profile": "remote-agezt", "remote_peer": "nodeC"})
	if err != nil {
		t.Fatalf("dry-run with remote-agezt execution profile errored: %v", err)
	}
	if plan["execution_profile"] != "remote-agezt" || plan["warden_profile"] != "remote-agezt" {
		t.Errorf("remote-agezt execution profile plan = %v / %v, want remote-agezt / remote-agezt", plan["execution_profile"], plan["warden_profile"])
	}
	if plan["remote_peer"] != "nodeC" {
		t.Errorf("remote-agezt dry-run remote_peer = %v, want nodeC", plan["remote_peer"])
	}

	remoteKinds := map[event.Kind]bool{}
	var remoteCompleted map[string]any
	res, err := c3.Stream(context.Background(), controlplane.CmdRun,
		map[string]any{"intent": "delegate this", "execution_profile": "remote-agezt", "remote_peer": "nodeC"}, func(e *event.Event) {
			remoteKinds[e.Kind] = true
			if e.Kind == event.KindTaskCompleted {
				_ = json.Unmarshal(e.Payload, &remoteCompleted)
			}
		})
	if err != nil {
		t.Fatalf("remote-agezt run errored: %v", err)
	}
	answer, _ := res["answer"].(string)
	if !strings.Contains(answer, "ok") {
		t.Errorf("remote-agezt answer = %v, want ok", res["answer"])
	}
	for _, kind := range []event.Kind{event.KindTaskReceived, event.KindInfo, event.KindTaskCompleted} {
		if !remoteKinds[kind] {
			t.Errorf("remote-agezt stream missing %s event; got %v", kind, remoteKinds)
		}
	}
	if remoteCompleted["remote_peer"] != "nodeC" || remoteCompleted["remote_correlation"] != "run-abc" || remoteCompleted["remote_model"] != "m" {
		t.Errorf("remote-agezt completed metadata = %v, want peer/correlation/model", remoteCompleted)
	}
	_, err = c3.Stream(context.Background(), controlplane.CmdRun,
		map[string]any{"intent": "delegate this", "execution_profile": "remote-agezt", "assure": 2.0}, func(*event.Event) {})
	if err == nil || !strings.Contains(err.Error(), "cannot combine with assure") {
		t.Fatalf("remote-agezt assure err = %v, want clear rejection", err)
	}

	// Ordinary run still completes (mock is fresh — the dry-run spent nothing).
	res, err = c.Stream(context.Background(), controlplane.CmdRun,
		map[string]any{"intent": "hello"}, func(*event.Event) {})
	if err != nil {
		t.Fatalf("plain run errored: %v", err)
	}
	if res["answer"] != "ok" {
		t.Errorf("answer = %v, want ok", res["answer"])
	}
}

type remoteRunFooterTool struct {
	wantPeer string
}

func (remoteRunFooterTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: "remote_run", Description: "test remote run", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (t remoteRunFooterTool) Invoke(_ context.Context, input json.RawMessage) (agent.Result, error) {
	var in struct {
		Peer string `json:"peer"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return agent.Result{Output: "invalid input: " + err.Error(), IsError: true}, nil
	}
	if t.wantPeer != "" && in.Peer != t.wantPeer {
		return agent.Result{Output: "remote_run peer = " + in.Peer + ", want " + t.wantPeer, IsError: true}, nil
	}
	peer := t.wantPeer
	if peer == "" {
		peer = "nodeB"
	}
	return agent.Result{Output: "ok\n\n[peer=" + peer + " model=m correlation=run-abc]"}, nil
}

func TestRun_RejectsExecutionProfileBlockedByPolicy(t *testing.T) {
	t.Setenv("AGEZT_EXEC_PROFILE_DENY", "local")
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	_, err := c.Call(context.Background(), controlplane.CmdRun,
		map[string]any{"intent": "x", "dry_run": true, "execution_profile": "local"})
	if err == nil {
		t.Fatal("expected policy rejection, got nil")
	}
	if !strings.Contains(err.Error(), "execution profile \"local\" is blocked by policy") {
		t.Fatalf("error = %q", err.Error())
	}
}
