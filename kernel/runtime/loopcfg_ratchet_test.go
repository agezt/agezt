// SPDX-License-Identifier: MIT

package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The loop configuration — the SHAPE every governed run's loop must have —
// moved to the runs module in W3.76g (kernel/modules/runs/loopcfg).
//
// The daemon kept the STATE the shape is built from: its effective
// configuration, its live tool surface, its tool-phase service, its bus, its
// policy hook, its artifact port and its model catalog. What it must NOT
// keep is a second copy of the rule — its own assembler, its own tool-set
// shaping and its own discovery tool.
//
// And since W3.77 it keeps no DOOR over the rule either: the run engine and
// the delegation engine — both inside the module that owns the rule — hold
// the very same service and ask it directly. The daemon only WIRES it.
//
// These tests are the ratchet.

// TestTheLoopConfigurationLivesInTheRunsModule pins the move itself: the
// daemon's loop-configuration files are GONE, the module's are there.
func TestTheLoopConfigurationLivesInTheRunsModule(t *testing.T) {
	for _, gone := range []string{
		"loopconfig.go", "toolpolicy.go", "toolsearch.go",
	} {
		if _, err := os.Stat(gone); err == nil {
			t.Fatalf("the daemon kept %s; the loop configuration is the module's", gone)
		}
	}
	for _, want := range []string{"doc.go", "config.go", "service.go", "tools.go", "search.go", "elision.go"} {
		if _, err := os.Stat(filepath.Join("..", "modules", "runs", "loopcfg", want)); err != nil {
			t.Fatalf("the runs module must carry %s: %v", want, err)
		}
	}
}

// TestTheDaemonHoldsNoLoopConfigurationRule pins the daemon's side of the
// move: the tool-set shaping rules and the discovery tool are the MODULE'S.
func TestTheDaemonHoldsNoLoopConfigurationRule(t *testing.T) {
	forbidden := []string{
		"func filterTools(",
		"func applyAgentToolPolicy(",
		"func applyAgentNoisePolicyToPromptTools(",
		"func withToolSearch(",
		"func makeElidedSummarizer(",
		"func (k *Kernel) buildLoopConfig(",
	}
	for _, name := range forbidden {
		if hasDaemonDeclaration(t, name) {
			t.Fatalf("the daemon kept the loop-configuration rule %q", name)
		}
	}
}

// TestTheDaemonKeepsTheLoopConfigurationWiring pins the daemon's HALF of the
// move: the WIRING stays — the projection of the daemon's effective
// configuration and the live collaborators the module is built from.
//
// The file was loopcfg_doors.go and it is loopcfg_wiring.go now: the door it
// kept (BuildLoopConfig) left with the module in W3.77, and what is left is
// the state the rule is built from.
func TestTheDaemonKeepsTheLoopConfigurationWiring(t *testing.T) {
	src, err := os.ReadFile("loopcfg_wiring.go")
	if err != nil {
		t.Fatalf("read loopcfg_wiring.go: %v", err)
	}
	text := string(src)
	for _, wire := range []string{
		"func loopcfgConfig(",
		"func (k *Kernel) loopcfgDeps(",
		// The knobs are the daemon's own state, asked per run: the wiring
		// must supply them, or the rule cannot know a run's limits.
		"loopcfg.Config",
	} {
		if !strings.Contains(text, wire) {
			t.Fatalf("the daemon must keep the wiring %q", wire)
		}
	}
}

// TestTheDaemonHoldsNoLoopConfigurationDoor pins the daemon's HALF of W3.77:
// it holds NO verb over the rule and it never calls the rule itself.
//
// The daemon used to keep a door (BuildLoopConfig) and the run engine — which
// lives INSIDE the module that owns the rule — asked the host for it: a round
// trip inside one module. The module's consumers hold the module's service
// now; the daemon only wires it.
func TestTheDaemonHoldsNoLoopConfigurationDoor(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the daemon's directory: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		body := string(b)
		for _, bad := range []string{
			"func (k *Kernel) BuildLoopConfig(",
			// The daemon must not speak for the rule: the module's
			// consumers ask the module, and the daemon wires it.
			"k.loopcfg.Build",
		} {
			if strings.Contains(body, bad) {
				t.Errorf("%s carries %q; the loop configuration is the module's own, and the module's consumers ask the module (W3.77)", name, bad)
			}
		}
	}
}

// TestTheDaemonHoldsTheRuleNotACopy pins the direction: the daemon holds a
// POINTER to the module's service, and the module never imports the daemon.
func TestTheDaemonHoldsTheRuleNotACopy(t *testing.T) {
	if !hasDaemonField(t, "loopcfg *loopcfg.Service") {
		t.Fatal("the daemon must hold the runs module's loop-configuration service")
	}
	dir := filepath.Join("..", "modules", "runs", "loopcfg")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read the loop configuration: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if strings.Contains(string(src), `"github.com/agezt/agezt/kernel/runtime`) {
			t.Fatalf("%s imports the daemon; the rule must not name it", e.Name())
		}
	}
}
