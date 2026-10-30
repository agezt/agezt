// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"github.com/agezt/agezt/kernel/app/binding"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/executionprofile"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestDispatchedSeatRefusesEveryRemoteBackend: the dispatched seat path
// refuses every remote backend the execution profile list names, with the
// same message — and it refuses them before it resolves anything, so a new
// remote backend cannot slip past this gate.
func TestDispatchedSeatRefusesEveryRemoteBackend(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(binding.ScopeFromKernel(k, t.TempDir()))
	for _, id := range executionprofile.RemoteBackendIDs {
		_, label, err := s.applyWardenExecutionProfile(context.Background(), id)
		if err == nil || !strings.Contains(err.Error(), "is only available on direct runs") {
			t.Fatalf("%q: the dispatched seat must refuse a remote backend (label %q, err %v)", id, label, err)
		}
		if label != "" {
			t.Fatalf("%q: a refused profile reports no label, got %q", id, label)
		}
	}
	// The empty profile id is not a profile at all.
	if _, label, err := s.applyWardenExecutionProfile(context.Background(), "  "); err != nil || label != "" {
		t.Fatal("the empty profile id", label, err)
	}
	// A warden-family profile still resolves.
	if _, label, err := s.applyWardenExecutionProfile(context.Background(), "warden"); err != nil || label != string(warden.ProfileNamespace) {
		t.Fatal("the warden profile", label, err)
	}
}
