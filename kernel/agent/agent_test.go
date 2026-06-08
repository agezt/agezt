// SPDX-License-Identifier: MIT

package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/shell"
)

func newTestBus(t *testing.T) (*bus.Bus, *journal.Journal) {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { j.Close() })
	b := bus.New(j)
	t.Cleanup(b.Close)
	return b, j
}

func TestRun_NoTools_OneShot(t *testing.T) {
	b, j := newTestBus(t)
	prov := mock.New(mock.FinalText("Hello, world."))

	got, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider:      prov,
		Bus:           b,
		Actor:         "agent-1",
		CorrelationID: "corr-1",
	}, "say hi")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != "Hello, world." {
		t.Errorf("got %q, want %q", got, "Hello, world.")
	}

	// Expect events: task.received, llm.request, llm.response, task.completed.
	var kinds []event.Kind
	_ = j.Range(func(e *event.Event) error {
		kinds = append(kinds, e.Kind)
		return nil
	})
	want := []event.Kind{
		event.KindTaskReceived,
		event.KindLLMRequest,
		event.KindLLMResponse,
		event.KindTaskCompleted,
	}
	if !equalKinds(kinds, want) {
		t.Errorf("event kinds: got %v, want %v", kinds, want)
	}
}

// TestRun_LLMRequestRecordsContextSize (M372, SPEC-10 §3.5): the llm.request
// event records the assembled context size and a per-role breakdown (including
// the separately-sent system prompt) — the context-observability foundation for
// "how big was the context and where did it come from".
func TestRun_LLMRequestRecordsContextSize(t *testing.T) {
	b, j := newTestBus(t)
	const system = "you are a helpful assistant"
	const task = "summarize the codebase"
	if _, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider: mock.New(mock.FinalText("done")), Bus: b,
		Actor: "agent-1", CorrelationID: "corr-ctx", System: system,
	}, task); err != nil {
		t.Fatalf("Run: %v", err)
	}

	found := false
	_ = j.Range(func(e *event.Event) error {
		if e.Kind != event.KindLLMRequest {
			return nil
		}
		found = true
		var p struct {
			ContextChars int            `json:"context_chars"`
			ByRole       map[string]int `json:"context_by_role"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatalf("unmarshal llm.request: %v", err)
		}
		if p.ByRole["system"] != len(system) {
			t.Errorf("context_by_role[system] = %d, want %d", p.ByRole["system"], len(system))
		}
		if p.ByRole["user"] != len(task) {
			t.Errorf("context_by_role[user] = %d, want %d", p.ByRole["user"], len(task))
		}
		if p.ContextChars != len(system)+len(task) {
			t.Errorf("context_chars = %d, want %d", p.ContextChars, len(system)+len(task))
		}
		return nil
	})
	if !found {
		t.Fatal("no llm.request event was published")
	}
}

// TestRun_TaskCompletedCarriesAnswer — the run's final text is journaled on
// task.completed (M51) so `agt runs show` can display what the run produced.
func TestRun_TaskCompletedCarriesAnswer(t *testing.T) {
	b, j := newTestBus(t)
	const final = "The module is github.com/agezt/agezt."
	prov := mock.New(mock.FinalText(final))

	if _, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider: prov, Bus: b, Actor: "agent-1", CorrelationID: "corr-ans",
	}, "what is the module?"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var answer string
	var found bool
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindTaskCompleted {
			var p struct {
				Answer string `json:"answer"`
				Chars  int    `json:"chars"`
			}
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				t.Fatalf("unmarshal task.completed: %v", err)
			}
			answer = p.Answer
			found = true
			if p.Chars != len(final) {
				t.Errorf("chars = %d want %d", p.Chars, len(final))
			}
		}
		return nil
	})
	if !found {
		t.Fatal("no task.completed event")
	}
	if answer != final {
		t.Errorf("journaled answer = %q want %q", answer, final)
	}
}

// TestRun_AnswerTruncatedInJournal — a pathologically long answer is capped in
// the journal (M51) with a marker, while the FULL text is returned to the caller.
func TestRun_AnswerTruncatedInJournal(t *testing.T) {
	b, j := newTestBus(t)
	long := strings.Repeat("x", 20_000) // > the 8192-rune journal cap
	prov := mock.New(mock.FinalText(long))

	got, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider: prov, Bus: b, Actor: "agent-1", CorrelationID: "corr-long",
	}, "dump")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != long {
		t.Errorf("caller should get the FULL answer (%d chars), got %d", len(long), len(got))
	}

	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindTaskCompleted {
			var p struct {
				Answer string `json:"answer"`
				Chars  int    `json:"chars"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			if !strings.HasSuffix(p.Answer, "…[truncated]") {
				t.Errorf("journaled answer should be truncated with a marker; got %d chars ending %q",
					len([]rune(p.Answer)), tail(p.Answer))
			}
			if len([]rune(p.Answer)) > 8192+len([]rune("…[truncated]")) {
				t.Errorf("journaled answer too long: %d runes", len([]rune(p.Answer)))
			}
			if p.Chars != len(long) {
				t.Errorf("chars must record the TRUE length %d, got %d", len(long), p.Chars)
			}
		}
		return nil
	})
}

