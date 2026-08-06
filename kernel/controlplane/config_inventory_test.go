// SPDX-License-Identifier: MIT

package controlplane

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// TestConfigEnvVars_CoversCmdAgeztReads enforces the configEnvVars invariant
// (M127): the comment promises the list is "every Getenv(\"AGEZT_...\") the
// daemon reads", so `agt config show` reports the presence of every var. That
// promise had silently rotted — dozens of vars added over many milestones were
// never added here. This test makes the invariant self-enforcing: it scans the
// daemon's env-reading packages for AGEZT_* reads and fails if any is absent
// from the list, so the next omission is caught at test time, not by a
// confused operator.
//
// The scan roots cover cmd/agezt PLUS the plugin packages the Phase 2
// extractions moved env-reading boot code into (2.1 builtinchannels,
// 2.2 builtintools, 2.4 providerboot) — the extractions had silently moved
// reads out of the old cmd/agezt-only scan's sight. A missing root is skipped
// (partial checkouts), but ALL roots missing fails loudly: the guard would
// otherwise be vacuously green.
func TestConfigEnvVars_CoversCmdAgeztReads(t *testing.T) {
	// Locate the repo root relative to this test file (robust to the cwd).
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("runtime.Caller unavailable")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	roots := []string{
		filepath.Join(repoRoot, "cmd", "agezt"),
		filepath.Join(repoRoot, "plugins", "providerboot"),
		filepath.Join(repoRoot, "plugins", "builtinchannels"),
		filepath.Join(repoRoot, "plugins", "builtintools"),
	}

	listed := make(map[string]bool, len(configEnvVars))
	for _, v := range configEnvVars {
		listed[v] = true
	}

	// Two canonical read forms: `brand.EnvPrefix + "NAME"` and a bare
	// os.Getenv/os.LookupEnv("AGEZT_NAME"). Restricting to these precise forms
	// (not any "AGEZT_…" literal) avoids false positives from banner/help strings
	// that merely mention an env var without reading it. Deps-style getters
	// (`d.get(brand.EnvPrefix + "NAME")` in providerboot) match the first form.
	rePrefix := regexp.MustCompile(`EnvPrefix\s*\+\s*"([A-Z0-9_]+)"`)
	reGetenv := regexp.MustCompile(`os\.(?:Getenv|LookupEnv)\("(AGEZT_[A-Z0-9_]+)"`)

	missing := map[string]bool{}
	scanned := 0
	for _, dir := range roots {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Logf("scan root %s not found (%v) — skipping this root", dir, err)
			continue
		}
		scanned++
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatalf("read %s: %v", e.Name(), err)
			}
			src := string(data)
			for _, m := range rePrefix.FindAllStringSubmatch(src, -1) {
				if name := "AGEZT_" + m[1]; !listed[name] {
					missing[name] = true
				}
			}
			for _, m := range reGetenv.FindAllStringSubmatch(src, -1) {
				if !listed[m[1]] {
					missing[m[1]] = true
				}
			}
		}
	}
	if scanned == 0 {
		t.Fatal("none of the env-read scan roots exist — the inventory guard cannot run")
	}

	if len(missing) > 0 {
		names := make([]string, 0, len(missing))
		for n := range missing {
			names = append(names, n)
		}
		sort.Strings(names)
		t.Errorf("configEnvVars is missing %d env var(s) read in the daemon's env-reading packages: %v\n"+
			"add them to configEnvVars so `agt config show` reports their presence", len(names), names)
	}
}
