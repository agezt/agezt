// SPDX-License-Identifier: MIT

package builtinchannels

import (
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

func TestRegisterAll_SeedsRegistry(t *testing.T) {
	// Start clean by verifying the registry is initially empty for this test.
	initial := channel.Manifests()
	initialCount := len(initial)

	RegisterAll()

	got := channel.Manifests()
	gotCount := len(got)

	// Should have registered the manifests (minus whatever was in the registry before).
	if gotCount <= initialCount {
		t.Fatalf("Manifests count after RegisterAll = %d, initial = %d, expected more", gotCount, initialCount)
	}

	// Build a lookup by kind from what we registered.
	registered := map[string]bool{}
	for _, m := range manifests {
		registered[m.Kind] = true
	}

	// Every registered manifest should appear in Manifests().
	found := 0
	for _, m := range got {
		if registered[m.Kind] {
			found++
			if m.Kind == "" {
				t.Error("manifest has empty Kind")
			}
			if m.Display == "" {
				t.Errorf("manifest %q has empty Display", m.Kind)
			}
			if m.Description == "" {
				t.Errorf("manifest %q has empty Description", m.Kind)
			}
		}
	}
	if found != len(manifests) {
		t.Errorf("found %d of %d registered manifests in Manifests()", found, len(manifests))
	}
}

func TestRegisterAll_Idempotent(t *testing.T) {
	// Calling RegisterAll twice should not error (idempotent).
	RegisterAll()
	RegisterAll()
	// If we reach here without panic, it's fine.
}

func TestManifests_HaveTransport(t *testing.T) {
	for _, m := range manifests {
		if m.Transport == "" {
			t.Errorf("manifest %q (%s) has empty Transport", m.Kind, m.Display)
		}
	}
}

func TestManifests_HaveRequiredEnv(t *testing.T) {
	for _, m := range manifests {
		if len(m.RequiredEnv) == 0 {
			t.Errorf("manifest %q (%s) has no RequiredEnv", m.Kind, m.Display)
		}
	}
}
