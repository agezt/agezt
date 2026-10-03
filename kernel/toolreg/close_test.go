// SPDX-License-Identifier: MIT

package toolreg

import (
	"errors"
	"slices"
	"testing"
)

// TestSetCloseReleasesWhatBuildStarted: boot-spawned plugin children used to
// outlive the daemon — nothing held them to close. Set.Close calls every
// Built.Close once, in reverse build order; a spec gated off after starting
// something is closed immediately; and a set that fails to build closes what
// its earlier specs started instead of leaking it.
func TestSetCloseReleasesWhatBuildStarted(t *testing.T) {
	var closed []string
	closer := func(name string) func() error {
		return func() error { closed = append(closed, name); return nil }
	}

	resetRegistryForTest()
	Register(Spec{Name: "a", Build: func(BuildDeps) (Built, error) {
		return Built{Tool: &fakeTool{name: "a"}, Close: closer("a")}, nil
	}})
	Register(Spec{Name: "gated", Build: func(BuildDeps) (Built, error) {
		return Built{Close: closer("gated")}, nil // started something, then registered nothing
	}})
	Register(Spec{Name: "b", Build: func(BuildDeps) (Built, error) {
		return Built{Tool: &fakeTool{name: "b"}, Close: closer("b")}, nil
	}})
	set, err := BuildAll(BuildDeps{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(closed, []string{"gated"}) {
		t.Fatalf("after build closed = %v, want [gated]", closed)
	}
	if err := set.Close(); err != nil {
		t.Fatal(err)
	}
	if err := set.Close(); err != nil { // idempotent
		t.Fatal(err)
	}
	if !slices.Equal(closed, []string{"gated", "b", "a"}) {
		t.Fatalf("closed = %v, want [gated b a] (reverse order, once each)", closed)
	}

	closed = nil
	resetRegistryForTest()
	Register(Spec{Name: "plugins", Build: func(BuildDeps) (Built, error) {
		return Built{Tool: &fakeTool{name: "p"}, Close: closer("plugins")}, nil
	}})
	Register(Spec{Name: "bad", Build: func(BuildDeps) (Built, error) { return Built{}, errors.New("boom") }})
	if _, err := BuildAll(BuildDeps{}); err == nil {
		t.Fatal("expected build error")
	}
	if !slices.Equal(closed, []string{"plugins"}) {
		t.Fatalf("failed build leaked started resources: closed = %v", closed)
	}
}
