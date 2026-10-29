// SPDX-License-Identifier: MIT

package correlation

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The correlation family's ports are filled by the APPLICATION from the kernel
// handle — the host never binds them itself.
//
// The test pins what that wiring buys: the correlation the port mints is the
// daemon's own, and the actor it puts into the context is the one the daemon
// would have put there. A port bound to nothing would mint an empty identity,
// and an operation with an empty identity cannot be traced.
func TestTheCorrelationPortsReachTheKernelMint(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)
	corr := ports.Mint()
	if corr == "" {
		t.Fatal("the correlation port mints nothing — the daemon mints a correlation for every operation")
	}

	// Two mints are two identities.
	if again := ports.Mint(); again == corr {
		t.Fatal("the correlation port minted the same correlation twice")
	}

	// The actor decoration is the kernel's own.
	//
	// The correlation itself is the kernel's private context key — the kernel
	// has no public reader for it — so what the test can read is the ACTOR,
	// which the kernel does publish.
	ctx := ports.WithActor(context.Background(), "operator", corr)
	if got := k.ActorFromCtx(ctx); got != "operator" {
		t.Fatalf("the correlation port put the actor %q into the context, want %q", got, "operator")
	}
}

// A host with no kernel has no correlation.
//
// There is nothing to mint and nobody to name in the context: the port says
// so by minting the empty correlation and by leaving the context alone.
func TestTheCorrelationPortsSurviveAHostWithNoKernel(t *testing.T) {
	ports := PortsFromKernel(nil)
	if got := ports.Mint(); got != "" {
		t.Fatalf("a host with no kernel mints the correlation %q", got)
	}
	ctx := context.Background()
	if out := ports.WithActor(ctx, "operator", "corr"); out != ctx {
		t.Fatal("a host with no kernel rewrites the context")
	}
}
