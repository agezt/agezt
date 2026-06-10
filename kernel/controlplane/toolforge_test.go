// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// wireRunner fakes the code-exec sandbox behind the control plane: pass/fail
// is scripted per test.
type wireRunner struct {
	out   string
	isErr bool
}

func (r *wireRunner) RunScript(_ context.Context, _, _, _ string) (string, bool, error) {
	return r.out, r.isErr, nil
}

// TestToolforge_WireRoundTrip drives the full operator pipeline over the
// wire: draft → promote refused (untested) → test (pass) → promote → list
// shows it live → quarantine → edit code (demote) → remove. This is exactly
// what `agt toolforge` and the console speak.
func TestToolforge_WireRoundTrip(t *testing.T) {
	runner := &wireRunner{out: "echo-ok"}
	_, _, c, _ := startPairWithConfig(t, runtime.Config{
		Provider:     mock.New(mock.FinalText("unused")),
		ScriptRunner: runner,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Draft.
	res, err := c.Call(ctx, controlplane.CmdToolforgeDraft, map[string]any{
		"tool": map[string]any{
			"name": "echo", "description": "echoes input", "language": "python",
			"code": "print(open('stdin.txt').read())",
		},
	})
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	tool, _ := res["tool"].(map[string]any)
	if tool["status"] != "draft" {
		t.Fatalf("draft status = %v", tool["status"])
	}

	// Promote before any test → refused (the forge's core invariant).
	if _, err := c.Call(ctx, controlplane.CmdToolforgePromote, map[string]any{"ref": "echo"}); err == nil ||
		!strings.Contains(err.Error(), "test") {
		t.Fatalf("untested promote: got %v, want a test-first refusal", err)
	}

	// Test (sandbox pass) → recorded.
	res, err = c.Call(ctx, controlplane.CmdToolforgeTest, map[string]any{"ref": "echo", "input": `{"x":1}`})
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	if ok, _ := res["ok"].(bool); !ok || res["output"] != "echo-ok" {
		t.Fatalf("test verdict = %v / %v", res["ok"], res["output"])
	}

	// Promote → ACTIVE, callable as forge_echo.
	res, err = c.Call(ctx, controlplane.CmdToolforgePromote, map[string]any{"ref": "echo"})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	tool, _ = res["tool"].(map[string]any)
	if tool["status"] != "active" || tool["callable_as"] != "forge_echo" {
		t.Fatalf("promoted = %v", tool)
	}

	// List counts it as live; the code body stays out of the list view.
	res, err = c.Call(ctx, controlplane.CmdToolforgeList, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if ac, _ := res["active_count"].(float64); ac != 1 {
		t.Fatalf("active_count = %v", res["active_count"])
	}
	tools, _ := res["tools"].([]any)
	if first, _ := tools[0].(map[string]any); first["code"] != nil {
		t.Fatal("list leaked the code body")
	}

	// Show carries the full record.
	res, err = c.Call(ctx, controlplane.CmdToolforgeShow, map[string]any{"ref": "echo"})
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	tool, _ = res["tool"].(map[string]any)
	if code, _ := tool["code"].(string); !strings.Contains(code, "stdin.txt") {
		t.Fatalf("show missing code: %v", tool)
	}

	// Quarantine (kill switch).
	res, err = c.Call(ctx, controlplane.CmdToolforgeQuarantine, map[string]any{"ref": "echo", "reason": "smoke"})
	if err != nil {
		t.Fatalf("quarantine: %v", err)
	}
	tool, _ = res["tool"].(map[string]any)
	if tool["status"] != "quarantined" {
		t.Fatalf("quarantined = %v", tool["status"])
	}

	// Edit the code → demoted to draft, test record cleared.
	res, err = c.Call(ctx, controlplane.CmdToolforgeEdit, map[string]any{
		"ref": "echo", "tool": map[string]any{"code": "print('v2')"},
	})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	tool, _ = res["tool"].(map[string]any)
	if tool["status"] != "draft" || tool["tested_ok"] != false {
		t.Fatalf("code edit = %v/%v, want draft/untested", tool["status"], tool["tested_ok"])
	}

	// Remove.
	res, err = c.Call(ctx, controlplane.CmdToolforgeRemove, map[string]any{"ref": "echo"})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if removed, _ := res["removed"].(bool); !removed {
		t.Fatal("remove reported false")
	}
}

// TestToolforge_FailedTestKeepsGateShut: a failing sandbox run is recorded
// honestly over the wire and promotion stays refused.
func TestToolforge_FailedTestKeepsGateShut(t *testing.T) {
	runner := &wireRunner{out: "Traceback ...", isErr: true}
	_, _, c, _ := startPairWithConfig(t, runtime.Config{
		Provider:     mock.New(mock.FinalText("unused")),
		ScriptRunner: runner,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := c.Call(ctx, controlplane.CmdToolforgeDraft, map[string]any{
		"tool": map[string]any{"name": "broken", "description": "d", "language": "python", "code": "boom("},
	}); err != nil {
		t.Fatalf("draft: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdToolforgeTest, map[string]any{"ref": "broken"})
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	if ok, _ := res["ok"].(bool); ok {
		t.Fatal("failing run reported ok")
	}
	if _, err := c.Call(ctx, controlplane.CmdToolforgePromote, map[string]any{"ref": "broken"}); err == nil {
		t.Fatal("promote accepted after a failed test")
	}
}
