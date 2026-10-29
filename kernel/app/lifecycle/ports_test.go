// SPDX-License-Identifier: MIT

package lifecycle

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The daemon lifecycle family's ports are filled by the APPLICATION from the
// kernel handle — the host never binds them itself.
//
// The test pins the FILLED half: the port really DRAINS the daemon. The drain
// cancels the runs in flight, waits for them and halts the daemon — and the
// daemon's own halted flag says so afterwards. A port bound to nothing would
// refuse, and the update flow would never drain anything.
func TestTheLifecyclePortsDrainTheDaemon(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)

	timedOut, active, err := ports.Drain(time.Second)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if timedOut {
		t.Fatal("the drain timed out with no runs in flight")
	}
	if active != 0 {
		t.Fatalf("the drain reported %d runs still active, want none", active)
	}
	if !k.IsHalted() {
		t.Fatal("the drain did not halt the daemon")
	}
}

// A run that would not stop is the drain's own hard case, and the daemon's
// own answer is what says so: the drain gave up before the run was done, and
// it says so with its timeout.
//
// The run hangs on a provider that never answers until the test lets it, so
// the drain has something real to wait for.
func TestTheLifecyclePortsReportADrainThatGaveUp(t *testing.T) {
	provider := mock.New()
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	provider.Responder = func(llm.CompletionRequest) llm.CompletionResponse {
		once.Do(func() { close(started) })
		<-release
		return mock.FinalText("late")
	}
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: provider})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()
	go func() { _, _ = k.RunWith(context.Background(), k.NewCorrelation(), "hang") }()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("the run never reached the provider")
	}

	timedOut, _, err := PortsFromKernel(k).Drain(50 * time.Millisecond)
	close(release)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if !timedOut {
		t.Fatal("the drain did not report the timeout it hit with a run still in flight")
	}
}

// A host with no kernel holds no daemon to drain.
//
// The port REFUSES, and the refusal is a refusal: the caller must hear that
// the daemon could not be drained, and not read "no runs were active" as
// "drained" — the update flow would land a new binary under a running daemon.
func TestTheLifecyclePortsRefuseForAHostWithNoKernel(t *testing.T) {
	timedOut, active, err := PortsFromKernel(nil).Drain(time.Second)
	if err == nil {
		t.Fatal("a host with no kernel drained a daemon")
	}
	if timedOut || active != 0 {
		t.Fatalf("the refusal answered %v/%d, want no answer at all", timedOut, active)
	}
}

// The port's names may not be the kernel's own: a port that carried the name
// of the verb it stands for would let the adapter name the kernel's verb
// through the port's door.
//
// The rule is checked HERE, by reflection over the port's own method set and
// the kernel's, so it does not depend on what the guard happens to own today.
func TestTheLifecyclePortNamesAreNotTheKernelVerbs(t *testing.T) {
	port := reflect.TypeOf(Ports{})
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the lifecycle port's %s is the kernel's own verb — a port may not carry the name of the verb it stands for", name)
		}
	}
	if port.NumMethod() == 0 {
		t.Fatal("the lifecycle port has no methods — the family cannot answer anything")
	}
}