// tail returns the last few runes of s for error messages.
func tail(s string) string {
	r := []rune(s)
	if len(r) > 16 {
		return string(r[len(r)-16:])
	}
	return s
}

func TestRun_ToolCallRoundtrip(t *testing.T) {
	b, j := newTestBus(t)
	prov := mock.New(
		mock.ToolUse("call-1", "shell", map[string]string{"command": "echo hello"}),
		mock.FinalText("The shell printed 'hello'."),
	)
	sh := shell.New()

	got, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider:      prov,
		Tools:         map[string]agent.Tool{"shell": sh},
		Bus:           b,
		Actor:         "agent-1",
		CorrelationID: "corr-shell",
	}, "use shell to say hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(got, "hello") {
		t.Errorf("final answer should mention 'hello': %q", got)
	}

	var kinds []event.Kind
	_ = j.Range(func(e *event.Event) error {
		kinds = append(kinds, e.Kind)
		return nil
	})
	// When no Policy is configured the loop still publishes a
	// policy.decision event for each ToolCall (allow + "no policy
	// configured") so the journal is honest about the gating posture.
	want := []event.Kind{
		event.KindTaskReceived,
		event.KindLLMRequest, event.KindLLMResponse,
		event.KindPolicyDecision, event.KindToolInvoked, event.KindToolResult,
		event.KindLLMRequest, event.KindLLMResponse,
		event.KindTaskCompleted,
	}
	if !equalKinds(kinds, want) {
		t.Errorf("event kinds:\n  got  %v\n  want %v", kinds, want)
	}
}

func TestRun_PolicyDeny_SkipsToolInvoke(t *testing.T) {
	b, j := newTestBus(t)
	prov := mock.New(
		mock.ToolUse("c1", "shell", map[string]string{"command": "echo nope"}),
		mock.FinalText("the call was denied; proceeding without it"),
	)
	denyAll := func(_ context.Context, _ agent.ToolCall) agent.PolicyVerdict {
		return agent.PolicyVerdict{
			Allow:      false,
			Capability: "shell",
			Reason:     "test policy: deny everything",
			HardDenied: true,
		}
	}
	ans, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider:      prov,
		Tools:         map[string]agent.Tool{"shell": shell.New()},
		Bus:           b,
		Actor:         "agent-deny",
		CorrelationID: "corr-deny",
		Policy:        denyAll,
	}, "run shell")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(ans, "denied") {
		t.Errorf("final ans should reflect denial: %q", ans)
	}

	// Critical: tool.invoked must be ABSENT (the call never ran) and the
	// tool.result must carry the deny reason.
	var sawInvoked bool
	var lastToolResultOutput string
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindToolInvoked {
			sawInvoked = true
		}
		if e.Kind == event.KindToolResult {
			// decode payload's "output" field
			var p struct {
				Output string `json:"output"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			lastToolResultOutput = p.Output
		}
		return nil
	})
	if sawInvoked {
		t.Error("tool.invoked must NOT be published when policy denies")
	}
	if !strings.Contains(lastToolResultOutput, "denied by policy") {
		t.Errorf("tool.result missing denial; got %q", lastToolResultOutput)
	}
}

// M605: a tool the policy hard-denies is dropped from the set offered to the
// model on the next iteration — so the model can't keep burning iterations on a
// call that will always be refused. Observable via llm.request's "tools" count:
// it should fall from 1 (shell offered) to 0 (shell dropped after the denial).
func TestRun_HardDeniedTool_DroppedFromLaterOffers(t *testing.T) {
	b, j := newTestBus(t)
	prov := mock.New(
		mock.ToolUse("c1", "shell", map[string]string{"command": "echo nope"}),
		mock.FinalText("ok, proceeding without it"),
	)
	denyShell := func(_ context.Context, _ agent.ToolCall) agent.PolicyVerdict {
		return agent.PolicyVerdict{Allow: false, Capability: "shell", Reason: "denied", HardDenied: true}
	}
	if _, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider:      prov,
		Tools:         map[string]agent.Tool{"shell": shell.New()},
		Bus:           b,
		Actor:         "agent-drop",
		CorrelationID: "corr-drop",
		Policy:        denyShell,
	}, "run shell"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var toolCounts []int
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindLLMRequest {
			var p struct {
				Tools int `json:"tools"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			toolCounts = append(toolCounts, p.Tools)
		}
		return nil
	})
	if len(toolCounts) < 2 {
		t.Fatalf("expected >=2 llm.request events, got %v", toolCounts)
	}
	if toolCounts[0] != 1 {
		t.Errorf("first iteration should offer the shell tool (1), got %d", toolCounts[0])
	}
	if last := toolCounts[len(toolCounts)-1]; last != 0 {
		t.Errorf("after a hard deny the shell tool should be dropped (0 offered), got %d", last)
	}
}

