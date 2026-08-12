// SPDX-License-Identifier: MIT

package controlplane

import (
	"fmt"
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
// The scan is an EXCLUSION list, not an inclusion list, and that is the whole
// point. It used to name eight directories explicitly, each added reactively
// after a refactor moved env-reading code somewhere new ("Phase 2.5: …",
// "Phase 2.6: …"). Every such move shrank the guard's coverage until a human
// noticed, and by 2026-08-12 thirteen vars — including AGEZT_AGENTGW_TOKEN_SECRET
// and AGEZT_AWS_CREDENTIAL_PROCESS_ALLOWED — were being read in packages the
// guard could not see. Walking everything and naming the exceptions inverts the
// default: a new package is COVERED until someone consciously excludes it, so
// the next extraction cannot silently narrow the check.
func TestConfigEnvVars_CoversCmdAgeztReads(t *testing.T) {
	// Locate the repo root relative to this test file (robust to the cwd).
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("runtime.Caller unavailable")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	// Everything the DAEMON links. cmd/agt is deliberately absent: it is the
	// CLI, and its own vars (AGEZT_SHOW_REASONING and friends) are not daemon
	// configuration, so `agt config show` has no business reporting them.
	roots := []string{
		filepath.Join(repoRoot, "kernel"),
		filepath.Join(repoRoot, "plugins"),
		filepath.Join(repoRoot, "internal"),
		filepath.Join(repoRoot, "cmd", "agezt"),
	}
	// Directories whose AGEZT_* reads are NOT daemon configuration. Keep this
	// list short and justified — every entry is a hole in the guard.
	excluded := map[string]bool{
		// Non-_test.go files that exist only to serve tests, so their env
		// reads are fixtures rather than operator-facing configuration.
		"kernel/internal/testfixtures": true,
	}

	listed := make(map[string]bool, len(configEnvVars))
	for _, v := range configEnvVars {
		listed[v] = true
	}

	// Two forms, and the second is deliberately broad.
	//
	// `brand.EnvPrefix + "NAME"` produces no "AGEZT_NAME" literal at all, so it
	// needs its own pattern; Deps-style getters (`d.Get(brand.EnvPrefix+"NAME")`)
	// match it too.
	//
	// The second used to be a narrow `os.Getenv/os.LookupEnv("AGEZT_NAME")`, on
	// the theory that matching any literal would false-positive on banner and
	// help strings that merely MENTION a var. In practice the narrowness cost far
	// more than it saved: it missed `const TokenSecretEnv = "AGEZT_AGENTGW_TOKEN_SECRET"`
	// (a name bound to a constant, read elsewhere) and `envLookup(lookup, "AGEZT_…")`
	// (an indirection helper) — so a security-relevant var and the whole provider
	// `compat` surface stayed invisible. Matching ANY AGEZT_ literal was measured
	// against the tree on 2026-08-12: 28 hits, 28 of them real env vars, zero
	// mention-only strings. A false positive here is also cheap and visible — the
	// var gets listed, or its package gets an excluded[] entry with a reason —
	// whereas a false negative is invisible until an operator hits it.
	rePrefix := regexp.MustCompile(`EnvPrefix\s*\+\s*"([A-Z0-9_]+)"`)
	reGetenv := regexp.MustCompile(`"(AGEZT_[A-Z0-9_]+)"`)

	missing := map[string]bool{}
	where := map[string]string{}
	scannedRoots, scannedFiles := 0, 0
	for _, root := range roots {
		if _, err := os.Stat(root); err != nil {
			t.Logf("scan root %s not found (%v) — skipping this root", root, err)
			continue
		}
		scannedRoots++
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel := filepath.ToSlash(mustRel(t, repoRoot, path))
			if d.IsDir() {
				if excluded[rel] {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			scannedFiles++
			src := string(data)
			note := func(name string) {
				if listed[name] {
					return
				}
				missing[name] = true
				if where[name] == "" {
					where[name] = rel
				}
			}
			for _, m := range rePrefix.FindAllStringSubmatch(src, -1) {
				note("AGEZT_" + m[1])
			}
			for _, m := range reGetenv.FindAllStringSubmatch(src, -1) {
				note(m[1])
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	if scannedRoots == 0 {
		t.Fatal("none of the env-read scan roots exist — the inventory guard cannot run")
	}
	// A walk that silently matched nothing would be vacuously green, which is
	// exactly the failure this rewrite exists to prevent.
	if scannedFiles == 0 {
		t.Fatal("the env-read scan visited no Go files — the guard is vacuous")
	}

	if len(missing) > 0 {
		names := make([]string, 0, len(missing))
		for n := range missing {
			names = append(names, n)
		}
		sort.Strings(names)
		var b strings.Builder
		for _, n := range names {
			fmt.Fprintf(&b, "\n  %s (%s)", n, where[n])
		}
		t.Errorf("configEnvVars is missing %d env var(s) the daemon reads:%s\n"+
			"add them to configEnvVars (alphabetically) so `agt config show` reports their presence, "+
			"or exclude the package above if the read is not operator-facing configuration", len(names), b.String())
	}
}

// mustRel makes a repo-relative path for error messages and exclusion matching.
func mustRel(t *testing.T, base, path string) string {
	t.Helper()
	rel, err := filepath.Rel(base, path)
	if err != nil {
		t.Fatalf("filepath.Rel(%s, %s): %v", base, path, err)
	}
	return rel
}
