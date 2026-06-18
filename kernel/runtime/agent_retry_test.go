// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
)

type failThenOKProvider struct {
	calls int
}

func (p *failThenOKProvider) Name() string { return "fail-then-ok" }

func (p *failThenOKProvider) Complete(context.Context, agent.CompletionRequest) (*agent.CompletionResponse, error) {
	p.calls++
	if p.calls == 1 {
		return nil, errors.New("transient provider failure")
	}
	if p.calls > 2 {
		return &agent.CompletionResponse{
			Message:    agent.Message{Role: agent.RoleAssistant, Content: `{"complete":true,"gap":""}`},
			StopReason: agent.StopEndTurn,
		}, nil
	}
	return &agent.CompletionResponse{
		Message:    agent.Message{Role: agent.RoleAssistant, Content: "recovered"},
		StopReason: agent.StopEndTurn,
	}, nil
}

func TestRunWithRetry_ProfilePolicyRetriesWholeRun(t *testing.T) {
	prov := &failThenOKProvider{}
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: prov})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{Slug: "healer"})
	policy := roster.RetryPolicy{MaxAttempts: 2, Backoff: "exponential", BaseDelaySec: 0, MaxDelaySec: 5, RetryOn: []string{"error", "timeout"}}
	ans, err := k.RunWithRetry(ctx, "corr-retry", "do the thing", policy)
	if err != nil {
		t.Fatalf("RunWithRetry: %v", err)
	}
	if ans != "recovered" {
		t.Fatalf("answer = %q, want recovered", ans)
	}
	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", prov.calls)
	}

	var retryPayload map[string]any
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindAgentRetry && e.CorrelationID == "corr-retry" {
			_ = json.Unmarshal(e.Payload, &retryPayload)
		}
		return nil
	})
	if retryPayload == nil {
		t.Fatal("agent.retry event missing")
	}
	if retryPayload["agent"] != "healer" || int(retryPayload["attempt"].(float64)) != 1 ||
		int(retryPayload["next_attempt"].(float64)) != 2 || int(retryPayload["max_attempts"].(float64)) != 2 ||
		retryPayload["reason"] != "error" {
		t.Fatalf("agent.retry payload missing retry decision fields: %v", retryPayload)
	}
	if retryPayload["backoff"] != "exponential" || int(retryPayload["base_delay_sec"].(float64)) != 0 ||
		int(retryPayload["max_delay_sec"].(float64)) != 5 {
		t.Fatalf("agent.retry payload missing policy timing fields: %v", retryPayload)
	}
	rawRetryOn, _ := retryPayload["retry_on"].([]any)
	var retryOn []string
	for _, v := range rawRetryOn {
		retryOn = append(retryOn, v.(string))
	}
	if !reflect.DeepEqual(retryOn, []string{"error", "timeout"}) {
		t.Fatalf("retry_on = %v, want [error timeout]", retryOn)
	}
}

func TestRunAssured_UsesAgentProfileRetryPolicyInsideSemanticAttempts(t *testing.T) {
	prov := &failThenOKProvider{}
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: prov})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{
		Slug: "healer",
		RetryPolicy: &roster.RetryPolicy{
			MaxAttempts: 2,
			RetryOn:     []string{"error", "timeout"},
		},
	})
	ans, assured, err := k.RunAssured(ctx, "corr-assured-retry", "do the thing", 1)
	if err != nil {
		t.Fatalf("RunAssured: %v", err)
	}
	if ans != "recovered" || !assured.Complete {
		t.Fatalf("assured result answer=%q complete=%v history=%+v", ans, assured.Complete, assured.History)
	}
	if prov.calls != 3 {
		t.Fatalf("provider calls = %d, want failed run + retry run + verifier", prov.calls)
	}

	var retryPayload map[string]any
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindAgentRetry && e.CorrelationID == "corr-assured-retry" {
			_ = json.Unmarshal(e.Payload, &retryPayload)
		}
		return nil
	})
	if retryPayload == nil {
		t.Fatal("agent.retry event missing for assured run")
	}
	if retryPayload["agent"] != "healer" || int(retryPayload["next_attempt"].(float64)) != 2 {
		t.Fatalf("agent.retry payload wrong for assured run: %v", retryPayload)
	}
}
