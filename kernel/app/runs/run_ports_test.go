// SPDX-License-Identifier: MIT

package runs

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
)

// The run service's ports are filled by the APPLICATION from the kernel
// handle — the composition root hands over the ports, never the handle.
//
// The test pins the FILLED half: the port really READS the daemon's roster,
// the daemon's default model and the daemon's tool registry, it really MINTS a
// correlation, and it really asks the vision gate. A port bound to nothing
// would answer "no kernel" for every question, and the resolve path would
// refuse every run while looking perfectly wired.
func TestTheRunPortsReachTheDaemon(t *testing.T) {
	k := runKernel(t)
	ports := RunPortsFromKernel(k)

	if !ports.Ready() {
		t.Fatal("the port reports no daemon — the resolve path would refuse every run")
	}

	// The roster half: the agent a run runs as.
	if _, err := k.AddProfile(agentapi.Profile{Slug: "scout", Name: "Scout", Enabled: true}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	got, found := ports.Profile("scout")
	if !found || got.Name != "Scout" {
		t.Fatalf("the port did not find the agent the roster holds: %+v found=%v", got, found)
	}
	if _, found := ports.Profile("nobody"); found {
		t.Error("the port found an agent the roster does not hold")
	}

	// The identity half: the daemon's default model.
	if got, want := ports.DefaultModel(), k.Model(); got != want {
		t.Errorf("the port's default model = %q, want the daemon's %q", got, want)
	}

	// The correlation half.
	if corr := ports.Correlation(); corr == "" {
		t.Error("the port minted no correlation — a run would be journaled under nothing")
	}

	// The vision gate half: the request itself when it carries no images.
	admission, err := ports.AdmitVision(context.Background(), "corr", "mock-model", "look at this", nil)
	if err != nil {
		t.Fatalf("AdmitVision: %v", err)
	}
	if admission.Intent != "look at this" {
		t.Errorf("the admission's intent = %q", admission.Intent)
	}
	if len(admission.Images) != 0 {
		t.Errorf("the admission admitted images the run did not ask for: %v", admission.Images)
	}

	// The tools half: the registry the execution profiles ask about.
	if ports.HasTool("no-such-tool") {
		t.Error("the port found a tool the daemon does not register")
	}
}

// A host with no daemon holds no daemon to run on.
//
// The READS answer with nothing, and the vision gate answers with the request
// itself: a request that carries no images is admitted as it is.
func TestTheRunPortsRefuseForAHostWithNoDaemon(t *testing.T) {
	ports := RunPortsFromKernel(nil)

	if ports.Ready() {
		t.Fatal("a host with no daemon reports a daemon")
	}
	if _, found := ports.Profile("scout"); found {
		t.Error("a host with no daemon holds an agent")
	}
	if got := ports.DefaultModel(); got != "" {
		t.Errorf("a host with no daemon has a default model: %q", got)
	}
	if got := ports.Correlation(); got != "" {
		t.Errorf("a host with no daemon minted a correlation: %q", got)
	}
	if ports.HasTool("shell") {
		t.Error("a host with no daemon registers a tool")
	}
	admission, err := ports.AdmitVision(context.Background(), "corr", "model", "do it", nil)
	if err != nil {
		t.Fatalf("AdmitVision: %v", err)
	}
	if admission.Intent != "do it" {
		t.Errorf("the admission's intent = %q, want the request's own", admission.Intent)
	}
}

// The port SEES the daemon's tool registry: a tool the daemon registers is
// found, and a tool it does not is not.
//
// The half that matters is the positive one. A port that answered "no" for
// every tool would look right to a test that only asks about tools nobody
// registered — and it would refuse every execution profile that needs the
// shell.
func TestTheRunPortsFindARegisteredTool(t *testing.T) {
	ports := RunPortsFromKernel(runKernelWithTools(t))

	if !ports.HasTool("shell") {
		t.Error("the port did not find the shell tool the daemon registers")
	}
	if !ports.HasTool("file") {
		t.Error("the port did not find the file tool the daemon registers")
	}
	if ports.HasTool("no-such-tool") {
		t.Error("the port found a tool the daemon does not register")
	}
}

// The port holds the WARDEN behind the execution profile family's own ports:
// the run asks about the isolation surface through them, and a port that
// held nothing would answer about no isolation at all.
func TestTheRunPortsHoldTheWarden(t *testing.T) {
	ports := RunPortsFromKernel(runKernel(t))

	exec := ports.ExecProfile()
	if exec.Inventory == nil {
		t.Error("the run's ports hold no inventory — the execution profile family would answer about nothing")
	}
	if exec.EffectiveProfile == nil {
		t.Error("the run's ports hold no warden — the effective isolation surface could not be asked for")
	}
}

// The run service REFUSES a host that holds no daemon, and it refuses it
// with the refusal the family wrote once.
//
// The refusal is a value, not a string: the port's answer has to be
// comparable by whoever asks, and a run path that spelled the refusal again
// would give two answers for one absence.
func TestTheRunServiceRefusesAHostWithNoDaemon(t *testing.T) {
	service := NewRunService(RunPortsFromKernel(nil))

	_, err := service.resolve(context.Background(), RunRequest{
		Intent: json.RawMessage(`"x"`),
	})
	if err == nil {
		t.Fatal("a host with no daemon started a run")
	}
	if !errors.Is(err, ErrNoKernel) {
		t.Fatalf("error = %v, want the family's own refusal", err)
	}
}

// The run ASKS the vision gate before it looks at anything.
//
// The daemon's model cannot see, so the gate refuses — and the refusal is
// the port's to carry: a port that answered with the request itself would
// admit images the daemon never looked at.
func TestTheRunServiceAsksTheVisionGate(t *testing.T) {
	service := NewRunService(RunPortsFromKernel(runKernel(t)))

	_, err := service.resolve(context.Background(), RunRequest{
		Intent: json.RawMessage(`"look at this"`),
		Images: json.RawMessage(`["an-image"]`),
	})
	if err == nil {
		t.Fatal("the run looked at an image without asking the vision gate")
	}
	if !strings.Contains(err.Error(), "does not support vision") {
		t.Fatalf("error = %v, want the vision gate's refusal", err)
	}
}

// The remote-agezt profile is refused when the daemon registers no
// remote_run tool: the profile's whole meaning is a peer, and a peer-less
// daemon cannot delegate.
func TestTheRunServiceRefusesTheRemoteProfileWithoutPeers(t *testing.T) {
	service := NewRunService(RunPortsFromKernel(runKernel(t)))

	_, err := service.resolve(context.Background(), RunRequest{
		Intent:      json.RawMessage(`"x"`),
		ExecProfile: json.RawMessage(`"remote-agezt"`),
	})
	if err == nil {
		t.Fatal("the remote profile was accepted without a peer")
	}
	if !strings.Contains(err.Error(), "requires configured AGEZT peers") {
		t.Fatalf("error = %v, want the peers refusal", err)
	}
}

// The daemon's default prompt is the IDENTITY family's own, and the run's
// port carries it: a run that asks "is a daemon-default prompt configured?"
// must be answered by the family that owns the daemon's default identity.
func TestTheRunPortsCarryTheDaemonsDefaultPrompt(t *testing.T) {
	k := runKernel(t)
	k.SetSystem("the daemon's own prompt")
	ports := RunPortsFromKernel(k)

	if got := ports.Prompt(); got != "the daemon's own prompt" {
		t.Errorf("the prompt = %q, want the daemon's own", got)
	}
}

// The run's own events reach the caller.
//
// The execute path subscribes to the run's OWN subject before the run starts,
// and the events the run publishes land there. A path that subscribed to
// another subject would answer correctly and show nothing — the operator
// would watch an empty stream.
func TestTheRunEventsReachTheCaller(t *testing.T) {
	service := NewRunService(RunPortsFromKernel(runKernel(t)))

	events := 0
	out, err := service.Execute(context.Background(), RunRequest{
		Intent: json.RawMessage(`"say hi"`),
	}, func(*event.Event) error {
		events++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Answer == nil || *out.Answer != "the answer" {
		t.Fatalf("answer = %v, want the run's own", out.Answer)
	}
	if events == 0 {
		t.Fatal("the run's own events never reached the caller — the execute path listened on the wrong subject")
	}
}

// The assured loop is the daemon's own, and the run's port carries it: the
// run asks for the assured answer, not for a plain one.
func TestTheRunPortsCarryTheAssuredLoop(t *testing.T) {
	ports := RunPortsFromKernel(runKernel(t))

	answer, _, err := ports.Assured(context.Background(), "corr", "say hi", 1)
	if err != nil {
		t.Fatalf("Assured: %v", err)
	}
	if answer != "the answer" {
		t.Errorf("the assured answer = %q, want the run's own", answer)
	}
}

// The run service's port names may not be the kernel's own: a port that
// carried the name of the verb it stands for would let the adapter name the
// kernel's verb through the port's door.
func TestTheRunServicePortNamesAreNotTheKernelVerbs(t *testing.T) {
	port := reflect.TypeOf(RunPorts{})
	kernel := reflect.TypeOf(&runtime.Kernel{})
	if port.NumMethod() == 0 {
		t.Fatal("the run service's port has no methods — the family cannot answer anything")
	}
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the run service's port %s is the kernel's own verb — a port may not carry the name of the verb it stands for", name)
		}
	}
}

// The EXECUTE path's capabilities are really the daemon's: the run's own
// subject on the bus, the bus itself, the daemon's run verbs, the daemon's
// ceiling, the daemon's default prompt, the live provider, the catalog and
// the tool registry.
//
// This is the half that says the port is FILLED. A port that answered empty
// for every one of these would let a run start on a host that has no daemon
// and would report a plan that knows nothing about the daemon it would run
// on.
func TestTheRunPortsReachTheDaemonForTheExecutePath(t *testing.T) {
	k := runKernelWithToolsAndCeiling(t)
	ports := RunPortsFromKernel(k)

	// The bus half: the run's own subject, and the bus to subscribe on.
	if got, want := ports.Subject("abc"), "agent.agent-abc.>"; got != want {
		t.Errorf("the run's subject = %q, want %q", got, want)
	}
	if !ports.EventBus().Ready() {
		t.Error("the port holds no bus — the run could not stream its events")
	}

	// The run verbs half: the launcher starts and cancels.
	if ports.Launcher().Cancel("no-such-run") {
		t.Error("the port cancelled a run that does not exist")
	}
	if _, err := ports.Launcher().Launch(context.Background(), "corr", "say hi", nil); err != nil {
		t.Errorf("the launcher refused a plain run: %v", err)
	}

	// The daemon's own ceiling.
	if got, want := ports.Ceiling(), k.MaxDuration(); got != want {
		t.Errorf("the ceiling = %v, want the daemon's %v", got, want)
	}

	// The daemon's default prompt — the identity family's own.
	if got, want := ports.Prompt(), k.System(); got != want {
		t.Errorf("the prompt = %q, want the daemon's %q", got, want)
	}

	// The live provider, the catalog and the tool registry.
	if ports.LiveProvider() == nil {
		t.Error("the port holds no live provider — the run would have nothing to talk to")
	}
	if ports.CatalogSnapshot() == nil {
		t.Error("the port holds no catalog — the plan could not say what the model can do")
	}
	if _, ok := ports.RegisteredTools()["shell"]; !ok {
		t.Error("the port's registry does not hold the shell tool the daemon registers")
	}

	// The runner: the request really reaches the daemon's tool runner. The
	// runner answers with the tool's own refusal when the tool is not
	// trusted — what matters is that the refusal comes from the RUNNER and
	// not from an empty port.
	_, err := ports.InvokeTool(context.Background(), "corr", "agent", "file", json.RawMessage(`{}`))
	if err == nil {
		t.Error("the runner did not answer — the port may not be wired to the daemon's runner")
	} else if !strings.Contains(err.Error(), "file") {
		t.Errorf("the runner's error = %v, want it to name the tool it refused", err)
	}
}

// A host with no daemon holds no daemon to run on, and the execute path's
// capabilities say so rather than calling into nothing.
func TestTheRunPortsRefuseTheExecutePathWithoutADaemon(t *testing.T) {
	ports := RunPortsFromKernel(nil)

	if ports.EventBus().Ready() {
		t.Error("a host with no daemon holds a bus")
	}
	if got := ports.Subject("abc"); got != "" {
		t.Errorf("a host with no daemon has a run subject: %q", got)
	}
	if ports.Launcher().Cancel("abc") {
		t.Error("a host with no daemon cancelled a run")
	}
	if _, err := ports.Launcher().Launch(context.Background(), "corr", "x", nil); err == nil {
		t.Error("a host with no daemon started a run")
	}
	if _, _, err := ports.Assured(context.Background(), "corr", "x", 1); err == nil {
		t.Error("a host with no daemon ran the assured loop")
	}
	if got := ports.Ceiling(); got != 0 {
		t.Errorf("a host with no daemon has a ceiling: %v", got)
	}
	if got := ports.Prompt(); got != "" {
		t.Errorf("a host with no daemon has a default prompt: %q", got)
	}
	if ports.LiveProvider() != nil {
		t.Error("a host with no daemon holds a live provider")
	}
	if cat := ports.CatalogSnapshot(); cat == nil || len(cat.Providers) != 0 {
		t.Errorf("a host with no daemon holds a catalog with providers: %v", cat)
	}
	if got := ports.RegisteredTools(); got != nil {
		t.Errorf("a host with no daemon registers tools: %v", got)
	}
	if _, err := ports.InvokeTool(context.Background(), "corr", "agent", "shell", json.RawMessage(`{}`)); err == nil {
		t.Error("a host with no daemon ran a tool")
	}
	if err := ports.PublishRemoteEvent("corr", event.KindInfo, "task", map[string]any{}); err == nil {
		t.Error("a host with no daemon published a remote event")
	}
}

// The remote run's own event reaches the run's stream: the subject is the
// run's own, and the bus carries it.
//
// The mirror publishes through this port, so a remote event that landed
// somewhere else would leave the operator watching an empty stream.
func TestTheRunPortsPublishTheRemoteEventOntoTheRunStream(t *testing.T) {
	ports := RunPortsFromKernel(runKernel(t))

	sub, err := ports.EventBus().Subscribe(ports.Subject("corr-remote"), 16)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Cancel()

	if err := ports.PublishRemoteEvent("corr-remote", event.KindInfo, "task", map[string]any{"phase": "completed"}); err != nil {
		t.Fatalf("PublishRemoteEvent: %v", err)
	}

	select {
	case ev := <-sub.C:
		if ev == nil {
			t.Fatal("the subscription closed without an event")
		}
		if ev.CorrelationID != "corr-remote" {
			t.Errorf("the event's correlation = %q, want the run's own", ev.CorrelationID)
		}
		if ev.Subject != "agent.agent-corr-remote.task" {
			t.Errorf("the event's subject = %q, want the run's own subject", ev.Subject)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the remote event never reached the run's stream")
	}
}
