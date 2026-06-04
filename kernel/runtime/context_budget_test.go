// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

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
		mock.ToolUse("c1", "dump", map[string]any{}),
		mock.ToolUse("c2", "dump", map[string]any{}),
		mock.ToolUse("c3", "dump", map[string]any{}),
		mock.FinalText("done"),
	)
	k, err := runtime.Open(runtime.Config{
		BaseDir:           t.TempDir(),
		Provider:          prov,
		Model:             "mockmodel",
		Catalog:           cat,
		ContextBudgetAuto: true,
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

// TestRun_AutoBudgetOffForUnknownModel: auto mode with a model absent from the
// catalog leaves compaction off (no guessing a window) — no context.compacted.
func TestRun_AutoBudgetOffForUnknownModel(t *testing.T) {
	prov := mock.New(
		mock.ToolUse("c1", "dump", map[string]any{}),
		mock.ToolUse("c2", "dump", map[string]any{}),
		mock.FinalText("done"),
	)
	k, err := runtime.Open(runtime.Config{
		BaseDir:           t.TempDir(),
		Provider:          prov,
		Model:             "stranger", // not in any catalog
		Catalog:           catalog.NewEmpty(),
		ContextBudgetAuto: true,
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
