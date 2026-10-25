// SPDX-License-Identifier: MIT

package runtime_test

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/worldmodelapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestTheWorldFaceIsTheContractsPort: the kernel's own face is the contract's
// port.
//
// The width is checked STATICALLY, on the accessor itself: a narrowed face
// would still answer the read side, so only the declared return type can tell
// the two apart.
func TestTheWorldFaceIsTheContractsPort(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if got, want := reflect.TypeOf(k.World).Out(0), reflect.TypeOf((*worldmodelapi.Graph)(nil)).Elem(); got != want {
		t.Fatalf("the kernel's world face is %v, want the contract's graph port %v", got, want)
	}
}

// TestTheWorldPortStaysNilWithoutALibrary: a kernel opened without a world
// model hands over a NIL port — not a port wrapping nothing.
//
// The check is on the CONCRETE value: a nil *Graph handed to an interface
// parameter is already a non-nil interface, and the first write would then
// call into a graph that is not there.
func TestTheWorldPortStaysNilWithoutALibrary(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if got := k.World(); got != nil {
		t.Fatalf("a kernel with no world model must yield a nil graph port, got %v", got)
	}
}

// TestTheWorldToolIsRegisteredOnlyWithAGraph: the world tool is the graph's
// agent-facing face — a kernel with no graph registers no world tool.
//
// A tool that calls into nothing would answer the agent with a panic, and the
// tool set is part of what the kernel's surfaces report.
func TestTheWorldToolIsRegisteredOnlyWithAGraph(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:   t.TempDir(),
		Provider:  mock.New(),
		WorldTool: true,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if _, ok := k.Tools()["world"]; ok {
		t.Fatal("a kernel with no world model must register no world tool")
	}

	withGraph := withWorld(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), WorldTool: true})
	k2, err := runtime.Open(withGraph)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k2.Close() })
	if _, ok := k2.Tools()["world"]; !ok {
		t.Fatal("a kernel with a world model must register the world tool")
	}
}

// TestTheWorldPortAnswersTheLiveLibrary: the port is not a shape — it answers
// the LIVE rows. An entity written through the port has to be in the graph the
// host opened.
func TestTheWorldPortAnswersTheLiveLibrary(t *testing.T) {
	k, err := runtime.Open(withWorld(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if k.World() == nil {
		t.Fatal("the kernel holds no world port")
	}
	e, _, err := k.World().Upsert("corr", worldmodelapi.UpsertSpec{Kind: worldmodelapi.KindProject, Name: "Lictor"})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, found, err := k.World().Get(e.ID)
	if err != nil || !found {
		t.Fatalf("the port must answer the live rows, got %+v (%v, %v)", got, found, err)
	}
	if got.Name != "Lictor" {
		t.Fatalf("the port answered %q, want Lictor", got.Name)
	}
	if all, err := k.World().Entities(); err != nil || len(all) != 1 {
		t.Fatalf("the port must answer the live entities, got %d (%v)", len(all), err)
	}
}
