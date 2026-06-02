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
// (M127): the comment promises the list is "every Getenv(\"AGEZT_...\") in
// cmd/agezt/", so `agt config show` reports the presence of every var the daemon
// reads. That promise had silently rotted — dozens of vars added over many
// milestones were never added here. This test makes the invariant self-enforcing:
// it scans cmd/agezt for AGEZT_* reads and fails if any is absent from the list,
// so the next omission is caught at test time, not by a confused operator.
func TestConfigEnvVars_CoversCmdAgeztReads(t *testing.T) {
	// Locate cmd/agezt relative to this test file (robust to the cwd).
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("runtime.Caller unavailable")
	}
	dir := filepath.Join(filepath.Dir(thisFile), "..", "..", "cmd", "agezt")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("cmd/agezt not found (%v) — skipping inventory guard", err)
	}

	listed := make(map[string]bool, len(configEnvVars))
	for _, v := range configEnvVars {
		listed[v] = true
	}

	// Two canonical read forms: `brand.EnvPrefix + "NAME"` and a bare
	// os.Getenv/os.LookupEnv("AGEZT_NAME"). Restricting to these precise forms
	// (not any "AGEZT_…" literal) avoids false positives from banner/help strings
	// that merely mention an env var without reading it.
	rePrefix := regexp.MustCompile(`EnvPrefix\s*\+\s*"([A-Z0-9_]+)"`)
	reGetenv := regexp.MustCompile(`os\.(?:Getenv|LookupEnv)\("(AGEZT_[A-Z0-9_]+)"`)

	missing := map[string]bool{}
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

	if len(missing) > 0 {
		names := make([]string, 0, len(missing))
		for n := range missing {
			names = append(names, n)
		}
		sort.Strings(names)
		t.Errorf("configEnvVars is missing %d env var(s) read in cmd/agezt: %v\n"+
			"add them to configEnvVars so `agt config show` reports their presence", len(names), names)
	}
}
