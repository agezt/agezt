// SPDX-License-Identifier: MIT

package execprofile

import (
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The port's THIRD capability is the warden's own answer: the profile the
// daemon really runs for a requested one.
//
// The warden is the execution profile family's business — the guard says so
// by name — and the run path asks this family for it instead of reaching for
// the kernel's own verb. The test pins the FILLED half: the port really
// answers with the daemon's warden.
func TestTheExecutionProfilePortsAnswerTheWarden(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(),
		Tools:    map[string]toolapi.Tool{"shell": probeTool{}},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ports := PortsFromKernel(k)
	if ports.EffectiveProfile == nil {
		t.Fatal("the port answers nothing for the warden — the run path would panic")
	}

	for _, want := range []warden.Profile{warden.ProfileNone, warden.ProfileNamespace, warden.ProfileContainer} {
		got := ports.EffectiveProfile(want)
		if daemon := k.Warden().EffectiveProfile(want); got != daemon {
			t.Errorf("the port's effective profile for %q = %q, want the daemon's %q", want, got, daemon)
		}
	}
}

// A host with no daemon holds no warden, and the answer is the CONSERVATIVE
// one: no isolation.
//
// A run that asked for the container profile is then REFUSED rather than
// silently unisolated — the port's answer is the refusal's own ground.
func TestTheExecutionProfilePortsAnswerAnEmptyHostWithoutAWarden(t *testing.T) {
	ports := PortsFromKernel(nil)

	if ports.EffectiveProfile == nil {
		t.Fatal("the port answers nothing for the warden — the run path would panic")
	}
	if got := ports.EffectiveProfile(warden.ProfileContainer); got != warden.ProfileNone {
		t.Errorf("a host with no daemon answers the container profile as %q, want the conservative %q", got, warden.ProfileNone)
	}
	if got := ports.EffectiveProfile(warden.ProfileNamespace); got != warden.ProfileNone {
		t.Errorf("a host with no daemon answers the namespace profile as %q, want the conservative %q", got, warden.ProfileNone)
	}
}