func TestRun_UnknownTool_RecordedNotFatal(t *testing.T) {
	b, _ := newTestBus(t)
	prov := mock.New(
		mock.ToolUse("call-1", "nonexistent", map[string]string{}),
		mock.FinalText("I tried but the tool was missing."),
	)
	got, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider:      prov,
		Tools:         map[string]agent.Tool{}, // no tools registered
		Bus:           b,
		Actor:         "agent-1",
		CorrelationID: "corr-x",
	}, "do something")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(got, "missing") {
		t.Errorf("expected final answer; got %q", got)
	}
}

func TestRun_HonorsContextCancel(t *testing.T) {
	// A provider that blocks until ctx is cancelled.
	blockingProv := &blockingProvider{released: make(chan struct{})}
	b, _ := newTestBus(t)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := agent.Run(ctx, agent.LoopConfig{
			Provider:      blockingProv,
			Bus:           b,
			Actor:         "agent-1",
			CorrelationID: "corr-halt",
		}, "do forever")
		done <- err
	}()

	// Give the run a tick to enter Provider.Complete, then halt.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("got err=%v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after ctx.Cancel")
	}
}

func TestRun_MaxIterStops(t *testing.T) {
	// Provider that always asks for another tool call → loop must hit MaxIter.
	b, _ := newTestBus(t)
	prov := &repeatingToolUseProvider{}

	_, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider:      prov,
		Tools:         map[string]agent.Tool{"shell": shell.New()},
		Bus:           b,
		Actor:         "agent-loop",
		CorrelationID: "corr-loop",
		MaxIter:       3,
	}, "loop forever")
	if !errors.Is(err, agent.ErrMaxIter) {
		t.Errorf("got err=%v, want ErrMaxIter", err)
	}
	if prov.calls != 3 {
		t.Errorf("provider called %d times, want 3", prov.calls)
	}
}

