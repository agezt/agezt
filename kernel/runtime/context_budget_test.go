// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// allowDump builds an Edict engine that permits the dump tool, so its full
// (large) output actually reaches the transcript instead of a short policy
// denial — otherwise the only elidable content is a fixed ~80-char denial
// message and compaction can't reclaim against it.
func allowDump() *edict.Engine {
	return edict.New(edict.Options{
		Levels: map[edict.Capability]edict.TrustLevel{"dump": edict.LevelAllow},
	})
}

// dumpTool returns a fixed large output so context builds up across rounds.
type dumpTool struct{ out string }

func (d dumpTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: "dump", Description: "emit a blob", InputSchema: json.RawMessage(`{"type":"object"}`)}
}
func (d dumpTool) Invoke(context.Context, json.RawMessage) (agent.Result, error) {
	return agent.Result{Output: d.out}, nil
}

// TestRun_AutoContextBudgetFromCatalog: with ContextBudgetAuto and a catalog
// model whose context window is tiny, the kernel derives a small budget and the
// run compacts (SPEC-10 §3 / M394). Proves the auto-derivation wires end to end.
func TestRun_AutoContextBudgetFromCatalog(t *testing.T) {
	cat := catalog.NewEmpty()
	cat.Providers["p"] = &catalog.Provider{
		ID: "p",
		Models: map[string]*catalog.Model{
			// 10-token window → AutoContextBudgetChars = 10*4*0.5 = 20 chars.
			"mockmodel": {ID: "mockmodel", Limit: catalog.Limit{Context: 10}},
		},
	}

	prov := mock.New(
		testToolUse("c1", "dump", map[string]any{}),
		testToolUse("c2", "dump", map[string]any{}),
		testToolUse("c3", "dump", map[string]any{}),
		mock.FinalText("done"),
	)
	k, err := runtime.Open(runtime.Config{
		BaseDir:           t.TempDir(),
		Provider:          prov,
		Model:             "mockmodel",
		Catalog:           cat,
		ContextBudgetAuto: true,
		Edict:             allowDump(),
		Tools:             map[string]agent.Tool{"dump": dumpTool{out: strings.Repeat("Z", 2000)}},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, _, err := k.Run(context.Background(), "dump repeatedly"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	n := 0
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindContextCompacted {
			n++
		}
		return nil
	})
	if n == 0 {
		t.Error("auto budget should have triggered at least one context.compacted")
	}
}

// TestRun_ContextProtectFirstPlumbsThrough: with a tight explicit budget, a run
// normally compacts; setting ContextProtectFirst high enough to shield every
// elidable message suppresses that compaction — proving the field reaches the
// loop (SPEC-10 §3 / M395). If it weren't plumbed (default 0) the run would still
// compact, so the absence of context.compacted is the lock-in.
func TestRun_ContextProtectFirstPlumbsThrough(t *testing.T) {
	newProv := func() *mock.Provider {
		return mock.New(
			testToolUse("c1", "dump", map[string]any{}),
			testToolUse("c2", "dump", map[string]any{}),
			testToolUse("c3", "dump", map[string]any{}),
			mock.FinalText("done"),
		)
	}
	tools := map[string]agent.Tool{"dump": dumpTool{out: strings.Repeat("Z", 2000)}}

	compactedCount := func(k *runtime.Kernel) int {
		n := 0
		_ = k.Journal().Range(func(e *event.Event) error {
			if e.Kind == event.KindContextCompacted {
				n++
			}
			return nil
		})
		return n
	}

	// Control: tight budget, no first-protection → compacts.
	kc, err := runtime.Open(runtime.Config{
		BaseDir: t.TempDir(), Provider: newProv(), Model: "m",
		ContextBudget: 200, Tools: tools, Edict: allowDump(),
	})
	if err != nil {
		t.Fatalf("Open control: %v", err)
	}
	t.Cleanup(func() { kc.Close() })
	if _, _, err := kc.Run(context.Background(), "dump"); err != nil {
		t.Fatalf("Run control: %v", err)
	}
	if compactedCount(kc) == 0 {
		t.Fatal("control with no protect-first should have compacted")
	}

	// Same tight budget, but shield the first 100 messages → nothing elidable.
	kp, err := runtime.Open(runtime.Config{
		BaseDir: t.TempDir(), Provider: newProv(), Model: "m",
		ContextBudget: 200, ContextProtectFirst: 100, Tools: tools, Edict: allowDump(),
	})
	if err != nil {
		t.Fatalf("Open protected: %v", err)
	}
	t.Cleanup(func() { kp.Close() })
	if _, _, err := kp.Run(context.Background(), "dump"); err != nil {
		t.Fatalf("Run protected: %v", err)
	}
	if got := compactedCount(kp); got != 0 {
		t.Errorf("protect-first shielding all messages must suppress compaction, got %d", got)
	}
}

// TestRun_ContextSummarizeEmbedsAbstractiveSummary: with ContextSummarize on, an
// elided tool output's stub carries a one-line summary produced by a provider
// call (routed through the same mock). The summary then flows back into a later
// request's context — which the capturing mock observes — proving M398 wires end
// to end (loop → makeElidedSummarizer → provider → stub → next request).
func TestRun_ContextSummarizeEmbedsAbstractiveSummary(t *testing.T) {
	const canned = "CANNED-SUMMARY-XYZ"
	sawSummaryInContext := false
	toolTurns := 0

	prov := &mock.Provider{Responder: func(req agent.CompletionRequest) agent.CompletionResponse {
		// A summarisation call is a single user message with the M398 prompt.
		if len(req.Messages) == 1 && strings.HasPrefix(req.Messages[0].Content, "Summarize this tool output") {
			return mock.FinalText(canned)
		}
		// Otherwise it's an agent-loop turn: did a prior elision's stub (carrying
		// the canned summary) make it back into the context we were handed?
		for _, m := range req.Messages {
			if strings.Contains(m.Content, canned) {
				sawSummaryInContext = true
			}
		}
		if toolTurns < 3 {
			toolTurns++
			return testToolUse("c", "dump", map[string]any{})
		}
		return mock.FinalText("done")
	}}

	k, err := runtime.Open(runtime.Config{
		BaseDir: t.TempDir(), Provider: prov, Model: "m",
		ContextBudget: 200, ContextSummarize: true, Edict: allowDump(),
		Tools: map[string]agent.Tool{"dump": dumpTool{out: strings.Repeat("Z", 2000)}},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, _, err := k.Run(context.Background(), "dump"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !sawSummaryInContext {
		t.Error("the abstractive summary should have replaced an elided stub and reached a later request's context")
	}
}

// TestRun_AutoBudgetOffForUnknownModel: auto mode with a model absent from the
// catalog leaves compaction off (no guessing a window) — no context.compacted.
func TestRun_AutoBudgetOffForUnknownModel(t *testing.T) {
	prov := mock.New(
		testToolUse("c1", "dump", map[string]any{}),
		testToolUse("c2", "dump", map[string]any{}),
		mock.FinalText("done"),
	)
	k, err := runtime.Open(runtime.Config{
		BaseDir:           t.TempDir(),
		Provider:          prov,
		Model:             "stranger", // not in any catalog
		Catalog:           catalog.NewEmpty(),
		ContextBudgetAuto: true,
		Edict:             allowDump(),
		Tools:             map[string]agent.Tool{"dump": dumpTool{out: strings.Repeat("Z", 2000)}},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, _, err := k.Run(context.Background(), "dump"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	n := 0
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindContextCompacted {
			n++
		}
		return nil
	})
	if n != 0 {
		t.Errorf("unknown model in auto mode must not compact, got %d", n)
	}
}

// TestRun_ContextSummarizeReasoningHeadroom: the elided-summary call's token
// cap follows the model's catalog reasoning flag (M926). A reasoning model
// spends output tokens on its chain of thought before the summary line — at
// the tight 64-token cap it returns empty content (observed live on
// deepseek-v4-pro), silently degrading every abstractive summary to the
// extractive head stub. Plain models keep the tight cap (spend stays
// negligible); reasoning models get headroom.
func TestRun_ContextSummarizeReasoningHeadroom(t *testing.T) {
	for _, tc := range []struct {
		name      string
		reasoning bool
		wantMax   int
	}{
		{"reasoning model gets headroom", true, 1024},
		{"plain model keeps the tight cap", false, 64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cat := catalog.NewEmpty()
			cat.Providers["p"] = &catalog.Provider{
				ID:     "p",
				Models: map[string]*catalog.Model{"mockmodel": {ID: "mockmodel", Reasoning: tc.reasoning}},
			}

			gotMax := 0
			toolTurns := 0
			prov := &mock.Provider{Responder: func(req agent.CompletionRequest) agent.CompletionResponse {
				if len(req.Messages) == 1 && strings.HasPrefix(req.Messages[0].Content, "Summarize this tool output") {
					gotMax = req.MaxTokens
					return mock.FinalText("one-line summary")
				}
				if toolTurns < 3 {
					toolTurns++
					return testToolUse("c", "dump", map[string]any{})
				}
				return mock.FinalText("done")
			}}

			k, err := runtime.Open(runtime.Config{
				BaseDir: t.TempDir(), Provider: prov, Model: "mockmodel", Catalog: cat,
				ContextBudget: 200, ContextSummarize: true, Edict: allowDump(),
				Tools: map[string]agent.Tool{"dump": dumpTool{out: strings.Repeat("Z", 2000)}},
			})
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			t.Cleanup(func() { k.Close() })

			if _, _, err := k.Run(context.Background(), "dump repeatedly"); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if gotMax != tc.wantMax {
				t.Errorf("summary call MaxTokens = %d, want %d", gotMax, tc.wantMax)
			}
		})
	}
}
