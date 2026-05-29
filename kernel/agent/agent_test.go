// SPDX-License-Identifier: MIT

package agent_test

import (
	"context"
	"encoding/json"
	"errors"
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
	stopReason agent.StopReason
	gotInvoked bool
	gotIter    int
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
		Message:    agent.Message{Role: agent.RoleAssistant, Content: strings.Join(p.chunks, "")},
		StopReason: stop,
		Usage:      agent.Usage{InputTokens: 10, OutputTokens: 5},
	}, nil
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
