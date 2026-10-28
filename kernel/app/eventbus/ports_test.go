// SPDX-License-Identifier: MIT

package eventbus

import (
	"errors"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The event bus application's ports are filled by the APPLICATION from the
// kernel handle — the host never binds them itself.
//
// The test pins what that wiring buys: the events the port publishes are the
// events the kernel's bus carries, and a subscriber on the kernel's own bus
// sees them. A port bound to another bus would leave the operator watching a
// stream the daemon does not publish on.
func TestTheEventBusPortsReachTheKernelBus(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)
	if !ports.Ready() {
		t.Fatal("the bus port is not ready — the kernel holds a bus")
	}

	// A subscriber on the KERNEL's own bus sees what the port publishes.
	sub, err := k.Bus().Subscribe("bus.>", 16)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Cancel()

	if _, err := ports.Publish(event.Spec{Subject: "bus.port", Kind: event.KindInfo, Actor: "test"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case ev := <-sub.C:
		if ev.Subject != "bus.port" {
			t.Fatalf("the kernel's bus carried %q, want bus.port", ev.Subject)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the event published through the port never reached the kernel's bus")
	}

	// And the port subscribes to the same bus: an event published on the
	// kernel's bus reaches a subscriber the port opened.
	sub2, err := ports.Subscribe("bus.>", 16)
	if err != nil {
		t.Fatalf("Subscribe through the port: %v", err)
	}
	defer sub2.Cancel()
	if sub2 == nil {
		t.Fatal("the port subscribed to nothing")
	}
	if _, err := k.Bus().Publish(event.Spec{Subject: "bus.back", Kind: event.KindInfo, Actor: "test"}); err != nil {
		t.Fatalf("Publish on the kernel's bus: %v", err)
	}
	select {
	case ev := <-sub2.C:
		if ev.Subject != "bus.back" {
			t.Fatalf("the port's subscription carried %q, want bus.back", ev.Subject)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the event published on the kernel's bus never reached the port's subscription")
	}
}

// A host with no bus must not call into nothing.
//
// The adapter used to spell "there is no bus" itself, five times across five
// files. The application spells it once: a host with no bus publishes
// nothing, subscribes to nothing and has no redactor.
func TestTheEventBusPortsSurviveAHostWithNoBus(t *testing.T) {
	ports := PortsFromKernel(nil)
	if ports.Ready() {
		t.Fatal("a host with no kernel reports a ready bus")
	}
	if _, err := ports.Publish(event.Spec{Subject: "bus.none", Kind: event.KindInfo}); !errors.Is(err, ErrNoBus) {
		t.Fatalf("Publish on a host with no bus: %v, want ErrNoBus", err)
	}
	if _, err := ports.Subscribe("bus.>", 16); !errors.Is(err, ErrNoBus) {
		t.Fatalf("Subscribe on a host with no bus: %v, want ErrNoBus", err)
	}
	if ports.Redactor() != nil {
		t.Fatal("a host with no bus has a redactor")
	}
}
