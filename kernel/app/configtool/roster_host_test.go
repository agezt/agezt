// SPDX-License-Identifier: MIT

package configtool

// Provenance: the test host helper for the roster (W3.56). The kernel no
//             longer opens the roster file itself — the composition root
//             does (cmd/agezt) and hands the kernel the contract's port — so
//             a test that wants a roster opens one and injects it, the same
//             way the daemon does.

import (
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/modules/agents/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// withRoster gives the config a roster: a real store under the config's base
// dir, the way the daemon opens roster/roster.json and hands it over.
//
// The store is opened ONCE per config: a test that reopens the same config —
// a restart — keeps the same store, so a profile written before the restart
// is still there after it. That is the whole point of the file.
func withRoster(t *testing.T, cfg kernelruntime.Config) kernelruntime.Config {
	t.Helper()
	if cfg.Roster != nil {
		return cfg
	}
	base := cfg.BaseDir
	if base == "" {
		base = t.TempDir()
	}
	store, err := roster.Open(filepath.Join(base, "roster"))
	if err != nil {
		t.Fatalf("roster.Open: %v", err)
	}
	cfg.Roster = store
	return cfg
}
