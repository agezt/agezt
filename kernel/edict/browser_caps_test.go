// SPDX-License-Identifier: MIT

package edict

import "testing"

// browser.read / memory / world were tool capabilities the engine never
// registered, so they hit the unknown-capability default-deny and were
// impossible to use or grant (M613). These pin that they're now first-class.

func TestNewlyRegisteredCaps_HaveDefaultsAndAreAllowed(t *testing.T) {
	e := New(Options{}) // defaults, AskAllow
	for _, cap := range []Capability{CapBrowserRead, CapMemory, CapWorld} {
		if _, ok := e.Levels()[cap]; !ok {
			t.Errorf("%q must have a default trust level (was unregistered)", cap)
		}
		// Under the default AskAllow, none of these should land on a hard deny.
		if o := e.Decide(cap, "{}"); o.Decision == DecisionDeny {
			t.Errorf("%q decided %v (reason %q); want not-deny under defaults", cap, o.Decision, o.Reason)
		}
	}
}

func TestNewlyRegisteredCaps_InAllCapabilities(t *testing.T) {
	all := map[Capability]bool{}
	for _, c := range AllCapabilities() {
		all[c] = true
	}
	for _, c := range []Capability{CapBrowserRead, CapMemory, CapWorld} {
		if !all[c] {
			t.Errorf("AllCapabilities() is missing %q — the policy control center can't list/grant it", c)
		}
	}
}

// UnknownAllow flips an unconfigured capability from default-deny to allow, so
// AGEZT_ALLOW_ALL covers even capabilities not in DefaultLevels (e.g. a future
// plugin tool), while hard-deny still fires first.
func TestUnknownAllow_AllowsUnconfiguredCap(t *testing.T) {
	const novel = Capability("some.plugin.cap")

	strict := New(Options{})
	if o := strict.Decide(novel, "{}"); o.Decision != DecisionDeny {
		t.Errorf("default engine: unconfigured cap should default-deny, got %v", o.Decision)
	}

	permissive := New(Options{UnknownAllow: true})
	if o := permissive.Decide(novel, "{}"); o.Decision != DecisionAllow {
		t.Errorf("UnknownAllow: unconfigured cap should allow, got %v (%q)", o.Decision, o.Reason)
	}
	// Hard-deny still wins even under UnknownAllow.
	if o := permissive.Decide(CapShell, "rm -rf /"); !o.HardDenied {
		t.Error("UnknownAllow must not bypass the hard-deny floor")
	}
}

// The tool→capability map resolves the three tools to their registered caps.
func TestToolMap_ResolvesNewTools(t *testing.T) {
	cases := map[string]Capability{
		"browser.read": CapBrowserRead,
		"memory":       CapMemory,
		"world":        CapWorld,
	}
	for tool, want := range cases {
		if got := CapabilityForToolCall(tool, []byte(`{}`)); got != want {
			t.Errorf("CapabilityForToolCall(%q) = %q, want %q", tool, got, want)
		}
	}
}