// failureReasonOf walks the journal and returns the reason tag of the
// single task.failed event (or "" with ok=false if none). M30 helper.
func failureReasonOf(t *testing.T, j interface {
	Range(func(*event.Event) error) error
}) (reason string, ok bool) {
	t.Helper()
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindTaskFailed {
			var p struct {
				Reason string `json:"reason"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			reason = p.Reason
			ok = true
		}
		return nil
	})
	return reason, ok
}

// TestRun_ProviderErrorEmitsTaskFailed — a provider that errors out must
// produce a terminal task.failed (reason=error) and NO task.completed, so
// `agt runs` shows a real failure instead of a phantom orphan (M30).
func TestRun_ProviderErrorEmitsTaskFailed(t *testing.T) {
	b, j := newTestBus(t)
	_, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider:      &errorProvider{},
		Bus:           b,
		Actor:         "agent-err",
		CorrelationID: "corr-err",
	}, "boom")
	if err == nil {
		t.Fatal("expected error from provider, got nil")
	}

	reason, ok := failureReasonOf(t, j)
	if !ok {
		t.Fatal("no task.failed event emitted on provider error")
	}
	if reason != "error" {
		t.Errorf("reason = %q want %q", reason, "error")
	}
	// And there must be no task.completed.
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindTaskCompleted {
			t.Errorf("unexpected task.completed on the error path")
		}
		return nil
	})
}

// TestRun_MaxIterEmitsTaskFailed — exhausting the iteration budget is a
// failure terminal with reason=max_iters.
func TestRun_MaxIterEmitsTaskFailed(t *testing.T) {
	b, j := newTestBus(t)
	_, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider:      &repeatingToolUseProvider{},
		Tools:         map[string]agent.Tool{"shell": shell.New()},
		Bus:           b,
		Actor:         "agent-loop",
		CorrelationID: "corr-loop",
		MaxIter:       2,
	}, "loop forever")
	if !errors.Is(err, agent.ErrMaxIter) {
		t.Fatalf("got err=%v, want ErrMaxIter", err)
	}
	reason, ok := failureReasonOf(t, j)
	if !ok {
		t.Fatal("no task.failed event emitted on max-iter")
	}
	if reason != "max_iters" {
		t.Errorf("reason = %q want %q", reason, "max_iters")
	}
}

// TestRun_CancelEmitsTaskFailed — a cancelled context terminates the run
// with reason=canceled (the operator-halt path).
func TestRun_CancelEmitsTaskFailed(t *testing.T) {
	blockingProv := &blockingProvider{released: make(chan struct{})}
	b, j := newTestBus(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_, _ = agent.Run(ctx, agent.LoopConfig{
			Provider:      blockingProv,
			Bus:           b,
			Actor:         "agent-cancel",
			CorrelationID: "corr-cancel",
		}, "wait")
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancel")
	}
	reason, ok := failureReasonOf(t, j)
	if !ok {
		t.Fatal("no task.failed event emitted on cancel")
	}
	if reason != "canceled" {
		t.Errorf("reason = %q want %q", reason, "canceled")
	}
}

// TestRun_SuccessEmitsNoTaskFailed — the happy path must NOT emit a
// task.failed (the defer no-ops on a nil error).
func TestRun_SuccessEmitsNoTaskFailed(t *testing.T) {
	b, j := newTestBus(t)
	_, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider:      mock.New(mock.FinalText("ok")),
		Bus:           b,
		Actor:         "agent-ok",
		CorrelationID: "corr-ok",
	}, "say ok")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, ok := failureReasonOf(t, j); ok {
		t.Error("task.failed emitted on a successful run")
	}
}

// blockingTool blocks Invoke until ctx is cancelled, then returns ctx.Err()
// — used to exercise the per-tool timeout (M34).
type blockingTool struct{}

func (blockingTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: "slow", Description: "blocks", InputSchema: json.RawMessage(`{"type":"object"}`)}
}
func (blockingTool) Invoke(ctx context.Context, _ json.RawMessage) (agent.Result, error) {
	<-ctx.Done()
	return agent.Result{}, ctx.Err()
}

// TestRun_ToolTimeoutFeedsErrorNotFailure — a tool that overruns its
// ToolTimeout yields an IsError result fed back to the model, and the RUN
// still completes (M34). Distinct from the per-run MaxDuration (M31), which
// fails the whole run.
func TestRun_ToolTimeoutFeedsErrorNotFailure(t *testing.T) {
	b, j := newTestBus(t)
	// Round 1: model asks for the slow tool. Round 2: it gives a final
	// answer (after seeing the timeout error result).
	prov := mock.New(
		mock.ToolUse("c1", "slow", map[string]string{}),
		mock.FinalText("gave up on the slow tool"),
	)
	ans, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider:      prov,
		Tools:         map[string]agent.Tool{"slow": blockingTool{}},
		Bus:           b,
		Actor:         "agent-tt",
		CorrelationID: "corr-tt",
		ToolTimeout:   30 * time.Millisecond,
	}, "use the slow tool")
	if err != nil {
		t.Fatalf("run should complete despite a tool timeout, got err=%v", err)
	}
	if ans != "gave up on the slow tool" {
		t.Errorf("ans = %q want final answer", ans)
	}

	// The tool.result must be an error mentioning the timeout, and there
	// must be NO task.failed (the run succeeded).
	var sawTimeoutResult, sawFailed bool
	_ = j.Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindToolResult:
			var p struct {
				Output string `json:"output"`
				Error  bool   `json:"error"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			if p.Error && strings.Contains(p.Output, "timeout") {
				sawTimeoutResult = true
			}
		case event.KindTaskFailed:
			sawFailed = true
		}
		return nil
	})
	if !sawTimeoutResult {
		t.Error("expected an is_error tool.result mentioning the timeout")
	}
	if sawFailed {
		t.Error("a per-tool timeout must not emit task.failed (run continues)")
	}
}

