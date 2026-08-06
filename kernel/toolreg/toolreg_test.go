// SPDX-License-Identifier: MIT

package toolreg

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/runtime"
)

// resetRegistryForTest empties the package-level registry so each test builds
// against exactly the fakes it registers.
func resetRegistryForTest() {
	regMu.Lock()
	defer regMu.Unlock()
	regOrder = nil
	registry = map[string]Spec{}
}

// fakeTool is a minimal agent.Tool with a fixed name.
type fakeTool struct{ name string }

func (f *fakeTool) Definition() agent.ToolDef { return agent.ToolDef{Name: f.name} }
func (f *fakeTool) Invoke(context.Context, json.RawMessage) (agent.Result, error) {
	return agent.Result{}, nil
}

// fakeGuarded is a netguard-capable fake: it records the callback it received.
type fakeGuarded struct {
	fakeTool
	mu      sync.Mutex
	onBlock func(ip, reason string)
}

func (f *fakeGuarded) SetOnBlock(fn func(ip, reason string)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onBlock = fn
}

func (f *fakeGuarded) block(ip, reason string) {
	f.mu.Lock()
	fn := f.onBlock
	f.mu.Unlock()
	if fn != nil {
		fn(ip, reason)
	}
}

func TestRegisterLookupNames(t *testing.T) {
	resetRegistryForTest()
	Register(Spec{Name: "a"})
	Register(Spec{Name: "b"})
	Register(Spec{Name: "a"}) // replace keeps order slot — idempotent RegisterAll
	if got := Names(); strings.Join(got, ",") != "a,b" {
		t.Fatalf("Names() = %v, want [a b]", got)
	}
	if _, ok := Lookup("a"); !ok {
		t.Fatal("Lookup(a) missing")
	}
	if _, ok := Lookup("zzz"); ok {
		t.Fatal("Lookup(zzz) unexpectedly found")
	}
}

func TestBuildAllSkipsUnbuiltAndCollectsExtra(t *testing.T) {
	resetRegistryForTest()
	Register(Spec{Name: "gated-off", Build: func(BuildDeps) (Built, error) {
		return Built{}, nil // env-gated off: nil Tool, no Extra
	}})
	Register(Spec{Name: "main", Build: func(BuildDeps) (Built, error) {
		return Built{Tool: &fakeTool{name: "main"}, Desc: "main()"}, nil
	}})
	Register(Spec{Name: "family", Build: func(BuildDeps) (Built, error) {
		return Built{
			Tool: &fakeTool{name: "family"},
			Extra: map[string]agent.Tool{
				"family.a": &fakeTool{name: "family.a"},
				"family.b": &fakeTool{name: "family.b"},
			},
			Desc: "family(+2 verbs)",
		}, nil
	}})
	set, err := BuildAll(BuildDeps{})
	if err != nil {
		t.Fatalf("BuildAll: %v", err)
	}
	tools := set.Tools()
	for _, want := range []string{"main", "family", "family.a", "family.b"} {
		if _, ok := tools[want]; !ok {
			t.Errorf("Tools() missing %q", want)
		}
	}
	if _, ok := tools["gated-off"]; ok {
		t.Error("gated-off spec must be skipped")
	}
	if len(tools) != 4 {
		t.Errorf("Tools() has %d entries, want 4: %v", len(tools), tools)
	}
	if got := strings.Join(set.Descs(), "|"); got != "main()|family(+2 verbs)" {
		t.Errorf("Descs() = %q", got)
	}
}

func TestBuildAllNameCollisionNamesBothClaimants(t *testing.T) {
	resetRegistryForTest()
	Register(Spec{Name: "first", Build: func(BuildDeps) (Built, error) {
		return Built{Tool: &fakeTool{name: "shared"}}, nil
	}})
	Register(Spec{Name: "second", Build: func(BuildDeps) (Built, error) {
		return Built{Tool: &fakeTool{name: "shared"}}, nil
	}})
	_, err := BuildAll(BuildDeps{})
	if err == nil {
		t.Fatal("expected collision error")
	}
	for _, want := range []string{`"shared"`, "first", "second"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("collision error %q missing %q", err, want)
		}
	}
}

func TestBuildAllBuildErrorAborts(t *testing.T) {
	resetRegistryForTest()
	boom := errors.New("boom")
	Register(Spec{Name: "bad", Build: func(BuildDeps) (Built, error) { return Built{}, boom }})
	if _, err := BuildAll(BuildDeps{}); !errors.Is(err, boom) {
		t.Fatalf("BuildAll err = %v, want wrapped boom", err)
	}
}

