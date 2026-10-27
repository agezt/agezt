// SPDX-License-Identifier: MIT

package cadenceapi

import (
	"context"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/catalogapi"
	"github.com/agezt/agezt/kernel/contract/eventapi"
	"github.com/agezt/agezt/kernel/contract/journalapi"
	"github.com/agezt/agezt/kernel/contract/memoryapi"
)

// TestTheSystemTaskPortsNameOnlyContractRows: every type the port set names
// must be a contract's own row or a contract's own port.
//
// A module's type in a port's signature would drag that module into every
// layer that holds the port — which is exactly the edge this port set exists
// to close: the cadence module must never name the runs module again.
func TestTheSystemTaskPortsNameOnlyContractRows(t *testing.T) {
	typ := reflect.TypeOf(SystemTaskPorts{})

	want := map[string]reflect.Type{
		"Publish":          reflect.TypeOf((*eventapi.Publisher)(nil)).Elem(),
		"Memory":           reflect.TypeOf((*memoryapi.Maintainer)(nil)).Elem(),
		"DistillProfile":   reflect.TypeOf(func(context.Context, string) (memoryapi.ProfileReport, error) { return memoryapi.ProfileReport{}, nil }),
		"RosterList":       reflect.TypeOf(func() []agentapi.Profile { return nil }),
		"CollectArtifacts": reflect.TypeOf(func(int64) (int, int64) { return 0, 0 }),
		"ScanJournal":      reflect.TypeOf((*journalapi.Scanner)(nil)).Elem(),
		"CatalogSyncer":    reflect.TypeOf((*catalogapi.Syncer)(nil)).Elem(),
		"CatalogStore":     reflect.TypeOf((*catalogapi.Store)(nil)).Elem(),
		"Reload":           reflect.TypeOf(func() (*catalogapi.Catalog, bool, error) { return nil, false, nil }),
	}
	if typ.NumField() != len(want) {
		t.Fatalf("the port set has %d fields, want %d: %v", typ.NumField(), len(want), typ)
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		expected, ok := want[field.Name]
		if !ok {
			t.Errorf("the port set has an unexpected field %q", field.Name)
			continue
		}
		if field.Type != expected {
			t.Errorf("field %q is %v, want %v", field.Name, field.Type, expected)
		}
	}
}

// TestThePortsAreNilWhenTheDaemonHasNoCapability: a missing capability is a
// NIL field, and a nil field is a refusal.
//
// The ports are held in a struct, so "no memory" is a nil interface field
// and not a memory that answers zeroes — "this daemon has no memory" and
// "the memory is empty" are different facts.
func TestThePortsAreNilWhenTheDaemonHasNoCapability(t *testing.T) {
	var ports SystemTaskPorts
	if ports.Publish != nil || ports.Memory != nil || ports.ScanJournal != nil ||
		ports.CatalogSyncer != nil || ports.CatalogStore != nil {
		t.Fatal("an empty port set must hold no capability")
	}
	if ports.CollectArtifacts != nil || ports.DistillProfile != nil ||
		ports.RosterList != nil || ports.Reload != nil {
		t.Fatal("an empty port set must hold no capability")
	}
}

// TestTheArtifactPortIsOneVerbWide: the artifact index is a TEN-verb host
// port, and two of its verbs delete bytes while a third switches off the
// journal's ref guard.
//
// The collect task needs exactly one of them. A port that handed the task
// the whole index would let a maintenance job clear the guard that protects
// the bytes a run's output still points at.
func TestTheArtifactPortIsOneVerbWide(t *testing.T) {
	typ := reflect.TypeOf(SystemTaskPorts{}).Field(4)
	if typ.Type.Kind() != reflect.Func {
		t.Fatalf("the artifact port must be a function, got %v", typ.Type)
	}
	if typ.Type.NumIn() != 1 {
		t.Errorf("the artifact port must take 1 argument, got %v", typ.Type)
	}
	if typ.Type.In(0) != reflect.TypeOf(int64(0)) {
		t.Errorf("the artifact port must take the cutoff, got %v", typ.Type.In(0))
	}
	if typ.Type.NumOut() != 2 {
		t.Errorf("the artifact port must answer 2 values, got %v", typ.Type)
	}
	if typ.Type.Out(0) != reflect.TypeOf(int(0)) {
		t.Errorf("the artifact port must answer the collected count, got %v", typ.Type.Out(0))
	}
	if typ.Type.Out(1) != reflect.TypeOf(int64(0)) {
		t.Errorf("the artifact port must answer the freed bytes, got %v", typ.Type.Out(1))
	}
}

// TestTheMemoryPortIsTheMaintainerHalf: the memory port is the contract's
// MAINTAINER — the hygiene verbs — and not the whole manager.
//
// The whole manager carries the reader, the writer, the distiller and the
// tool: a task that cleans the store has no business recalling from it,
// writing to it or handing out its tool.
func TestTheMemoryPortIsTheMaintainerHalf(t *testing.T) {
	maint := reflect.TypeOf((*memoryapi.Maintainer)(nil)).Elem()
	manager := reflect.TypeOf((*memoryapi.Manager)(nil)).Elem()
	if maint.NumMethod() >= manager.NumMethod() {
		t.Fatalf("the maintainer port has %d verbs and the manager %d — the half is not narrower",
			maint.NumMethod(), manager.NumMethod())
	}
	for _, name := range []string{"CleanLowValue", "DedupeDistilled"} {
		if _, ok := maint.MethodByName(name); !ok {
			t.Errorf("the maintainer port must have %s", name)
		}
	}
	for _, name := range []string{"Remember", "Tool"} {
		if _, ok := maint.MethodByName(name); ok {
			t.Errorf("the maintainer port must not have %s", name)
		}
	}
}

// TestTheDistillPortTakesTheCallersContext: the distill verb is the one task
// that asks a model a question, so the caller's context and correlation
// travel with it.
func TestTheDistillPortTakesTheCallersContext(t *testing.T) {
	typ := reflect.TypeOf(SystemTaskPorts{}).Field(2).Type
	if typ.Kind() != reflect.Func {
		t.Fatalf("the distill port must be a function, got %v", typ)
	}
	if typ.NumIn() != 2 {
		t.Fatalf("the distill port must take 2 arguments, got %v", typ)
	}
	if typ.In(0) != reflect.TypeOf((*context.Context)(nil)).Elem() {
		t.Errorf("the distill port must take the context, got %v", typ.In(0))
	}
	if typ.In(1) != reflect.TypeOf("") {
		t.Errorf("the distill port must take the correlation, got %v", typ.In(1))
	}
	if typ.NumOut() != 2 {
		t.Fatalf("the distill port must answer 2 values, got %v", typ)
	}
	if typ.Out(0) != reflect.TypeOf(memoryapi.ProfileReport{}) {
		t.Errorf("the distill port must answer the contract's report, got %v", typ.Out(0))
	}
	if typ.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
		t.Errorf("the distill port must answer an error, got %v", typ.Out(1))
	}
}
