// SPDX-License-Identifier: MIT

package wardenapi

import (
	"context"
	"reflect"
	"testing"
)

// TestTheWardenPortCarriesExactlyTwoVerbs: the port a plugin drives the
// sandbox through is Run + EffectiveProfile — nothing else.
//
// The platform's engine carries a third verb (SetBus), and that verb names
// the BUS: a row the contract layer may not import (archcheck's principle
// P4). If the verb ever crept into this port, the contract would have to name
// the bus too — and every holder of the port would drag it along.
func TestTheWardenPortCarriesExactlyTwoVerbs(t *testing.T) {
	typ := reflect.TypeOf((*Engine)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Engine must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 2 {
		t.Fatalf("the port has %d methods, want 2: %v", typ.NumMethod(), typ)
	}
	for _, name := range []string{"Run", "EffectiveProfile"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the port must have a %s method", name)
		}
	}

	run, ok := typ.MethodByName("Run")
	if !ok {
		t.Fatal("the port must have a Run method")
	}
	// An interface method's type carries no receiver: In(0) is the first
	// ARGUMENT.
	if run.Type.NumIn() != 2 {
		t.Errorf("Run must take 2 arguments, got %v", run.Type)
	}
	if run.Type.In(0) != reflect.TypeOf((*context.Context)(nil)).Elem() {
		t.Errorf("Run must take a context first, got %v", run.Type.In(0))
	}
	if run.Type.In(1) != reflect.TypeOf(Spec{}) {
		t.Errorf("Run must take the contract's spec, got %v", run.Type.In(1))
	}
	if run.Type.NumOut() != 2 {
		t.Errorf("Run must answer a result and an error, got %v", run.Type)
	}
	if run.Type.Out(0) != reflect.TypeOf((*Result)(nil)) {
		t.Errorf("Run must answer the contract's result, got %v", run.Type.Out(0))
	}

	eff, ok := typ.MethodByName("EffectiveProfile")
	if !ok {
		t.Fatal("the port must have an EffectiveProfile method")
	}
	if eff.Type.NumIn() != 1 || eff.Type.In(0) != reflect.TypeOf(Profile("")) {
		t.Errorf("EffectiveProfile must take the contract's profile, got %v", eff.Type)
	}
	if eff.Type.NumOut() != 1 || eff.Type.Out(0) != reflect.TypeOf(Profile("")) {
		t.Errorf("EffectiveProfile must answer the contract's profile, got %v", eff.Type)
	}
}

// TestTheProfileVocabularyIsClosed: the four canonical isolation modes and
// nothing else. A profile that passed as known would be asked for a
// guarantee the host cannot give.
func TestTheProfileVocabularyIsClosed(t *testing.T) {
	for _, p := range []Profile{ProfileNone, ProfileNamespace, ProfileContainer, ProfileMicroVM} {
		if !p.IsKnown() {
			t.Errorf("%q must be a known profile", p)
		}
	}
	for _, p := range []Profile{"", "container-ish", "None"} {
		if p.IsKnown() {
			t.Errorf("%q must not pass as a known profile", p)
		}
	}
}

// TestTheRowsKeepTheirFields: the three rows moved from the platform as they
// were. A dropped field would silently change what a run is asked for — a
// lost Limit is a cap that is not applied, a lost CorrelationID is an event
// nobody can trace back.
func TestTheRowsKeepTheirFields(t *testing.T) {
	for _, tc := range []struct {
		name string
		typ  reflect.Type
		want []string
	}{
		{"Limits", reflect.TypeOf(Limits{}), []string{
			"Timeout", "MaxOutputBytes", "WaitDelay",
			"CPUSeconds", "AddressSpaceBytes", "MaxOpenFiles", "MaxFileSizeBytes",
		}},
		{"Spec", reflect.TypeOf(Spec{}), []string{
			"Profile", "Argv", "WorkDir", "Env", "Limits", "Actor", "CorrelationID",
		}},
		{"Result", reflect.TypeOf(Result{}), []string{
			"EffectiveProfile", "RequestedProfile", "Downgraded", "ExitCode",
			"Stdout", "Stderr", "Truncated", "TimedOut", "Duration", "Started", "Ended",
		}},
	} {
		if tc.typ.NumField() != len(tc.want) {
			t.Errorf("%s has %d fields, want %d", tc.name, tc.typ.NumField(), len(tc.want))
			continue
		}
		for i, name := range tc.want {
			if got := tc.typ.Field(i).Name; got != name {
				t.Errorf("%s field %d = %q, want %q", tc.name, i, got, name)
			}
		}
	}
}

// TestTheSpecCarriesTheContractRows: a spec is made of the contract's own
// rows — a profile and a limits row from this package, not from a platform
// package the contract may not import.
func TestTheSpecCarriesTheContractRows(t *testing.T) {
	typ := reflect.TypeOf(Spec{})
	profile := typ.Field(0)
	if profile.Name != "Profile" || profile.Type != reflect.TypeOf(Profile("")) {
		t.Errorf("Spec.Profile must be the contract's profile, got %v", profile.Type)
	}
	limits := typ.Field(4)
	if limits.Name != "Limits" || limits.Type != reflect.TypeOf(Limits{}) {
		t.Errorf("Spec.Limits must be the contract's limits, got %v", limits.Type)
	}
}