func TestConfigureInvokesHooksInRegistrationOrder(t *testing.T) {
	resetRegistryForTest()
	var order []string
	mk := func(name string) {
		Register(Spec{
			Name:  name,
			Build: func(BuildDeps) (Built, error) { return Built{Tool: &fakeTool{name: name}}, nil },
			Configure: func(tool agent.Tool, d KernelDeps) error {
				order = append(order, name)
				if tool.Definition().Name != name {
					t.Errorf("Configure(%s) got tool %q", name, tool.Definition().Name)
				}
				return nil
			},
		})
	}
	mk("one")
	mk("two")
	mk("three")
	set, err := BuildAll(BuildDeps{})
	if err != nil {
		t.Fatalf("BuildAll: %v", err)
	}
	if err := set.Configure(KernelDeps{}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if got := strings.Join(order, ","); got != "one,two,three" {
		t.Fatalf("hook order = %s, want one,two,three", got)
	}
}

func TestConfigureHookErrorNamesSpec(t *testing.T) {
	resetRegistryForTest()
	Register(Spec{
		Name:      "fragile",
		Build:     func(BuildDeps) (Built, error) { return Built{Tool: &fakeTool{name: "fragile"}}, nil },
		Configure: func(agent.Tool, KernelDeps) error { return errors.New("nope") },
	})
	set, err := BuildAll(BuildDeps{})
	if err != nil {
		t.Fatalf("BuildAll: %v", err)
	}
	err = set.Configure(KernelDeps{})
	if err == nil || !strings.Contains(err.Error(), "fragile") {
		t.Fatalf("Configure err = %v, want mention of fragile", err)
	}
}

func TestConfigureAutoWiresNetguard(t *testing.T) {
	resetRegistryForTest()
	main := &fakeGuarded{fakeTool: fakeTool{name: "netmain"}}
	extra := &fakeGuarded{fakeTool: fakeTool{name: "netmain.verb"}}
	plain := &fakeTool{name: "plain"}
	Register(Spec{Name: "netmain", Netguard: true, Build: func(BuildDeps) (Built, error) {
		return Built{Tool: main, Extra: map[string]agent.Tool{"netmain.verb": extra}}, nil
	}})
	Register(Spec{Name: "plain", Build: func(BuildDeps) (Built, error) {
		return Built{Tool: plain}, nil
	}})
	set, err := BuildAll(BuildDeps{})
	if err != nil {
		t.Fatalf("BuildAll: %v", err)
	}

	var mu sync.Mutex
	granted := map[string]bool{}     // tool names the factory minted a callback for
	published := map[string]string{} // tool name → "ip/reason" seen by the callback
	publish := func(toolName string) func(ip, reason string) {
		mu.Lock()
		granted[toolName] = true
		mu.Unlock()
		return func(ip, reason string) {
			mu.Lock()
			published[toolName] = ip + "/" + reason
			mu.Unlock()
		}
	}
	if err := set.Configure(KernelDeps{NetguardPublish: publish}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	if !granted["netmain"] || !granted["netmain.verb"] {
		t.Fatalf("netguard callbacks not minted for both instances: %v", granted)
	}
	if granted["plain"] {
		t.Fatal("non-Netguard spec must not be wired")
	}
	// The callbacks must actually be stored and route to the right tool name.
	main.block("10.0.0.1", "private")
	extra.block("169.254.169.254", "metadata")
	if published["netmain"] != "10.0.0.1/private" {
		t.Errorf("netmain publish = %q", published["netmain"])
	}
	if published["netmain.verb"] != "169.254.169.254/metadata" {
		t.Errorf("netmain.verb publish = %q", published["netmain.verb"])
	}
}

func TestNetguardGapsReportsUnawareNetguardSpec(t *testing.T) {
	resetRegistryForTest()
	Register(Spec{Name: "guarded-ok", Netguard: true, Build: func(BuildDeps) (Built, error) {
		return Built{Tool: &fakeGuarded{fakeTool: fakeTool{name: "guarded-ok"}}}, nil
	}})
	// Netguard:true but the built instance has no SetOnBlock — the LD-2 class
	// this registry exists to catch (the fetch tool lived like this for months).
	Register(Spec{Name: "guarded-gap", Netguard: true, Build: func(BuildDeps) (Built, error) {
		return Built{Tool: &fakeTool{name: "guarded-gap"}}, nil
	}})
	// Netguard spec that gates itself off: nothing built ⇒ not a gap.
	Register(Spec{Name: "guarded-off", Netguard: true, Build: func(BuildDeps) (Built, error) {
		return Built{}, nil
	}})
	set, err := BuildAll(BuildDeps{})
	if err != nil {
		t.Fatalf("BuildAll: %v", err)
	}
	gaps := set.NetguardGaps()
	if len(gaps) != 1 || gaps[0] != "guarded-gap" {
		t.Fatalf("NetguardGaps() = %v, want [guarded-gap]", gaps)
	}
}

func TestApplyPreOpenAndConfigureLate(t *testing.T) {
	resetRegistryForTest()
	var calls []string
	Register(Spec{
		Name:    "hooked",
		Build:   func(BuildDeps) (Built, error) { return Built{Tool: &fakeTool{name: "hooked"}}, nil },
		PreOpen: func(tool agent.Tool, cfg *runtime.Config) { calls = append(calls, "preopen") },
		Late: func(tool agent.Tool, d LateDeps) error {
			calls = append(calls, "late")
			return nil
		},
	})
	set, err := BuildAll(BuildDeps{})
	if err != nil {
		t.Fatalf("BuildAll: %v", err)
	}
	set.ApplyPreOpen(nil)
	if err := set.ConfigureLate(LateDeps{}); err != nil {
		t.Fatalf("ConfigureLate: %v", err)
	}
	if got := strings.Join(calls, ","); got != "preopen,late" {
		t.Fatalf("calls = %s", got)
	}
}
