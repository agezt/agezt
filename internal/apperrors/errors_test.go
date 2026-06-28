// SPDX-License-Identifier: MIT

//lint:file-ignore SA1012 Wrap/Wrapf accept a context.Context as their first
// argument and must tolerate nil; these tests deliberately pass nil to verify
// that nil-context path rather than papering over it with context.TODO().

package apperrors

import (
	"errors"
	"testing"
)

func TestWrap(t *testing.T) {
	inner := errors.New("connection refused")

	got := Wrap(nil, "controlplane: listen", inner)
	if got == nil {
		t.Fatal("Wrap returned nil")
	}
	if got == inner {
		t.Error("Wrap should return a new error, not the same instance")
	}
	want := "controlplane: listen: connection refused"
	if got.Error() != want {
		t.Errorf("Wrap() = %q, want %q", got.Error(), want)
	}
}

func TestWrap_NilError(t *testing.T) {
	got := Wrap(nil, "controlplane: listen", nil)
	if got != nil {
		t.Errorf("Wrap(nil) = %v, want nil", got)
	}
}

func TestWrapf(t *testing.T) {
	inner := errors.New("timeout")

	got := Wrapf(nil, "agent: provider %s", inner, "anthropic")
	if got == nil {
		t.Fatal("Wrapf returned nil")
	}
	want := "agent: provider anthropic: timeout"
	if got.Error() != want {
		t.Errorf("Wrapf() = %q, want %q", got.Error(), want)
	}
}

func TestWrapf_NilError(t *testing.T) {
	got := Wrapf(nil, "agent: provider %s", nil, "anthropic")
	if got != nil {
		t.Errorf("Wrapf(nil) = %v, want nil", got)
	}
}

func TestWrapSimple(t *testing.T) {
	inner := errors.New("connection refused")

	got := WrapSimple("controlplane: listen", inner)
	if got == nil {
		t.Fatal("WrapSimple returned nil")
	}
	if got == inner {
		t.Error("WrapSimple should return a new error, not the same instance")
	}
	want := "controlplane: listen: connection refused"
	if got.Error() != want {
		t.Errorf("WrapSimple() = %q, want %q", got.Error(), want)
	}
}

func TestWrapSimple_NilError(t *testing.T) {
	got := WrapSimple("controlplane: listen", nil)
	if got != nil {
		t.Errorf("WrapSimple(nil) = %v, want nil", got)
	}
}

func TestWrapSimplef(t *testing.T) {
	inner := errors.New("timeout")

	got := WrapSimplef("agent: provider %s", inner, "anthropic")
	if got == nil {
		t.Fatal("WrapSimplef returned nil")
	}
	want := "agent: provider anthropic: timeout"
	if got.Error() != want {
		t.Errorf("WrapSimplef() = %q, want %q", got.Error(), want)
	}
}

func TestWrapSimplef_NilError(t *testing.T) {
	got := WrapSimplef("agent: provider %s", nil, "anthropic")
	if got != nil {
		t.Errorf("WrapSimplef(nil) = %v, want nil", got)
	}
}
