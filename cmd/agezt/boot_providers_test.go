// SPDX-License-Identifier: MIT

package main

import (
	"io"
	"testing"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/modules/providers/boot"
	"github.com/agezt/agezt/kernel/modules/providers/catalog"
)

// TestBootDepsWiresThePorts pins the daemon's single wiring point: every boot
// path goes through bootDeps, and the bundle it returns carries the module's
// ports. The module refuses to run unwired — and the daemon's boot path is
// exactly the place where forgetting them would turn into a boot that never
// starts.
func TestBootDepsWiresThePorts(t *testing.T) {
	d := bootDeps(catalog.NewEmpty(), func(string) string { return "" }, t.TempDir(), io.Discard)
	if d.Ports == nil {
		t.Fatal("bootDeps returned a bundle without the wire adapters")
	}
	// And the bundle boots: the module accepts it and answers with the
	// unconfigured sentinel (no provider configured).
	t.Setenv(brand.EnvPrefix+"PROVIDER", "")
	t.Setenv(brand.EnvPrefix+"MODEL", "")
	t.Setenv(brand.EnvPrefix+"DEMO_ECHO", "")
	res, err := boot.Boot(d)
	if err != nil {
		t.Fatalf("Boot: %v", err)
	}
	if res.Primary != boot.UnconfiguredName {
		t.Errorf("Primary = %q, want the %q sentinel", res.Primary, boot.UnconfiguredName)
	}
}

// TestFirstRunNudgePolarity pins the banner nudge's polarity (Phase 2.4 drift
// fix #3): the "setup needed" nudge must fire exactly when the daemon booted
// UNCONFIGURED (sentinel primary), and must NOT fire for the deliberate
// AGEZT_DEMO_ECHO=1 mock. The old check (`model == "mock"`) was exactly
// backwards: the unconfigured sentinel resolves model "" (nudge never fired
// for the state that needs it), while demo-echo resolves model "mock" (nudge
// fired for a correctly configured demo daemon).
func TestFirstRunNudgePolarity(t *testing.T) {
	lookup := func(string) string { return "" }

	// Unconfigured boot → nudge fires.
	t.Setenv(brand.EnvPrefix+"PROVIDER", "")
	t.Setenv(brand.EnvPrefix+"MODEL", "")
	t.Setenv(brand.EnvPrefix+"DEMO_ECHO", "")
	unconf, err := boot.Boot(boot.Deps{Catalog: catalog.NewEmpty(), Lookup: lookup, BaseDir: t.TempDir(), Ports: providerPorts()})
	if err != nil {
		t.Fatalf("Boot (unconfigured): %v", err)
	}
	if unconf.Model == "mock" {
		t.Fatalf("sanity: unconfigured boot resolved model %q — the old model==\"mock\" check could never fire here", unconf.Model)
	}
	if !firstRunSetupNeeded(unconf) {
		t.Error("unconfigured boot must trigger the first-run setup nudge")
	}

	// Explicit demo-echo boot resolves model "mock" — this is where the OLD
	// inverted check fired; the nudge must stay silent.
	t.Setenv(brand.EnvPrefix+"DEMO_ECHO", "1")
	demo, err := boot.Boot(boot.Deps{Catalog: catalog.NewEmpty(), Lookup: lookup, BaseDir: t.TempDir(), Ports: providerPorts()})
	if err != nil {
		t.Fatalf("Boot (demo echo): %v", err)
	}
	if demo.Model != "mock" {
		t.Fatalf("sanity: demo-echo boot resolved model %q, want mock", demo.Model)
	}
	if firstRunSetupNeeded(demo) {
		t.Error("demo-echo boot (deliberately configured) must NOT trigger the setup nudge")
	}

	// Nil-safe.
	if firstRunSetupNeeded(nil) {
		t.Error("nil boot result must not trigger the nudge")
	}
}
