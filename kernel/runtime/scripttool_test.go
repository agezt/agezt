// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/toolforge"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// stubRunner fakes the code-exec sandbox: it records what was asked and
// returns a canned output.
type stubRunner struct {
	lang, code, input string
	calls             int
	out               string
	isErr             bool
}

func (r *stubRunner) RunScript(_ context.Context, language, code, inputJSON string) (string, bool, error) {
	r.calls++
	r.lang, r.code, r.input = language, code, inputJSON
	return r.out, r.isErr, nil
}

func openForgeKernel(t *testing.T, prov agent.Provider, runner toolforge.Runner) *runtime.Kernel {
	t.Helper()
	k, err := runtime.Open(runtime.Config{
		BaseDir:      t.TempDir(),
		Provider:     prov,
		ScriptRunner: runner,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

// promoteEcho drafts + test-passes + promotes one script tool named "echo".
func promoteEcho(t *testing.T, k *runtime.Kernel, runner *stubRunner) toolforge.ScriptTool {
	t.Helper()
	st, err := k.DraftScriptTool("", toolforge.ScriptTool{
		Name:        "echo",
		Description: "echoes its input back",
		Language:    "python",
		Code:        "print(open('stdin.txt').read())",
	})
	if err != nil {
		t.Fatalf("DraftScriptTool: %v", err)
	}
	runner.out, runner.isErr = "test-ok", false
	rec, out, err := k.TestScriptTool(context.Background(), "", st.ID, `{"sample":1}`)
	if err != nil || !rec.TestedOK || out != "test-ok" {
		t.Fatalf("TestScriptTool = %v/%v/%q", rec.TestedOK, err, out)
	}
	if runner.input != `{"sample":1}` {
		t.Fatalf("test input = %q, want the sample payload", runner.input)
	}
	if _, err := k.PromoteScriptTool("", st.ID); err != nil {
		t.Fatalf("PromoteScriptTool: %v", err)
	}
	return st
}

// TestRunWith_OffersAndExecutesForgedTool is the arc's e2e: a drafted, tested,
// PROMOTED script is offered to the model as forge_echo, the model calls it,
// the sandbox runner receives the script + the call's raw JSON input, and its
// stdout flows back into the loop.
func TestRunWith_OffersAndExecutesForgedTool(t *testing.T) {
	prov := mock.New(
		testToolUse("c1", "forge_echo", map[string]any{"text": "merhaba"}),
		mock.FinalText("done"),
	)
	var first agent.CompletionRequest
	seen := false
	prov.OnRequest = func(r agent.CompletionRequest) {
		if !seen {
			first, seen = r, true
		}
	}
	runner := &stubRunner{}
	k := openForgeKernel(t, prov, runner)
	promoteEcho(t, k, runner)

	runner.out, runner.isErr = "echoed: merhaba", false
	calls := runner.calls
	if _, err := k.RunWith(context.Background(), k.NewCorrelation(), "echo something"); err != nil {
		t.Fatalf("RunWith: %v", err)
	}

	// The model was OFFERED the forged tool, with the forge description note.
	var def *agent.ToolDef
	for i := range first.Tools {
		if first.Tools[i].Name == "forge_echo" {
			def = &first.Tools[i]
		}
	}
	if def == nil {
		t.Fatalf("forge_echo not offered; tools = %v", toolNames(first.Tools))
	}
	if !strings.Contains(def.Description, "echoes its input back") {
		t.Errorf("description lost: %q", def.Description)
	}

	// The runner executed the stored script with the call's raw input.
	if runner.calls != calls+1 {
		t.Fatalf("runner calls = %d, want one run-call", runner.calls-calls)
	}
	if runner.lang != "python" || !strings.Contains(runner.code, "stdin.txt") {
		t.Errorf("runner got %q/%q, want the stored script", runner.lang, runner.code)
	}
	if !strings.Contains(runner.input, `"merhaba"`) {
		t.Errorf("call input did not reach the script: %q", runner.input)
	}
}

// TestRunWith_DraftAndQuarantineNeverOffered: only ACTIVE tools reach the
// model — a draft (even tested) and a quarantined tool stay invisible.
func TestRunWith_DraftAndQuarantineNeverOffered(t *testing.T) {
	prov := mock.New(mock.FinalText("ok"), mock.FinalText("ok"))
	var req agent.CompletionRequest
	prov.OnRequest = func(r agent.CompletionRequest) { req = r }
	runner := &stubRunner{}
	k := openForgeKernel(t, prov, runner)

	st, err := k.DraftScriptTool("", toolforge.ScriptTool{
		Name: "echo", Description: "d", Language: "python", Code: "print(1)",
	})
	if err != nil {
		t.Fatalf("DraftScriptTool: %v", err)
	}
	if _, err := k.RunWith(context.Background(), k.NewCorrelation(), "hi"); err != nil {
		t.Fatalf("RunWith(draft): %v", err)
	}
	if names := toolNames(req.Tools); contains(names, "forge_echo") {
		t.Fatalf("a DRAFT was offered: %v", names)
	}

	runner.out = "ok"
	if _, _, err := k.TestScriptTool(context.Background(), "", st.ID, ""); err != nil {
		t.Fatalf("TestScriptTool: %v", err)
	}
	if _, err := k.PromoteScriptTool("", st.ID); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if _, err := k.QuarantineScriptTool("", st.ID, "smoke"); err != nil {
		t.Fatalf("Quarantine: %v", err)
	}
	if _, err := k.RunWith(context.Background(), k.NewCorrelation(), "hi again"); err != nil {
		t.Fatalf("RunWith(quarantined): %v", err)
	}
	if names := toolNames(req.Tools); contains(names, "forge_echo") {
		t.Fatalf("a QUARANTINED tool was offered: %v", names)
	}
}

// TestRunWith_ToolAllowlistGatesForgedTools: a per-run tool restriction
// (WithTools) applies to forged tools exactly like registered ones — merge
// happens before the filter, so a restricted run can't smuggle them back.
func TestRunWith_ToolAllowlistGatesForgedTools(t *testing.T) {
	prov := mock.New(mock.FinalText("ok"), mock.FinalText("ok"))
	var req agent.CompletionRequest
	prov.OnRequest = func(r agent.CompletionRequest) { req = r }
	runner := &stubRunner{}
	k := openForgeKernel(t, prov, runner)
	promoteEcho(t, k, runner)

	ctx := runtime.WithTools(context.Background(), []string{"memory"}) // not forge_echo
	if _, err := k.RunWith(ctx, k.NewCorrelation(), "restricted"); err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	if names := toolNames(req.Tools); contains(names, "forge_echo") {
		t.Fatalf("allowlist did not gate the forged tool: %v", names)
	}

	ctx = runtime.WithTools(context.Background(), []string{"forge_echo"})
	if _, err := k.RunWith(ctx, k.NewCorrelation(), "allowed"); err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	if names := toolNames(req.Tools); !contains(names, "forge_echo") {
		t.Fatalf("allowlisted forged tool missing: %v", names)
	}
}

// TestTestScriptTool_FailureRecordedAndPromoteRefused: a failing sandbox test
// is recorded honestly and keeps the promotion gate shut.
func TestTestScriptTool_FailureRecordedAndPromoteRefused(t *testing.T) {
	prov := mock.New(mock.FinalText("ok"))
	runner := &stubRunner{out: "Traceback ...", isErr: true}
	k := openForgeKernel(t, prov, runner)

	st, err := k.DraftScriptTool("", toolforge.ScriptTool{
		Name: "broken", Description: "d", Language: "python", Code: "boom(",
	})
	if err != nil {
		t.Fatalf("DraftScriptTool: %v", err)
	}
	rec, out, err := k.TestScriptTool(context.Background(), "", st.Name, "")
	if err != nil {
		t.Fatalf("TestScriptTool: %v", err)
	}
	if rec.TestedOK || !strings.Contains(out, "Traceback") {
		t.Fatalf("failure not recorded: ok=%v out=%q", rec.TestedOK, out)
	}
	if _, err := k.PromoteScriptTool("", st.ID); err == nil {
		t.Fatal("promote accepted after a FAILED test")
	}
}

func toolNames(defs []agent.ToolDef) []string {
	out := make([]string, 0, len(defs))
	for _, d := range defs {
		out = append(out, d.Name)
	}
	return out
}

func contains(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// TestRequestToolPromotion (M813): the agent's promotion request blocks on
// the REAL approval registry; a grant promotes through the operator path, a
// deny comes back as the decision, and untested/active/ghost are refused
// before the operator ever sees a request.
func TestRequestToolPromotion(t *testing.T) {
	runner := &stubRunner{}
	k := openForgeKernel(t, mock.New(mock.FinalText("unused")), runner)

	st, err := k.DraftScriptTool("", toolforge.ScriptTool{
		Name: "greet", Description: "says hi", Language: "python", Code: "print('hi')",
	})
	if err != nil {
		t.Fatalf("draft: %v", err)
	}

	// Untested → refused without touching the registry.
	if _, _, _, err := k.RequestToolPromotion(context.Background(), "", st.Name); !errors.Is(err, toolforge.ErrUntested) {
		t.Fatalf("untested err = %v", err)
	}
	runner.out = "ok"
	if _, _, err := k.TestScriptTool(context.Background(), "", st.Name, "{}"); err != nil {
		t.Fatalf("test: %v", err)
	}

	// The operator resolves the pending request from another goroutine.
	resolve := func(decision approval.Decision, reason string) {
		deadline := time.After(5 * time.Second)
		for {
			for _, req := range k.Approvals().Pending() {
				if req.Capability == "toolforge.promote" {
					_ = k.Approvals().Resolve(req.ID, decision, reason, "tester")
					return
				}
			}
			select {
			case <-deadline:
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
	}

	// Denied: the verdict rides back, the draft stays a draft.
	go resolve(approval.DecisionDeny, "needs validation")
	_, decision, reason, err := k.RequestToolPromotion(context.Background(), "", "greet")
	if err != nil || decision != approval.DecisionDeny || reason != "needs validation" {
		t.Fatalf("deny: %v/%v/%v", decision, reason, err)
	}
	if got, _ := k.ToolForge().Get("greet"); got.Status == toolforge.StatusActive {
		t.Fatal("denied tool went active")
	}

	// Granted: ACTIVE via the same path the operator CLI uses.
	go resolve(approval.DecisionGrant, "lgtm")
	promoted, decision, _, err := k.RequestToolPromotion(context.Background(), "", "greet")
	if err != nil || decision != approval.DecisionGrant || promoted.Status != toolforge.StatusActive {
		t.Fatalf("grant: %+v %v %v", promoted, decision, err)
	}

	// Already active / ghost → refused up front.
	if _, _, _, err := k.RequestToolPromotion(context.Background(), "", "greet"); err == nil {
		t.Fatal("re-promoting an active tool accepted")
	}
	if _, _, _, err := k.RequestToolPromotion(context.Background(), "", "ghost"); err == nil {
		t.Fatal("ghost accepted")
	}
}

func TestRequestToolPromotion_AutoPromote(t *testing.T) {
	runner := &stubRunner{out: "ok"}
	k, err := runtime.Open(runtime.Config{
		BaseDir:                t.TempDir(),
		Provider:               mock.New(mock.FinalText("unused")),
		ScriptRunner:           runner,
		AutoPromoteScriptTools: true,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	st, err := k.DraftScriptTool("", toolforge.ScriptTool{
		Name: "fast", Description: "fast path", Language: "python", Code: "print('ok')",
	})
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	if _, _, _, err := k.RequestToolPromotion(context.Background(), "", st.Name); !errors.Is(err, toolforge.ErrUntested) {
		t.Fatalf("untested err = %v", err)
	}
	if _, _, err := k.TestScriptTool(context.Background(), "", st.Name, "{}"); err != nil {
		t.Fatalf("test: %v", err)
	}

	promoted, decision, reason, err := k.RequestToolPromotion(context.Background(), "", st.Name)
	if err != nil {
		t.Fatalf("auto promote: %v", err)
	}
	if decision != approval.DecisionGrant || reason != "auto-promote enabled" {
		t.Fatalf("decision/reason = %q/%q, want grant/auto-promote enabled", decision, reason)
	}
	if promoted.Status != toolforge.StatusActive {
		t.Fatalf("status = %s, want active", promoted.Status)
	}
	if pending := k.Approvals().Pending(); len(pending) != 0 {
		t.Fatalf("auto promote left %d pending approval(s)", len(pending))
	}
}
