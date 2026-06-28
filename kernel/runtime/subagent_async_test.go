// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// asyncScriptProvider routes completions by ROLE instead of call order: with
// an async child running, the child's and the lead's provider calls interleave
// nondeterministically, so a sequential script would flake. Child calls are
// recognised by the sub-agent system preamble; lead calls walk the script.
type asyncScriptProvider struct {
	mu          sync.Mutex
	leadIdx     int
	lead        []func(req agent.CompletionRequest) agent.CompletionResponse
	childBlocks bool   // child call parks on ctx (for orphan-cancel tests)
	childText   string // child's final answer otherwise
}

func (p *asyncScriptProvider) Name() string { return "async-script" }

func (p *asyncScriptProvider) Complete(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	if strings.Contains(req.System, "focused sub-agent") {
		if p.childBlocks {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		// A little real work time, so the lead genuinely waits in delegate_await.
		time.Sleep(50 * time.Millisecond)
		r := mock.FinalText(p.childText)
		return &r, nil
	}
	p.mu.Lock()
	if p.leadIdx >= len(p.lead) {
		p.mu.Unlock()
		return nil, mock.ErrExhausted
	}
	f := p.lead[p.leadIdx]
	p.leadIdx++
	p.mu.Unlock()
	r := f(req)
	return &r, nil
}

var spawnIDRe = regexp.MustCompile(`spawned sub-agent (\S+) `)

// lastToolMessage returns the content of the most recent tool-role message.
func lastToolMessage(req agent.CompletionRequest) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == agent.RoleTool {
			return req.Messages[i].Content
		}
	}
	return ""
}

func openAsyncKernel(t *testing.T, prov agent.Provider) *runtime.Kernel {
	t.Helper()
	k, err := runtime.Open(runtime.Config{
		BaseDir:          t.TempDir(),
		Provider:         prov,
		SubAgentTool:     true,
		SubAgentMaxDepth: 1,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

// TestSubAgent_AsyncSpawnAndAwait (M881): delegate(async=true) returns a
// spawn_id immediately; delegate_await collects the child's result; the
// journal carries subagent.spawned{async} and a push-style
// subagent.completed{ok} under the parent correlation.
func TestSubAgent_AsyncSpawnAndAwait(t *testing.T) {
	prov := &asyncScriptProvider{
		childText: "child says hi",
		lead: []func(agent.CompletionRequest) agent.CompletionResponse{
			func(agent.CompletionRequest) agent.CompletionResponse {
				return testToolUse("a1", "delegate", map[string]any{"task": "t1", "async": true})
			},
			func(req agent.CompletionRequest) agent.CompletionResponse {
				m := spawnIDRe.FindStringSubmatch(lastToolMessage(req))
				if m == nil {
					return mock.FinalText("BUG: no spawn id in tool result")
				}
				return testToolUse("a2", "delegate_await", map[string]any{"spawn_id": m[1]})
			},
			func(req agent.CompletionRequest) agent.CompletionResponse {
				return mock.FinalText("lead got: " + lastToolMessage(req))
			},
		},
	}
	k := openAsyncKernel(t, prov)
	col := &collector{}
	col.watch(k)

	ans, _, err := k.Run(context.Background(), "fan out async")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ans != "lead got: child says hi" {
		t.Errorf("answer = %q, want the awaited child result", ans)
	}
	time.Sleep(100 * time.Millisecond)

	spawned := col.ofKind(event.KindSubAgentSpawned)
	if len(spawned) != 1 {
		t.Fatalf("subagent.spawned events = %d, want 1", len(spawned))
	}
	var sp struct {
		Async bool `json:"async"`
	}
	_ = json.Unmarshal(spawned[0].Payload, &sp)
	if !sp.Async {
		t.Error("subagent.spawned payload missing async:true")
	}

	completed := col.ofKind(event.KindSubAgentCompleted)
	if len(completed) != 1 {
		t.Fatalf("subagent.completed events = %d, want 1", len(completed))
	}
	var cp struct {
		OK    bool `json:"ok"`
		Async bool `json:"async"`
	}
	_ = json.Unmarshal(completed[0].Payload, &cp)
	if !cp.OK || !cp.Async {
		t.Errorf("subagent.completed payload = %s, want ok:true async:true", completed[0].Payload)
	}
}

// TestSubAgent_AsyncOrphanCancelledAtRunEnd (M881): an async child the lead
// never awaits is cancelled when the lead's run ends — it must not outlive
// its delegation tree. The child parks on its context; the cancel surfaces as
// subagent.completed{ok:false}.
func TestSubAgent_AsyncOrphanCancelledAtRunEnd(t *testing.T) {
	prov := &asyncScriptProvider{
		childBlocks: true,
		lead: []func(agent.CompletionRequest) agent.CompletionResponse{
			func(agent.CompletionRequest) agent.CompletionResponse {
				return testToolUse("a1", "delegate", map[string]any{"task": "t1", "async": true})
			},
			func(agent.CompletionRequest) agent.CompletionResponse {
				return mock.FinalText("lead done without awaiting")
			},
		},
	}
	k := openAsyncKernel(t, prov)
	col := &collector{}
	col.watch(k)

	ans, _, err := k.Run(context.Background(), "spawn and forget")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ans != "lead done without awaiting" {
		t.Errorf("answer = %q", ans)
	}

	// The orphan is cancelled by the run's cleanup; its goroutine then
	// journals subagent.completed{ok:false}. Poll briefly.
	deadline := time.Now().Add(3 * time.Second)
	for {
		completed := col.ofKind(event.KindSubAgentCompleted)
		if len(completed) == 1 {
			var cp struct {
				OK    bool   `json:"ok"`
				Error string `json:"error"`
			}
			_ = json.Unmarshal(completed[0].Payload, &cp)
			if cp.OK {
				t.Errorf("orphaned child completed ok=true, want a cancellation failure (payload %s)", completed[0].Payload)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("no subagent.completed for the orphaned async child within 3s — it was not cancelled")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
