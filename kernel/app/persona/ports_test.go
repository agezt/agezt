// SPDX-License-Identifier: MIT
package persona_test

import (
	"reflect"
	"testing"

	apppersona "github.com/agezt/agezt/kernel/app/persona"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The persona family's ports are filled by the APPLICATION from the kernel
// handle — the host never binds them itself.
//
// The test pins what that wiring buys: the identity the port hands over is the
// daemon's own — its default model and its default identity prompt — and the
// prompt can be changed LIVE through the port.
func TestThePersonaIdentityReachesTheKernel(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()
	k.SetModel("the-model")
	k.SetSystem("the prompt")

	identity := apppersona.IdentityFromKernel(k)

	if got := identity.DefaultModel(); got != "the-model" {
		t.Fatalf("the identity reports the model %q, want the daemon's own", got)
	}
	if got := identity.Prompt(); got != "the prompt" {
		t.Fatalf("the identity reports the prompt %q, want the daemon's own", got)
	}
	if !identity.PromptSet() {
		t.Fatal("the identity says no prompt is set — the daemon has one")
	}

	// The prompt changes LIVE: the next default run picks it up.
	identity.SetPrompt("another prompt")
	if got := k.System(); got != "another prompt" {
		t.Fatalf("the kernel holds the prompt %q, want the one the port set", got)
	}
	if !identity.PromptSet() {
		t.Fatal("the identity says no prompt is set — the daemon still has one")
	}
}

// A host with no kernel holds no identity.
//
// The port answers for that case with the empty answers: no model, no prompt,
// and a change that changes nothing.
func TestThePersonaIdentitySurvivesAHostWithNoKernel(t *testing.T) {
	identity := apppersona.IdentityFromKernel(nil)

	if got := identity.DefaultModel(); got != "" {
		t.Fatalf("a host with no kernel reports the model %q", got)
	}
	if got := identity.Prompt(); got != "" {
		t.Fatalf("a host with no kernel reports the prompt %q", got)
	}
	if identity.PromptSet() {
		t.Fatal("a host with no kernel says a prompt is set")
	}
	identity.SetPrompt("nothing to set")
}

// The port's names may not be the kernel's own: a port that carried the name
// of the verb it stands for would let the adapter name the kernel's verb
// through the port's door.
//
// The rule is checked HERE, by reflection over the port's own method set and
// the kernel's, so it does not depend on what the guard happens to own today.
func TestThePersonaPortNamesAreNotTheKernelVerbs(t *testing.T) {
	port := reflect.TypeOf(apppersona.Identity{})
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the persona port's %s is the kernel's own verb — a port may not carry the name of the verb it stands for", name)
		}
	}
	if port.NumMethod() == 0 {
		t.Fatal("the persona port has no methods — the family cannot answer anything")
	}
}