// TestRun_FastToolUnderTimeoutUnaffected — a tool that finishes within its
// ToolTimeout runs normally; the cap doesn't interfere.
func TestRun_FastToolUnderTimeoutUnaffected(t *testing.T) {
	b, _ := newTestBus(t)
	prov := mock.New(
		mock.ToolUse("c1", "shell", map[string]string{"command": "echo hi"}),
		mock.FinalText("done"),
	)
	ans, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider:      prov,
		Tools:         map[string]agent.Tool{"shell": shell.New()},
		Bus:           b,
		Actor:         "agent-ft",
		CorrelationID: "corr-ft",
		ToolTimeout:   5 * time.Second,
	}, "echo")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ans != "done" {
		t.Errorf("ans = %q want done", ans)
	}
}

// TestRun_RunCancelDuringToolFailsRun — if the RUN context is cancelled
// while a tool is executing, the run fails with context.Canceled rather
// than the tool timeout swallowing it into an error result and limping on
// (M34 must not mask run-level cancellation).
func TestRun_RunCancelDuringToolFailsRun(t *testing.T) {
	b, j := newTestBus(t)
	prov := mock.New(
		mock.ToolUse("c1", "slow", map[string]string{}),
		mock.FinalText("should never reach here"),
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, e := agent.Run(ctx, agent.LoopConfig{
			Provider:      prov,
			Tools:         map[string]agent.Tool{"slow": blockingTool{}},
			Bus:           b,
			Actor:         "agent-rc",
			CorrelationID: "corr-rc",
			ToolTimeout:   10 * time.Second, // long; the run cancel must win
		}, "use the slow tool")
		done <- e
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Errorf("got err=%v, want context.Canceled", e)
		}
	case <-time.After(time.Second):
		t.Fatal("run did not return after cancel during tool")
	}
	var reason string
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindTaskFailed {
			var p struct {
				Reason string `json:"reason"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			reason = p.Reason
		}
		return nil
	})
	if reason != "canceled" {
		t.Errorf("task.failed reason = %q want canceled", reason)
	}
}

func TestRun_RequiresProviderAndBusAndActor(t *testing.T) {
	b, _ := newTestBus(t)
	_, err := agent.Run(context.Background(), agent.LoopConfig{Bus: b, Actor: "a"}, "x")
	if err == nil || !strings.Contains(err.Error(), "provider required") {
		t.Errorf("missing provider: got %v", err)
	}
	_, err = agent.Run(context.Background(), agent.LoopConfig{Provider: mock.New(), Actor: "a"}, "x")
	if err == nil || !strings.Contains(err.Error(), "bus required") {
		t.Errorf("missing bus: got %v", err)
	}
	_, err = agent.Run(context.Background(), agent.LoopConfig{Provider: mock.New(), Bus: b}, "x")
	if err == nil || !strings.Contains(err.Error(), "actor required") {
		t.Errorf("missing actor: got %v", err)
	}
}

// ----- helpers -----

func equalKinds(a, b []event.Kind) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// errorProvider always fails its Complete call — drives the M30
// task.failed(reason=error) path.
type errorProvider struct{}

func (e *errorProvider) Name() string { return "erroring" }
func (e *errorProvider) Complete(_ context.Context, _ agent.CompletionRequest) (*agent.CompletionResponse, error) {
	return nil, errors.New("simulated upstream failure")
}

type blockingProvider struct {
	released chan struct{}
}

func (b *blockingProvider) Name() string { return "blocking" }
func (b *blockingProvider) Complete(ctx context.Context, _ agent.CompletionRequest) (*agent.CompletionResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-b.released:
		return &agent.CompletionResponse{
			Message:    agent.Message{Role: agent.RoleAssistant, Content: "done"},
			StopReason: agent.StopEndTurn,
		}, nil
	}
}

type repeatingToolUseProvider struct{ calls int }

func (r *repeatingToolUseProvider) Name() string { return "repeating" }
func (r *repeatingToolUseProvider) Complete(_ context.Context, _ agent.CompletionRequest) (*agent.CompletionResponse, error) {
	r.calls++
	return &agent.CompletionResponse{
		Message: agent.Message{
			Role: agent.RoleAssistant,
			ToolCalls: []agent.ToolCall{{
				ID:    "call-x",
				Name:  "shell",
				Input: json.RawMessage(`{"command":"true"}`),
			}},
		},
		StopReason: agent.StopToolUse,
	}, nil
}

// streamProv is a minimal StreamingProvider used to test the agent
// loop's streaming dispatch. It feeds a pre-set sequence of text
// fragments through onChunk and returns the assembled response.
type streamProv struct {
	chunks     []string
	reasoning  []string // M317: optional reasoning deltas emitted before the text
	stopReason agent.StopReason
	gotInvoked bool
	chunkErr   error // when non-nil, returned from onChunk on first call
}

func (p *streamProv) Name() string { return "stream-mock" }

func (p *streamProv) Complete(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	// Must exist for type-assertion; should NOT be called when
	// StreamingProvider is detected.
	p.gotInvoked = true
	return &agent.CompletionResponse{
		Message:    agent.Message{Role: agent.RoleAssistant, Content: strings.Join(p.chunks, "")},
		StopReason: p.stopReason,
	}, nil
}

func (p *streamProv) CompleteStream(ctx context.Context, req agent.CompletionRequest, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	for _, r := range p.reasoning {
		if err := onChunk(agent.Chunk{ReasoningDelta: r}); err != nil {
			return nil, err
		}
	}
	for i, c := range p.chunks {
		if err := onChunk(agent.Chunk{TextDelta: c}); err != nil {
			return nil, err
		}
		if i == 0 && p.chunkErr != nil {
			return nil, p.chunkErr
		}
	}
	stop := p.stopReason
	if stop == "" {
		stop = agent.StopEndTurn
	}
	return &agent.CompletionResponse{
		Message:          agent.Message{Role: agent.RoleAssistant, Content: strings.Join(p.chunks, "")},
		ReasoningContent: strings.Join(p.reasoning, ""),
		StopReason:       stop,
		Usage:            agent.Usage{InputTokens: 10, OutputTokens: 5},
	}, nil
}

// TestRun_PublishesReasoningEvents (M317): a reasoning model's chain-of-thought
// deltas are published as ephemeral llm.reasoning events (visible live, not
// durably journaled), distinct from the answer's llm.token events.
func TestRun_PublishesReasoningEvents(t *testing.T) {
	b, _ := newTestBus(t)
	prov := &streamProv{
		chunks:     []string{"42"},
		reasoning:  []string{"Let me ", "think."},
		stopReason: agent.StopEndTurn,
	}
	sub, err := b.Subscribe(">", 64)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Cancel()

	if _, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider: prov, Bus: b, Actor: "a", CorrelationID: "c",
	}, "go"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	deadline := time.After(time.Second)
	var reasoning strings.Builder
	gotResponse := false
collect:
	for {
		select {
		case ev := <-sub.C:
			switch ev.Kind {
			case event.KindLLMReasoning:
				if !ev.IsEphemeral() {
					t.Errorf("llm.reasoning must be ephemeral (Hash=\"\"): %+v", ev)
				}
				var p struct {
					Text string `json:"text"`
				}
				_ = json.Unmarshal(ev.Payload, &p)
				reasoning.WriteString(p.Text)
			case event.KindLLMResponse:
				gotResponse = true
				break collect
			}
		case <-deadline:
			break collect
		}
	}
	if !gotResponse {
		t.Fatal("never saw llm.response")
	}
	if reasoning.String() != "Let me think." {
		t.Errorf("reassembled reasoning=%q want 'Let me think.'", reasoning.String())
	}
}

// nonStreamReasoningProvider is a bare Provider (no CompleteStream) whose
// non-streaming Complete returns reasoning whole, as a real non-streaming
// reasoning-model call would.
type nonStreamReasoningProvider struct {
	answer    string
	reasoning string
}

func (p *nonStreamReasoningProvider) Name() string { return "nonstream-reasoning" }
func (p *nonStreamReasoningProvider) Complete(_ context.Context, _ agent.CompletionRequest) (*agent.CompletionResponse, error) {
	return &agent.CompletionResponse{
		Message:          agent.Message{Role: agent.RoleAssistant, Content: p.answer},
		ReasoningContent: p.reasoning,
		StopReason:       agent.StopEndTurn,
		Usage:            agent.Usage{InputTokens: 10, OutputTokens: 5},
	}, nil
}

// TestRun_PublishesReasoningEvents_NonStreaming (M325): a non-streaming provider
// returns reasoning whole (no deltas), so the loop must emit it as a single
// ephemeral llm.reasoning event — otherwise the chain of thought would be invisible
// to every consumer (pulse, ACP, the OpenAI API) for non-streaming runs.
func TestRun_PublishesReasoningEvents_NonStreaming(t *testing.T) {
	b, _ := newTestBus(t)
	prov := &nonStreamReasoningProvider{answer: "42", reasoning: "6*7 = 42."}
	sub, err := b.Subscribe(">", 64)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Cancel()

	if _, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider: prov, Bus: b, Actor: "a", CorrelationID: "c",
	}, "go"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	deadline := time.After(time.Second)
	var reasoning strings.Builder
	gotResponse, gotReasoningEvent := false, false
collect:
	for {
		select {
		case ev := <-sub.C:
			switch ev.Kind {
			case event.KindLLMReasoning:
				gotReasoningEvent = true
				if !ev.IsEphemeral() {
					t.Errorf("llm.reasoning must be ephemeral (Hash=\"\"): %+v", ev)
				}
				var p struct {
					Text string `json:"text"`
				}
				_ = json.Unmarshal(ev.Payload, &p)
				reasoning.WriteString(p.Text)
			case event.KindLLMResponse:
				gotResponse = true
				break collect
			}
		case <-deadline:
			break collect
		}
	}
	if !gotResponse {
		t.Fatal("never saw llm.response")
	}
	if !gotReasoningEvent {
		t.Fatal("non-streaming reasoning was not published as an llm.reasoning event")
	}
	if reasoning.String() != "6*7 = 42." {
		t.Errorf("reasoning event text=%q want '6*7 = 42.'", reasoning.String())
	}
}

func TestRun_UsesStreamingWhenAvailable(t *testing.T) {
	b, _ := newTestBus(t)
	prov := &streamProv{
		chunks:     []string{"Hel", "lo, ", "world."},
		stopReason: agent.StopEndTurn,
	}

	// Subscribe before Run so we don't miss the early ephemeral chunks.
	// `>` pattern catches everything; we filter for KindLLMToken below.
	sub, err := b.Subscribe(">", 64)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Cancel()

	got, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider:      prov,
		Bus:           b,
		Actor:         "stream-actor",
		CorrelationID: "corr-stream",
	}, "stream me")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != "Hello, world." {
		t.Errorf("got %q want %q", got, "Hello, world.")
	}

	// Critical: streaming path was used, not Complete.
	if prov.gotInvoked {
		t.Error("Complete was called; expected CompleteStream to fully service the request")
	}

	// Drain subscription, collect KindLLMToken events. Each must be
	// ephemeral (Hash="") and carry the correct text fragment.
	deadline := time.After(time.Second)
	var tokenFragments []string
	gotFinalResponse := false
collect:
	for {
		select {
		case ev := <-sub.C:
			if ev.Kind == event.KindLLMToken {
				if !ev.IsEphemeral() {
					t.Errorf("KindLLMToken event reports !IsEphemeral: %+v", ev)
				}
				var p struct {
					Text string `json:"text"`
					Iter int    `json:"iter"`
				}
				_ = json.Unmarshal(ev.Payload, &p)
				if p.Text == "" {
					t.Errorf("KindLLMToken with empty text: %+v", ev)
				}
				tokenFragments = append(tokenFragments, p.Text)
			}
			if ev.Kind == event.KindLLMResponse {
				if ev.IsEphemeral() {
					t.Error("KindLLMResponse reported IsEphemeral=true; the canonical record must be durable")
				}
				gotFinalResponse = true
			}
			if ev.Kind == event.KindTaskCompleted {
				break collect
			}
		case <-deadline:
			break collect
		}
	}
	want := []string{"Hel", "lo, ", "world."}
	if len(tokenFragments) != len(want) {
		t.Fatalf("got %d KindLLMToken events, want %d (%v)", len(tokenFragments), len(want), tokenFragments)
	}
	for i, frag := range tokenFragments {
		if frag != want[i] {
			t.Errorf("fragment[%d] = %q, want %q", i, frag, want[i])
		}
	}
	if !gotFinalResponse {
		t.Error("KindLLMResponse not seen — the assembled durable record is required")
	}
}

func TestRun_StreamingFallsBackToCompleteForNonStreamingProvider(t *testing.T) {
	// A bare Provider (no CompleteStream) must still work — the
	// type assertion should fail cleanly and Complete should run.
	// Uses the existing mock provider which is non-streaming.
	b, _ := newTestBus(t)
	prov := mock.New(mock.FinalText("Plain old text."))

	got, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider:      prov,
		Bus:           b,
		Actor:         "non-stream-actor",
		CorrelationID: "corr-non-stream",
	}, "plain")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != "Plain old text." {
		t.Errorf("got %q want %q", got, "Plain old text.")
	}
}

// countingTool records how many times it actually ran (the loop guard should
// stop the model from executing it more than the cap with identical input).
type countingTool struct{ calls int }

func (c *countingTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: "echo", Description: "echo", InputSchema: json.RawMessage(`{"type":"object"}`)}
}
func (c *countingTool) Invoke(_ context.Context, _ json.RawMessage) (agent.Result, error) {
	c.calls++
	return agent.Result{Output: "ok"}, nil
}

// repeatEchoProvider always asks for the same echo call with identical input.
type repeatEchoProvider struct {
	varyInput bool
	n         int
}

func (p *repeatEchoProvider) Name() string { return "repeat-echo" }
func (p *repeatEchoProvider) Complete(_ context.Context, _ agent.CompletionRequest) (*agent.CompletionResponse, error) {
	p.n++
	input := `{"x":1}`
	if p.varyInput {
		input = fmt.Sprintf(`{"x":%d}`, p.n) // distinct each call
	}
	return &agent.CompletionResponse{
		Message: agent.Message{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{
			ID: "call-x", Name: "echo", Input: json.RawMessage(input),
		}}},
		StopReason: agent.StopToolUse,
	}, nil
}

func TestRun_LoopGuard_CapsIdenticalCalls(t *testing.T) {
	b, _ := newTestBus(t)
	tool := &countingTool{}
	_, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider: &repeatEchoProvider{}, Tools: map[string]agent.Tool{"echo": tool},
		Bus: b, Actor: "a", CorrelationID: "c",
		MaxIter: 12, MaxIdenticalToolCalls: 3,
	}, "loop")
	if !errors.Is(err, agent.ErrMaxIter) {
		t.Errorf("got err=%v, want ErrMaxIter", err)
	}
	// Executed at most the cap, not all 12 iterations.
	if tool.calls != 3 {
		t.Errorf("tool executed %d times, want 3 (loop guard cap)", tool.calls)
	}
}

func TestRun_LoopGuard_DistinctInputsNotCapped(t *testing.T) {
	b, _ := newTestBus(t)
	tool := &countingTool{}
	_, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider: &repeatEchoProvider{varyInput: true}, Tools: map[string]agent.Tool{"echo": tool},
		Bus: b, Actor: "a", CorrelationID: "c",
		MaxIter: 6, MaxIdenticalToolCalls: 3,
	}, "loop")
	if !errors.Is(err, agent.ErrMaxIter) {
		t.Errorf("got err=%v, want ErrMaxIter", err)
	}
	// Each call has distinct input, so the guard never fires — all 6 run.
	if tool.calls != 6 {
		t.Errorf("tool executed %d times, want 6 (distinct inputs not capped)", tool.calls)
	}
}

func TestRun_LoopGuard_DisabledByNegative(t *testing.T) {
	b, _ := newTestBus(t)
	tool := &countingTool{}
	_, _ = agent.Run(context.Background(), agent.LoopConfig{
		Provider: &repeatEchoProvider{}, Tools: map[string]agent.Tool{"echo": tool},
		Bus: b, Actor: "a", CorrelationID: "c",
		MaxIter: 5, MaxIdenticalToolCalls: -1, // disabled
	}, "loop")
	if tool.calls != 5 {
		t.Errorf("tool executed %d times, want 5 (guard disabled)", tool.calls)
	}
}
