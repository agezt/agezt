// SPDX-License-Identifier: MIT

package intervention

import (
	"strings"
	"testing"
	"time"
)

func TestNormalize_DefaultsAndTrims(t *testing.T) {
	got, err := Normalize(Request{
		Primitive:     Primitive(" HALT "),
		CorrelationID: " corr-1 ",
		Scope:         " ",
	})
	if err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}
	if got.Primitive != PrimitiveHalt {
		t.Fatalf("Primitive = %q, want %q", got.Primitive, PrimitiveHalt)
	}
	if got.CorrelationID != "corr-1" {
		t.Fatalf("CorrelationID = %q, want corr-1", got.CorrelationID)
	}
	if got.Scope != "run" {
		t.Fatalf("Scope = %q, want run", got.Scope)
	}
	if got.Lease != DefaultLease {
		t.Fatalf("Lease = %s, want %s", got.Lease, DefaultLease)
	}
}

func TestNormalize_RequiresCorrelation(t *testing.T) {
	_, err := Normalize(Request{Primitive: PrimitiveQuery})
	if err == nil || !strings.Contains(err.Error(), "correlation required") {
		t.Fatalf("Normalize error = %v, want correlation required", err)
	}
}

func TestNormalize_RequiresDirectiveForMutatingDirectivePrimitives(t *testing.T) {
	for _, primitive := range []Primitive{PrimitiveRedirect, PrimitiveAdjust} {
		_, err := Normalize(Request{Primitive: primitive, CorrelationID: "corr"})
		if err == nil || !strings.Contains(err.Error(), "directive required") {
			t.Fatalf("%s error = %v, want directive required", primitive, err)
		}
	}
}

func TestNormalize_AcceptsExplicitLeaseScopeAndDirective(t *testing.T) {
	got, err := Normalize(Request{
		Primitive:     PrimitiveRedirect,
		CorrelationID: "corr",
		Directive:     "  use smaller model  ",
		Lease:         30 * time.Second,
		Scope:         "agent",
	})
	if err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}
	if got.Directive != "use smaller model" {
		t.Fatalf("Directive = %q", got.Directive)
	}
	if got.Lease != 30*time.Second {
		t.Fatalf("Lease = %s", got.Lease)
	}
	if got.Scope != "agent" {
		t.Fatalf("Scope = %q", got.Scope)
	}
}

func TestNormalize_RejectsUnknownPrimitive(t *testing.T) {
	_, err := Normalize(Request{Primitive: Primitive("pause"), CorrelationID: "corr"})
	if err == nil || !strings.Contains(err.Error(), "primitive must be") {
		t.Fatalf("Normalize error = %v, want primitive validation", err)
	}
}
