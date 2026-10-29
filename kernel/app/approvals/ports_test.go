// SPDX-License-Identifier: MIT
package approvals_test

import (
	"errors"
	"reflect"
	"testing"

	appapprovals "github.com/agezt/agezt/kernel/app/approvals"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The approvals family's ports are filled by the APPLICATION from the kernel
// handle — the host never binds them itself.
//
// The test pins what that wiring buys: the registry the port hands over is the
// daemon's own queue. A port bound to nothing would hand the live approvals no
// broker at all, and every request would refuse.
func TestTheApprovalPortsReachTheKernelRegistry(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := appapprovals.PortsFromKernel(k)

	if ports.Registry() == nil {
		t.Fatal("the approval port hands over no registry — the daemon holds one")
	}
	if ports.Registry() != k.Approvals() {
		t.Fatal("the approval port hands over a different registry than the kernel holds")
	}
	if ports.Broker() == nil {
		t.Fatal("the approval port hands over no broker — the daemon holds a queue")
	}
}

// A host with no kernel holds no approval queue.
//
// The port answers for that case with nothing at all: there is no registry to
// read from, and the surfaces that read it refuse instead of reading from
// nothing.
func TestTheApprovalPortsSurviveAHostWithNoKernel(t *testing.T) {
	ports := appapprovals.PortsFromKernel(nil)

	if ports.Registry() != nil {
		t.Fatal("a host with no kernel hands over an approval registry")
	}
	if ports.Broker() != nil {
		t.Fatal("a host with no kernel hands over an approval broker")
	}
}

// The port's names may not be the kernel's own: a port that carried the name
// of the verb it stands for would let the adapter name the kernel's verb
// through the port's door.
//
// The rule is checked HERE, by reflection over the port's own method set and
// the kernel's, so it does not depend on what the guard happens to own today.
func TestTheApprovalPortNamesAreNotTheKernelVerbs(t *testing.T) {
	port := reflect.TypeOf(appapprovals.Ports{})
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the approval port's %s is the kernel's own verb — a port may not carry the name of the verb it stands for", name)
		}
	}
	if port.NumMethod() == 0 {
		t.Fatal("the approval port has no methods — the family cannot answer anything")
	}
}

// A daemon that holds no queue REFUSES: it does not panic on a nil broker, and
// it says the same thing every time.
//
// The refusal is written once — in the family that answers for the approvals —
// so two surfaces cannot answer the same question two ways.
func TestTheLiveApprovalsRefuseWithoutABroker(t *testing.T) {
	live := appapprovals.NewLive(nil)

	if _, err := live.Pending(t.Context(), appapprovals.PendingRequest{}); !errors.Is(err, appapprovals.ErrNoBroker) {
		t.Fatalf("the pending list reports %v, want the family's own refusal", err)
	}
	if _, err := live.Decide(t.Context(), appapprovals.DecideRequest{}); !errors.Is(err, appapprovals.ErrNoBroker) {
		t.Fatalf("the decision reports %v, want the family's own refusal", err)
	}
}
