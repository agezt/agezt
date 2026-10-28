// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the memory family's ports — the APPLICATION fills
// them from the kernel handle.
//
// The test pins that binding end to end: the daemon's own memory answers for
// the daemon's kernel, and the request's memory follows the ROUTED kernel — a
// tenant's records are the tenant's records.
//
// The memory is the daemon's file (W3.57), so the test opens one per kernel:
// two memories, two records, and the ports must not mix them.
func TestTheAdapterBindsTheMemoryPortsToItsOwnKernel(t *testing.T) {
	root, err := runtime.Open(withMemory(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()
	tenant, err := runtime.Open(withMemory(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tenant.Close()

	rootRecord, _, err := root.Memory().Remember("corr", memoryapi.RememberSpec{
		Type: memoryapi.TypeFact, Subject: "daemon-record", Content: "the daemon's", Actor: "operator", Force: true,
	})
	if err != nil {
		t.Fatalf("Remember in the daemon's memory: %v", err)
	}
	tenantRecord, _, err := tenant.Memory().Remember("corr", memoryapi.RememberSpec{
		Type: memoryapi.TypeFact, Subject: "tenant-record", Content: "the tenant's", Actor: "operator", Force: true,
	})
	if err != nil {
		t.Fatalf("Remember in the tenant's memory: %v", err)
	}

	s := NewServer(root, t.TempDir())
	ctx := context.WithValue(context.Background(), appHostKey{}, appHost{kernel: tenant})

	// The daemon's own ports answer for its own kernel.
	if s.memoryPorts().Manager() == nil {
		t.Fatal("the daemon's memory ports hold no manager — the kernel holds a memory")
	}
	daemonRecords, err := s.memoryPorts().Active()
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if !recordsContain(daemonRecords, rootRecord.ID) {
		t.Error("the daemon's memory ports do not answer for its own kernel")
	}
	if recordsContain(daemonRecords, tenantRecord.ID) {
		t.Error("the daemon's memory ports see the tenant's record")
	}

	// The request's ports follow the routed kernel.
	if requestMemoryPorts(ctx).Manager() == nil {
		t.Fatal("the request's memory ports hold no manager — the routed kernel holds a memory")
	}
	tenantRecords, err := requestMemoryPorts(ctx).Active()
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if !recordsContain(tenantRecords, tenantRecord.ID) {
		t.Error("the request's memory ports do not answer for the routed kernel")
	}
	if recordsContain(tenantRecords, rootRecord.ID) {
		t.Error("the request's memory ports see the daemon's record — a tenant's records are the tenant's records")
	}
}

// recordsContain reports whether the records hold one with the id.
func recordsContain(records []memoryapi.Record, id string) bool {
	for _, r := range records {
		if r.ID == id {
			return true
		}
	}
	return false
}
