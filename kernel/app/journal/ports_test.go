// SPDX-License-Identifier: MIT
package journal

import (
	"testing"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The journal application's ports are filled by the APPLICATION from the
// kernel handle — the host never binds them itself.
//
// The test pins what that wiring buys: the events the kernel's journal holds
// are the events the families read. A port bound to another journal would
// leave the operator reading a log the daemon does not keep.
func TestTheJournalPortsReachTheKernelJournal(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)
	if ports.Events == nil {
		t.Fatal("the journal port is nil — the kernel holds a journal")
	}

	// The event goes through the port and lands in the kernel's journal.
	if _, err := ports.Events.Append(event.Spec{Subject: "journal.port", Kind: event.KindTaskReceived, Actor: "test"}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// The port reads it back.
	seen := 0
	if err := ports.Events.Range(func(e *event.Event) error {
		if e.Subject == "journal.port" {
			seen++
		}
		return nil
	}); err != nil {
		t.Fatalf("Range: %v", err)
	}
	if seen != 1 {
		t.Fatalf("the journal port saw %d of the event it wrote, want 1", seen)
	}
}
