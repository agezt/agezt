// SPDX-License-Identifier: MIT

package runs

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/warden"
)

func TestWardenProfileResolvesAWardenFamilyProfile(t *testing.T) {
	k := runKernel(t)
	ctx, label, err := WardenProfile(context.Background(), k, "warden")
	if err != nil {
		t.Fatalf("warden profile: %v", err)
	}
	// The label is the EFFECTIVE profile, not the id that named it.
	if label != "namespace" {
		t.Errorf("label = %q, want namespace", label)
	}
	if ctx == nil {
		t.Fatalf("context = nil, want the profile override layered onto it")
	}
	override, ok := warden.ProfileOverrideFrom(ctx)
	if !ok || override != warden.ProfileNamespace {
		t.Errorf("override = %q/%v, want the namespace profile layered onto the context", override, ok)
	}
}

// The id is matched case-insensitively, like every other profile id.
func TestWardenProfileResolvesAnUppercaseProfileID(t *testing.T) {
	k := runKernel(t)
	_, label, err := WardenProfile(context.Background(), k, "WARDEN")
	if err != nil {
		t.Fatalf("warden profile: %v", err)
	}
	if label != "namespace" {
		t.Errorf("label = %q, want namespace", label)
	}
}

// A profile the run tools do not route is refused, and the message says
// what is supported.
func TestWardenProfileRefusesAnUnroutableProfile(t *testing.T) {
	k := runKernel(t)
	_, _, err := WardenProfile(context.Background(), k, "not-a-profile")
	if err == nil {
		t.Fatalf("err = nil, want a refusal")
	}
	if !strings.Contains(err.Error(), "not routable") {
		t.Errorf("err = %v, want the routability refusal", err)
	}
	if !strings.Contains(err.Error(), "not-a-profile") {
		t.Errorf("err = %v, want the profile named", err)
	}
	if !strings.Contains(err.Error(), "local") || !strings.Contains(err.Error(), "warden") {
		t.Errorf("err = %v, want the supported profiles named", err)
	}
}

// The container profile needs an active container backend; without one it is
// refused, and the message says how to get one.
func TestWardenProfileRefusesTheContainerProfileWithoutItsBackend(t *testing.T) {
	k := runKernel(t)
	_, _, err := WardenProfile(context.Background(), k, "docker")
	if err == nil {
		t.Fatalf("err = nil, want the container backend refusal")
	}
	if !strings.Contains(err.Error(), "container backend") {
		t.Errorf("err = %v, want the container backend refusal", err)
	}
	if !strings.Contains(err.Error(), "AGEZT_WARDEN_DOCKER") {
		t.Errorf("err = %v, want the message to say how to get the backend", err)
	}
}
