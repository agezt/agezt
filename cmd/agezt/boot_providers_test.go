// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/plugins/providerboot"
)

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
	unconf, err := providerboot.Boot(providerboot.Deps{Catalog: catalog.NewEmpty(), Lookup: lookup, BaseDir: t.TempDir()})
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
	demo, err := providerboot.Boot(providerboot.Deps{Catalog: catalog.NewEmpty(), Lookup: lookup, BaseDir: t.TempDir()})
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
