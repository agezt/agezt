// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/worldmodel"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/shell"
)

func TestWorldEntitiesInjectedIntoSystemPrompt(t *testing.T) {
	prov := mock.New(mock.FinalText("answered"))
	var gotSystem string
	prov.OnRequest = func(req agent.CompletionRequest) { gotSystem = req.System }

	k, err := runtime.Open(runtime.Config{
		BaseDir:     t.TempDir(),
		Provider:    prov,
		System:      "base prompt",
		WorldInject: true,
		WorldTopK:   5,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	if _, _, err := k.World().Upsert("seed", worldmodel.UpsertSpec{
		Kind: worldmodel.KindProject, Name: "Lictor", Aliases: []string{"the portfolio"},
	}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := k.Run(context.Background(), "check the portfolio please"); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(gotSystem, "Lictor") {
		t.Fatalf("resolved entity was not injected into system prompt; got:\n%s", gotSystem)
	}
	if !strings.Contains(gotSystem, "base prompt") {
		t.Fatal("original system prompt must be preserved alongside injected entities")
	}
	if countKind(t, k, event.KindWorldRetrieved) != 1 {
		t.Fatal("injection must journal a worldmodel.retrieved event for `agt why`")
	}
}

func TestWorldInjectionOffByDefault(t *testing.T) {
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

	_, _, _ = k.World().Upsert("seed", worldmodel.UpsertSpec{Kind: worldmodel.KindProject, Name: "Lictor"})
	if _, _, err := k.Run(context.Background(), "Lictor"); err != nil {
		t.Fatal(err)
	}
	if gotSystem != "base" {
		t.Fatalf("with WorldInject off the system prompt must be untouched, got %q", gotSystem)
	}
}

func TestWorldToolRegisteredWhenEnabled(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:   t.TempDir(),
		Provider:  mock.New(mock.FinalText("ok")),
		Tools:     map[string]agent.Tool{"shell": shell.New()},
		WorldTool: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	if _, ok := k.Tools()["world"]; !ok {
		t.Fatal("world tool should be registered when WorldTool=true")
	}
	if _, ok := k.Tools()["shell"]; !ok {
		t.Fatal("configured tools must still be present")
	}
}

func TestWorldToolAbsentByDefault(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("ok")),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	if _, ok := k.Tools()["world"]; ok {
		t.Fatal("world tool must not be registered unless WorldTool=true")
	}
}
