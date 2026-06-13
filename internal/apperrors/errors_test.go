// SPDX-License-Identifier: MIT

package apperrors

import (
	"errors"
	"fmt"
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

func TestJoin(t *testing.T) {
	err1 := errors.New("first error")
	err2 := errors.New("second error")
	err3 := errors.New("third error")

	tests := []struct {
		name    string
		errs    []error
		want    string
		wantNil bool
	}{
		{
			name:    "single error",
			errs:    []error{err1},
			want:    "first error",
			wantNil: false,
		},
		{
			name:    "two errors",
			errs:    []error{err1, err2},
			want:    "first error | second error",
			wantNil: false,
		},
		{
			name:    "three errors",
			errs:    []error{err1, err2, err3},
			want:    "first error | second error | third error",
			wantNil: false,
		},
		{
			name:    "all nil",
			errs:    []error{nil, nil, nil},
			wantNil: true,
		},
		{
			name:    "mixed nil",
			errs:    []error{nil, err1, nil, err2},
			want:    "first error | second error",
			wantNil: false,
		},
		{
			name:    "nil only",
			errs:    nil,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Join(tt.errs...)
			if tt.wantNil {
				if got != nil {
					t.Errorf("Join() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("Join() = nil, want %q", tt.want)
			}
			if got.Error() != tt.want {
				t.Errorf("Join() = %q, want %q", got.Error(), tt.want)
			}
		})
	}
}

func TestIs(t *testing.T) {
	sentinel := errors.New("sentinel")

	t.Run("nil error", func(t *testing.T) {
		if Is(nil, sentinel) {
			t.Error("Is(nil, sentinel) = true, want false")
		}
	})

	t.Run("nil target", func(t *testing.T) {
		if Is(errors.New("err"), nil) {
			t.Error("Is(err, nil) = true, want false")
		}
	})

	t.Run("match", func(t *testing.T) {
		wrapped := fmt.Errorf("outer: %w", sentinel)
		if !Is(wrapped, sentinel) {
			t.Error("Is(wrapped, sentinel) = false, want true")
		}
	})

	t.Run("no match", func(t *testing.T) {
		other := errors.New("other")
		wrapped := fmt.Errorf("outer: %w", other)
		if Is(wrapped, sentinel) {
			t.Error("Is(wrapped, sentinel) = true, want false")
		}
	})
}

func TestAs(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		var target *testError
		if As(nil, &target) {
			t.Error("As(nil, &target) = true, want false")
		}
	})

	t.Run("nil target", func(t *testing.T) {
		if As(errors.New("err"), nil) {
			t.Error("As(err, nil) = true, want false")
		}
	})

	t.Run("match", func(t *testing.T) {
		original := &testError{msg: "test"}
		wrapped := fmt.Errorf("outer: %w", original)
		var target *testError
		if !As(wrapped, &target) {
			t.Error("As(wrapped, &target) = false, want true")
		}
		if target.msg != "test" {
			t.Errorf("target.msg = %q, want %q", target.msg, "test")
		}
	})

	t.Run("no match", func(t *testing.T) {
		wrapped := errors.New("other error")
		var target *testError
		if As(wrapped, &target) {
			t.Error("As(wrapped, &target) = true, want false")
		}
	})
}

func TestUnwrap(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if Unwrap(nil) != nil {
			t.Error("Unwrap(nil) != nil")
		}
	})

	t.Run("unwrapped", func(t *testing.T) {
		inner := errors.New("inner")
		// errors without unwrap return nil
		if Unwrap(inner) != nil {
			t.Error("Unwrap(non-wrapped) != nil")
		}
	})

	t.Run("wrapped", func(t *testing.T) {
		inner := errors.New("inner")
		wrapped := fmt.Errorf("outer: %w", inner)
		got := Unwrap(wrapped)
		if got != inner {
			t.Errorf("Unwrap(wrapped) = %v, want %v", got, inner)
		}
	})
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
