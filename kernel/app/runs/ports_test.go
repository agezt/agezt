// SPDX-License-Identifier: MIT

package runs

import (
	"context"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/scheduler"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The plan runner's ports REFUSE when there is no daemon.
//
// The plan runner's ports are filled from the daemon's own kernel, by type:
// there is no foreign handle to fill them with any more. A host with no
// daemon holds no plan machinery: the loop is nowhere and the plan verb
// refuses, so a submitted plan is answered by nobody.
func TestThePlanRunnerPortsRefuseWithoutADaemon(t *testing.T) {
	ports := PlanRunnerPortsFromKernel(nil)

	if runner := ports.LoopRunner(); runner != nil {
		t.Error("a host with no daemon holds a plan loop")
	}
	if _, err := ports.RunPlan(context.Background(), scheduler.Plan{}, "plan-1"); err == nil {
		t.Error("a host with no daemon ran a plan")
	}
}

// The runs family's ports are filled by the APPLICATION from the kernel
// handle — the host never binds them itself.
//
// The test pins the FILLED half: the port really STARTS the daemon's run and
// brings the answer back. A port bound to nothing would hand the surfaces no
// runner at all, and they would be told the run capability is unavailable — so
// a port that always refuses passes a daemon-with-no-kernel's test and is
// wrong.
func TestTheRunPortsLaunchTheDaemonRuns(t *testing.T) {
	provider := mock.New(mock.FinalText("the answer"))
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: provider})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)

	corr := k.NewCorrelation()
	answer, err := ports.Launch(context.Background(), corr, "do the thing", nil)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if answer != "the answer" {
		t.Fatalf("the plain run answered %q, want the daemon's own answer", answer)
	}
	if provider.CallCount() == 0 {
		t.Fatal("the plain run never reached the provider")
	}
	// The correlation is the run's identity: the run is journaled under it,
	// and a port that dropped it would leave the run untraceable.
	journaled := 0
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.CorrelationID == corr {
			journaled++
		}
		return nil
	})
	if journaled == 0 {
		t.Fatal("the run was not journaled under the correlation the port carried")
	}
}

// And the retry half: a policy means the run retries under it.
//
// The provider here is scripted with NO responses, so every attempt fails —
// which is what makes the two paths tell each other apart: a PLAIN run makes
// one attempt, a retrying run makes as many as the policy allows. A port that
// carried the policy away would make one attempt, and the test sees it.
func TestTheRunPortsLaunchARetryingRun(t *testing.T) {
	provider := mock.New()
	attempts := 0
	provider.OnRequest = func(llm.CompletionRequest) { attempts++ }
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: provider})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)

	policy := agentapi.RetryPolicy{MaxAttempts: 2, RetryOn: []string{"error"}}
	if _, err := ports.Launch(context.Background(), k.NewCorrelation(), "do the thing again", &policy); err == nil {
		t.Fatal("a run whose provider never answers should fail")
	}
	if attempts != 2 {
		t.Fatalf("the retrying run made %d attempts, want two under the policy", attempts)
	}
}

// And the plain half of the same coin: no policy means ONE attempt, however
// badly the run went.
func TestTheRunPortsLaunchAPlainRunWithoutRetrying(t *testing.T) {
	provider := mock.New()
	attempts := 0
	provider.OnRequest = func(llm.CompletionRequest) { attempts++ }
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: provider})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)

	if _, err := ports.Launch(context.Background(), k.NewCorrelation(), "do the thing once", nil); err == nil {
		t.Fatal("a run whose provider never answers should fail")
	}
	if attempts != 1 {
		t.Fatalf("the plain run made %d attempts, want one", attempts)
	}
}

// A host with no kernel holds no run capability.
//
// The port answers for that case with a refusal, and the refusal is a
// refusal: the surfaces are told the run capability is unavailable rather
// than handed a call into nothing.
func TestTheRunPortsRefuseForAHostWithNoKernel(t *testing.T) {
	ports := PortsFromKernel(nil)

	answer, err := ports.Launch(context.Background(), "corr", "do the thing", nil)
	if err == nil {
		t.Fatal("a host with no kernel started a run", answer)
	}
	if err.Error() != errNoRunner.Error() {
		t.Fatalf("the port refused for the wrong reason: %v", err)
	}
}

// The port's names may not be the kernel's own: a port that carried the name
// of the verb it stands for would let the adapter name the kernel's verb
// through the port's door.
//
// The rule is checked HERE, by reflection over the port's own method set and
// the kernel's, so it does not depend on what the guard happens to own today.
func TestTheRunPortNamesAreNotTheKernelVerbs(t *testing.T) {
	port := reflect.TypeOf(Ports{})
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the run port's %s is the kernel's own verb — a port may not carry the name of the verb it stands for", name)
		}
	}
	if port.NumMethod() == 0 {
		t.Fatal("the run port has no methods — the family cannot answer anything")
	}
}
