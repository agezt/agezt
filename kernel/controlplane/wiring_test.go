// SPDX-License-Identifier: MIT

package controlplane

// The adapter's wiring is ONE door.
//
// The composition root builds the scope and hands over what it has; the
// adapter has no setter to call later and no phase in which the root binds a
// second half of the wiring (the target architecture's P8: "wiring is
// declarative — modules declare Requires/Provides; the composition root
// resolves them").
//
// The rules are the reflection of the adapter's own API: they do not ask a
// helper what the adapter should be, they read what it IS.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/app/binding"
)

// TestTheAdaptersWiringIsOneDoor: the adapter exposes no back-wiring.
//
// A `SetX` on the adapter is a door the root can walk through at any time, and
// the adapter cannot know what state it is in. The late-binding phase is the
// same door with a schedule attached.
func TestTheAdaptersWiringIsOneDoor(t *testing.T) {
	server := reflect.TypeOf((*Server)(nil))
	for i := 0; i < server.NumMethod(); i++ {
		name := server.Method(i).Name
		if strings.HasPrefix(name, "Set") {
			t.Errorf("the adapter exports %s — the wiring is one door, and the composition root resolves it", name)
		}
		if name == "Bind" {
			t.Error("the adapter still has a Bind phase — the wiring is one door")
		}
	}
}

// TestTheLateArtifactsAreAskedForNotPushed: the artifacts that only exist
// AFTER the adapter is up are PROVIDERS.
//
// A provider takes nothing and answers the artifact (or nil, until the root
// publishes it). A value that the root PUSHES into the adapter later is the
// back-wiring this slice removed.
func TestTheLateArtifactsAreAskedForNotPushed(t *testing.T) {
	deps := reflect.TypeOf(Deps{})
	for _, name := range []string{"Pulse", "Observers", "ChannelSend", "StandingFire"} {
		field, ok := deps.FieldByName(name)
		if !ok {
			t.Fatalf("the deps have no %s", name)
		}
		if field.Type.Kind() != reflect.Func || field.Type.NumIn() != 0 || field.Type.NumOut() != 1 {
			t.Errorf("Deps.%s is %v, want a PROVIDER — a way to ask, not a value to push", name, field.Type)
		}
	}
}

// TestTheLateArtifactIsNilUntilTheRootPublishesIt: the provider's answer is
// nil until the root publishes the artifact.
//
// The daemon's pulse engine, live channels and standing runner come up AFTER
// the adapter. The adapter asks when it needs one, and the answer is nil
// until the artifact is there — the same window the late-binding phase had,
// without the phase.
func TestTheLateArtifactIsNilUntilTheRootPublishesIt(t *testing.T) {
	var published PulseController
	s := NewServerWithDeps(binding.ScopeFromKernel(nil, t.TempDir()), Deps{
		Pulse: func() PulseController { return published },
	})
	if s.pulseController() != nil {
		t.Error("the adapter saw a pulse engine that was not published yet")
	}
	if s.pulseObservers() != nil {
		t.Error("the adapter saw pulse observers that were not published yet")
	}
	published = askPulse{}
	if s.pulseController() == nil {
		t.Error("the adapter did not see the published pulse engine")
	}
}

// TestTheProviderIsAskedEveryTime: the adapter asks, it does not cache.
//
// The daemon publishes the late artifacts on its own goroutine; a cached
// answer would freeze the adapter at the moment it was built.
func TestTheProviderIsAskedEveryTime(t *testing.T) {
	asked := 0
	s := NewServerWithDeps(binding.ScopeFromKernel(nil, t.TempDir()), Deps{
		Pulse: func() PulseController {
			asked++
			return nil
		},
	})
	for i := 0; i < 3; i++ {
		s.pulseController()
	}
	if asked != 3 {
		t.Errorf("the provider was asked %d times, want 3 — the adapter asks, it does not cache", asked)
	}
}
