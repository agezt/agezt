// SPDX-License-Identifier: MIT

package binding

import (
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The bridge hands the handle to the family that owns the ports.
//
// The handle is the daemon's own kernel, and the family fills its own ports
// from it: that is the whole hand-over.
func TestTheBridgeFillsTheFamilysPortsFromTheHandle(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	called := false
	got := Fill(k, func(k *runtime.Kernel) string {
		called = true
		return k.Model()
	})
	if !called {
		t.Fatal("the bridge did not hand the handle to the family")
	}
	if got != "test-model" {
		t.Errorf("the family filled %q, want the daemon's own model", got)
	}
}

// A handle that is not the daemon's own yields the family's EMPTY ports.
//
// The bridge cannot know what the family does with the handle, so it answers
// with the zero value — and an empty port refuses rather than calling into
// nothing.
func TestTheBridgeAnswersWithEmptyPortsForAHandleThatIsNotTheDaemons(t *testing.T) {
	called := false
	got := Fill("not a daemon", func(*runtime.Kernel) string {
		called = true
		return "filled"
	})
	if called {
		t.Fatal("the bridge handed a foreign handle to the family")
	}
	if got != "" {
		t.Errorf("the bridge answered %q, want the family's empty value", got)
	}
}

// The bridge is a hand-over, not a decision: the family's own refusal
// reaches the caller as it is.
func TestTheBridgeHandsTheFamilysRefusalThrough(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	err = Fill(k, func(*runtime.Kernel) error { return errors.New("the family refused") })
	if err == nil || err.Error() != "the family refused" {
		t.Errorf("error = %v, want the family's own refusal", err)
	}
}
