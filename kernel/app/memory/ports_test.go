// SPDX-License-Identifier: MIT

package memory

import (
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/internal/testfixtures"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The memory family's ports are filled by the APPLICATION from the kernel
// handle — the host never binds them itself.
//
// The test pins what that wiring buys: the records the kernel's memory holds
// are the records the port reads, and the record the port forgets is really
// forgotten. A port bound to another memory would leave the operator reading a
// store the daemon does not keep.
func TestTheMemoryPortsReachTheKernelMemory(t *testing.T) {
	dir := t.TempDir()
	store, builder, err := testfixtures.MemoryPorts(dir)
	if err != nil {
		t.Fatalf("open the memory store: %v", err)
	}
	k, err := runtime.Open(runtime.Config{
		BaseDir:       dir,
		Provider:      mock.New(),
		MemoryStore:   store,
		MemoryBuilder: builder,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)
	if ports.Manager() == nil {
		t.Fatal("the memory port holds no manager — the kernel holds a memory")
	}

	record, _, err := ports.Manager().Remember("corr", memoryapi.RememberSpec{
		Type: memoryapi.TypeFact, Subject: "port-record", Content: "the memory port reads this", Actor: "operator", Force: true,
	})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	// The port lists the memory's active records.
	records, err := ports.Active()
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	found := false
	for _, r := range records {
		if r.ID == record.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("the memory port listed %d records, none of them the one the kernel's memory holds", len(records))
	}

	// And it forgets one of them.
	forgotten, err := ports.Forget(record.ID)
	if err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if !forgotten {
		t.Fatal("the memory port did not forget the record")
	}
	after, err := ports.Active()
	if err != nil {
		t.Fatalf("Active after the forget: %v", err)
	}
	for _, r := range after {
		if r.ID == record.ID {
			t.Fatal("the record the port forgot is still active")
		}
	}
}

// A host with no memory must not call into nothing.
//
// The adapter used to spell "there is no memory" itself: it asked the kernel
// whether there was one, and it wrote the forget's refusal out of the
// kernel's own. The application spells it once.
func TestTheMemoryPortsSurviveAHostWithNoMemory(t *testing.T) {
	ports := PortsFromKernel(nil)
	if ports.Manager() != nil {
		t.Fatal("a host with no kernel hands out a memory manager")
	}
	records, err := ports.Active()
	if err != nil {
		t.Fatalf("Active on a host with no memory: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("a host with no memory lists %d records", len(records))
	}
	if _, err := ports.Forget("r-1"); !errors.Is(err, ErrNoMemory) {
		t.Fatalf("Forget on a host with no memory: %v, want the memory's refusal", err)
	}
	if ports.NewCorrelation() != "" {
		t.Fatal("a host with no memory mints a correlation")
	}
	if _, err := ports.DistillBrain(t.Context(), "corr"); !errors.Is(err, ErrNoMemory) {
		t.Fatalf("DistillBrain on a host with no memory: %v, want the memory's refusal", err)
	}
}
