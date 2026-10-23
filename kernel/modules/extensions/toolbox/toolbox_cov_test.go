// SPDX-License-Identifier: MIT

package toolbox

import (
	"context"
	"strings"
	"testing"
)

func TestByName(t *testing.T) {
	// A name that exists in the catalog resolves.
	if len(Catalog) == 0 {
		t.Fatal("catalog is empty; cannot exercise byName")
	}
	want := Catalog[0].Name
	got, ok := byName(want)
	if !ok || got.Name != want {
		t.Fatalf("byName(%q) = %+v, %v; want match", want, got, ok)
	}
	// An unknown name is not found.
	if _, ok := byName("definitely-not-a-real-tool-xyz"); ok {
		t.Error("byName should report false for an unknown tool")
	}
}

func TestTail(t *testing.T) {
	// Short input is returned trimmed and whole.
	if got := tail("  hello  ", 100); got != "hello" {
		t.Errorf("tail short = %q, want %q", got, "hello")
	}
	// Long input keeps the last n bytes and is prefixed with an ellipsis.
	long := strings.Repeat("a", 50) + "TAILEND"
	got := tail(long, 5)
	if !strings.HasPrefix(got, "…") {
		t.Errorf("tail long should start with ellipsis, got %q", got)
	}
	if !strings.HasSuffix(got, "ILEND") {
		t.Errorf("tail long should keep the trailing bytes, got %q", got)
	}
}

func TestFirstLineVariants(t *testing.T) {
	// Empty input yields empty output.
	if got := firstLine(""); got != "" {
		t.Errorf("firstLine(empty) = %q, want empty", got)
	}
	// Leading blank lines are skipped; the first non-blank line wins.
	if got := firstLine("\n\n  v1.2.3  \nsecond"); got != "v1.2.3" {
		t.Errorf("firstLine multiline = %q, want %q", got, "v1.2.3")
	}
	// All-whitespace input falls through to the trimmed-tail branch.
	if got := firstLine("   \n  \n"); got != "" {
		t.Errorf("firstLine(whitespace) = %q, want empty", got)
	}
}

func TestProbeVersion(t *testing.T) {
	ctx := context.Background()
	// A binary that does not exist returns "" without error.
	if got := probeVersion(ctx, "this-binary-does-not-exist-xyz", []string{"--version"}); got != "" {
		t.Errorf("probeVersion(missing) = %q, want empty", got)
	}
	// `go version` is reliably present in the test toolchain and prints one line.
	if got := probeVersion(ctx, "go", []string{"version"}); !strings.Contains(got, "go") {
		t.Errorf("probeVersion(go version) = %q, want a line containing 'go'", got)
	}
}

func TestInstall_UnknownTool(t *testing.T) {
	res := Install(context.Background(), "definitely-not-a-real-tool-xyz")
	if !res.Skipped || res.Error != "unknown tool" {
		t.Fatalf("Install(unknown) = %+v; want Skipped with 'unknown tool'", res)
	}
}

func TestInstall_NoRecipeForHost(t *testing.T) {
	// Register a synthetic tool with no recipe for any OS/manager so Install
	// takes the "no install recipe for this host" branch without running a
	// real package manager. Restore the catalog afterwards.
	orig := Catalog
	defer func() { Catalog = orig }()
	Catalog = append(append([]Tool(nil), orig...), Tool{
		Name:     "cov-norecipe-tool",
		Category: "test",
		Recipes:  map[string][]Recipe{},
	})
	res := Install(context.Background(), "cov-norecipe-tool")
	if !res.Skipped || res.Error != "no install recipe for this host" {
		t.Fatalf("Install(no-recipe) = %+v; want Skipped with 'no install recipe for this host'", res)
	}
}

func TestOutdated_Runs(t *testing.T) {
	// Outdated is best-effort: it must always return a non-nil map and never
	// panic, regardless of which managers happen to be present on the host.
	got := Outdated(context.Background())
	if got == nil {
		t.Fatal("Outdated returned a nil map; want an initialized map")
	}
}

func TestManagerProbesAllOS(t *testing.T) {
	// Exercise every OS branch so managerProbes is fully covered regardless of
	// the host GOOS. Each branch must always include the language managers.
	for _, goos := range []string{"windows", "darwin", "linux"} {
		probes := managerProbes(goos)
		for _, want := range []string{"pip", "npm", "cargo", "go"} {
			if _, ok := probes[want]; !ok {
				t.Errorf("managerProbes(%q) missing language manager %q", goos, want)
			}
		}
	}
	// Spot-check OS-specific probes.
	if _, ok := managerProbes("windows")["winget"]; !ok {
		t.Error("windows probes should include winget")
	}
	if _, ok := managerProbes("darwin")["brew"]; !ok {
		t.Error("darwin probes should include brew")
	}
	if _, ok := managerProbes("linux")["apt-get"]; !ok {
		t.Error("linux probes should include apt-get")
	}
}
