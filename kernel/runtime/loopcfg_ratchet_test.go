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
// The daemon keeps the STATE the shape is built from: its effective
// configuration, its live tool surface, its tool-phase service, its bus, its
// policy hook, its artifact port and its model catalog. What it must NOT
// keep is a second copy of the rule — its own assembler, its own tool-set
// shaping and its own discovery tool.
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

// TestTheDaemonKeepsTheLoopConfigurationDoors pins the daemon's HALF of the
// move: the door file stays — it is the mechanism the module asks the daemon
// for.
func TestTheDaemonKeepsTheLoopConfigurationDoors(t *testing.T) {
	src, err := os.ReadFile("loopcfg_doors.go")
	if err != nil {
		t.Fatalf("read loopcfg_doors.go: %v", err)
	}
	text := string(src)
	for _, door := range []string{
		"func loopcfgConfig(",
		"func (k *Kernel) loopcfgDeps(",
		"func (k *Kernel) BuildLoopConfig(",
	} {
		if !strings.Contains(text, door) {
			t.Fatalf("the daemon must keep the door %q", door)
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
