// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/shell"
)

func countKind(t *testing.T, k *runtime.Kernel, kind event.Kind) int {
	t.Helper()
	n := 0
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind == kind {
			n++
		}
		return nil
	})
	return n
}

func TestMemoryInjectedIntoSystemPrompt(t *testing.T) {
	prov := mock.New(mock.FinalText("answered"))
	var gotSystem string
	prov.OnRequest = func(req agent.CompletionRequest) { gotSystem = req.System }

	k, err := runtime.Open(runtime.Config{
		BaseDir:      t.TempDir(),
		Provider:     prov,
		System:       "base prompt",
		MemoryInject: true,
		MemoryTopK:   5,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	if _, _, err := k.Memory().Remember("seed", memory.RememberSpec{
		Type: memory.TypeFact, Subject: "lictor", Content: "Agezt is a Go agentic OS",
	}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := k.Run(context.Background(), "tell me about lictor agezt"); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(gotSystem, "Agezt is a Go agentic OS") {
		t.Fatalf("recalled fact was not injected into system prompt; got:\n%s", gotSystem)
	}
	if !strings.Contains(gotSystem, "base prompt") {
		t.Fatal("original system prompt must be preserved alongside injected memory")
	}
	if countKind(t, k, event.KindMemoryRetrieved) != 1 {
		t.Fatal("injection must journal a memory.retrieved event for `agt why`")
	}
}

func TestMemoryInjectionOffByDefault(t *testing.T) {
	prov := mock.New(mock.FinalText("ok"))
	var gotSystem string
	prov.OnRequest = func(req agent.CompletionRequest) { gotSystem = req.System }

	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
		System:   "base",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	_, _, _ = k.Memory().Remember("seed", memory.RememberSpec{Subject: "x", Content: "secret fact about agezt"})
	if _, _, err := k.Run(context.Background(), "agezt"); err != nil {
		t.Fatal(err)
	}
	if gotSystem != "base" {
		t.Fatalf("with MemoryInject off the system prompt must be untouched, got %q", gotSystem)
	}
}

func TestMemoryToolRegisteredWhenEnabled(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:    t.TempDir(),
		Provider:   mock.New(mock.FinalText("ok")),
		Tools:      map[string]agent.Tool{"shell": shell.NewWithWarden(warden.New(nil))},
		MemoryTool: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	if _, ok := k.Tools()["memory"]; !ok {
		t.Fatal("memory tool should be registered when MemoryTool=true")
	}
	if _, ok := k.Tools()["shell"]; !ok {
		t.Fatal("configured tools must still be present")
	}
}

func TestMemoryToolAbsentByDefault(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("ok")),
		Tools:    map[string]agent.Tool{"shell": shell.NewWithWarden(warden.New(nil))},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	if _, ok := k.Tools()["memory"]; ok {
		t.Fatal("memory tool must not be registered unless MemoryTool=true")
	}
}

func TestAutoDistillAfterMultiToolRun(t *testing.T) {
	// Scripted: one shell tool call, a final answer, then the distill
	// LLM call returns a JSON facts payload (consumed from the same mock).
	prov := mock.New(
		testToolUse("c1", "shell", map[string]string{"command": "echo hi"}),
		mock.FinalText("the project is a go monorepo"),
		mock.FinalText(`{"facts":[{"subject":"lictor","content":"lictor is the agezt monorepo","type":"FACT"}]}`),
	)
	k, err := runtime.Open(runtime.Config{
		BaseDir:               t.TempDir(),
		Provider:              prov,
		Tools:                 map[string]agent.Tool{"shell": shell.NewWithWarden(warden.New(nil))},
		MemoryDistill:         true,
		MemoryDistillMinTools: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	if _, _, err := k.Run(context.Background(), "what is this project?"); err != nil {
		t.Fatal(err)
	}

	all, _ := k.Memory().All()
	if len(all) != 1 || all[0].Tags["source"] != "distill" {
		t.Fatalf("expected one distilled record tagged source=distill, got %+v", all)
	}
}

func TestNoDistillBelowThreshold(t *testing.T) {
	prov := mock.New(mock.FinalText("quick answer"))
	k, err := runtime.Open(runtime.Config{
		BaseDir:               t.TempDir(),
		Provider:              prov,
		MemoryDistill:         true,
		MemoryDistillMinTools: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	if _, _, err := k.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if n := k.Memory().Count(); n != 0 {
		t.Fatalf("no-tool run must not distill, got %d records", n)
	}
}
