// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"github.com/agezt/agezt/kernel/app/binding"
	"testing"
	"time"

	appapprovals "github.com/agezt/agezt/kernel/app/approvals"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// submitApproval files one waiting request and leaves it waiting.
//
// Submit BLOCKS until the request is decided, so the request is filed from a
// goroutine and the context is cancelled when the test is done with it.
func submitApproval(t *testing.T, registry *approval.Registry, correlation string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		registry.Submit(ctx, approval.SubmitSpec{Capability: "shell", ToolName: "shell", CorrelationID: correlation})
	}()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if registry.PendingCount() > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("the request %q never reached the queue", correlation)
}

// The host does not bind the approvals' ports — the APPLICATION fills them
// from the kernel handle.
//
// The test pins that binding end to end: the queue follows the ROUTED kernel —
// a tenant decides its own approvals — and the daemon's own request answers
// for the daemon's own queue.
func TestTheAdapterBindsTheApprovalPortsToTheRoutedKernel(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()
	tenant, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tenant.Close()

	// One request waits in the TENANT's queue.
	submitApproval(t, tenant.Approvals(), "corr-tenant")

	tenantCtx := context.WithValue(context.Background(), appHostKey{}, appHost{scope: scopeForKernel(tenant)})
	daemonCtx := context.WithValue(context.Background(), appHostKey{}, appHost{scope: scopeForKernel(root)})
	// The server is on the context too, so a binding that asked the DAEMON's
	// kernel instead of the routed one would answer — wrongly — rather than
	// fall over.
	s := NewServer(binding.ScopeFromKernel(root, t.TempDir()))
	tenantCtx = context.WithValue(tenantCtx, systemHostKey{}, s)
	daemonCtx = context.WithValue(daemonCtx, systemHostKey{}, s)

	// The request's ports follow the routed kernel.
	pending := requestApprovalPorts(tenantCtx).Broker().Pending()
	if len(pending) != 1 || pending[0].CorrelationID != "corr-tenant" {
		t.Fatalf("the request's approval ports report %v, want the tenant's own waiting request", pending)
	}

	// The daemon's own request answers for the daemon's own queue.
	if got := requestApprovalPorts(daemonCtx).Broker().Pending(); len(got) != 0 {
		t.Fatalf("the daemon's own approval ports report %v, want an empty queue", got)
	}
}

// The daemon-level binding answers for the DAEMON's queue: the mission
// control's pending count and the plan runner's node spec are the daemon's own
// surfaces, not a tenant's.
func TestTheAdapterBindsTheApprovalPortsToTheDaemon(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()
	tenant, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tenant.Close()

	submitApproval(t, tenant.Approvals(), "corr-tenant")

	s := NewServer(binding.ScopeFromKernel(root, t.TempDir()))

	pending := s.approvalPorts().Broker().Pending()
	if len(pending) != 0 {
		t.Fatalf("the daemon's approval ports report %v, want the DAEMON's own empty queue", pending)
	}
	if s.approvalPorts().Registry() != root.Approvals() {
		t.Fatal("the daemon's approval ports hand over a different registry than the daemon's kernel holds")
	}
}

// The application asks the port: the live approvals take the port's broker,
// and the broker is the daemon's own queue.
func TestTheLiveApprovalsReadThroughThePort(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()

	submitApproval(t, root.Approvals(), "corr-owned")

	ctx := context.WithValue(context.Background(), appHostKey{}, appHost{scope: scopeForKernel(root)})
	out, err := appapprovals.NewLive(requestApprovalPorts(ctx).Broker()).Pending(context.Background(), appapprovals.PendingRequest{})
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	if out.Count != 1 || out.Pending[0].CorrelationID != "corr-owned" {
		t.Fatalf("the live approvals report %v, want the daemon's own waiting request", out)
	}
}
