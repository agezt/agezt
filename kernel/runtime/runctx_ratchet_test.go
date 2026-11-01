// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
)

// The run context — the vocabulary a run carries — moved to the runs module in
// W3.76c. The daemon keeps the DOORS (the Kernel accessors, the resume
// handshake, the prompt-injection mode name) and nothing else: the keys, the
// setters, the getters and the run's own types are the module's.
//
// These tests are the ratchet: the daemon must not grow a second copy of the
// vocabulary, and the module must actually hold it.

// TestTheRunContextVocabularyLivesInTheRunsModule pins the move itself: the
// daemon's five runctx files and its prompt renderer are GONE, and the module
// carries the keys and the setters.
func TestTheRunContextVocabularyLivesInTheRunsModule(t *testing.T) {
	for _, gone := range []string{
		"runctx.go",
		"runctx_agent.go",
		"runctx_basic.go",
		"runctx_security.go",
		"runctx_wake.go",
		"prompt.go",
	} {
		if _, err := os.Stat(filepath.Join(".", gone)); err == nil {
			t.Fatalf("the daemon kept %s; the run context is the module's", gone)
		}
	}
	module := "../modules/runs/runctx"
	for _, want := range []string{
		"keys.go",
		"agent.go",
		"basic.go",
		"security.go",
		"wake.go",
	} {
		if _, err := os.Stat(filepath.Join(module, want)); err != nil {
			t.Fatalf("the runs module must carry %s: %v", want, err)
		}
	}
}

// TestTheDaemonHoldsNoRunContextKey is the ratchet on the daemon's side: the
// daemon must not define a context key for the run's dispatch. The keys it
// legitimately owns are its OWN resume handshake (ctxKeyResumeOwned /
// ctxKeyResumeSeed), which the module has no business knowing.
func TestTheDaemonHoldsNoRunContextKey(t *testing.T) {
	forbidden := []string{
		"ctxKeyModel",
		"ctxKeySystem",
		"ctxKeyImages",
		"ctxKeyJSONMode",
		"ctxKeyTrustCeiling",
		"ctxKeyRoot",
		"ctxKeyModelChain",
		"ctxKeyAgentIdent",
		"ctxKeySystemAgent",
		"ctxKeyAgentLifecycle",
		"ctxKeyAgentRetryPolicy",
		"ctxKeyAgentToolPolicy",
		"ctxKeyAgentConfigOverrides",
		"ctxKeyAgentNoisePolicy",
		"ctxKeyAutoApproveCaps",
		"ctxKeyTrustedObservations",
		"ctxKeyActor",
		"ctxKeyCorrelation",
		"ctxKeyRunTimeout",
		"ctxKeyMaxCost",
	}
	for _, name := range forbidden {
		if hasDaemonDeclaration(t, name) {
			t.Fatalf("the daemon declared %s; the run context's keys are the module's", name)
		}
	}
}

// TestTheDaemonKeepsItsOwnResumeKeys pins the other half of the rule: the
// daemon's resume handshake keys ARE the daemon's and must stay — they are the
// boot protocol, not the run's vocabulary.
func TestTheDaemonKeepsItsOwnResumeKeys(t *testing.T) {
	src, err := os.ReadFile("resume.go")
	if err != nil {
		t.Fatalf("read resume.go: %v", err)
	}
	text := string(src)
	for _, want := range []string{"ctxKeyResumeOwned", "ctxKeyResumeSeed"} {
		if !strings.Contains(text, want) {
			t.Fatalf("the daemon must keep its own %s", want)
		}
	}
}

// TestTheDaemonAsksTheModuleForTheRunsVocabulary pins the direction of the
// move: the daemon READS the run's dispatch through the module, it does not
// re-implement it.
func TestTheDaemonAsksTheModuleForTheRunsVocabulary(t *testing.T) {
	src, err := os.ReadFile("accessors_ctx.go")
	if err != nil {
		t.Fatalf("read accessors_ctx.go: %v", err)
	}
	text := string(src)
	for _, want := range []string{
		"runctx.SystemAgentFromCtx(",
		"runctx.ModelFromCtx(",
		"runctx.AgentDailyMcFromCtx(",
		"runctx.ModelChainFromCtx(",
		"runctx.WakeContextFromCtx(",
		"runctx.ImagesFromCtx(",
		"runctx.JSONModeFromCtx(",
		"runctx.MaxCostFromCtx(",
		"runctx.RunTimeoutFromCtx(",
		"runctx.MergeAutoApproveCapabilities(",
		"runctx.WithActor(",
		"runctx.ActorFromCtx(",
		"runctx.WithAgentProfile(",
		"runctx.WithMaxCost(",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("accessors_ctx.go must read the run's dispatch through %s", want)
		}
	}
}

// TestTheDaemonKeepsTheDoorsAndNothingMore pins the daemon's HALF of the
// move: the Kernel accessors stay (they are the doors), the package-level
// setters are gone (the hosts name the module directly — one spelling, no
// facade).
func TestTheDaemonKeepsTheDoorsAndNothingMore(t *testing.T) {
	src, err := os.ReadFile("accessors_ctx.go")
	if err != nil {
		t.Fatalf("read accessors_ctx.go: %v", err)
	}
	text := string(src)
	for _, door := range []string{
		"func (k *Kernel) SystemAgentFromCtx(",
		"func (k *Kernel) ActorFromCtx(",
		"func (k *Kernel) WithActorCorrelation(",
		"func (k *Kernel) WithAgentProfile(",
		"func (k *Kernel) WithMaxCost(",
	} {
		if !strings.Contains(text, door) {
			t.Fatalf("the daemon must keep the door %q", door)
		}
	}
	// The daemon's own package-level setter is gone: a host names the module.
	if hasDaemonDeclaration(t, "WithAgentProfile") || hasDaemonDeclaration(t, "WithMaxCost") {
		t.Fatal("the daemon kept a package-level run-context setter; the hosts name the module")
	}
}

// TestTheModuleCarriesTheRunsOwnTypes pins that the run's OWN types travelled
// with the vocabulary: the tool policy, the noise policy and the resolved
// string setting are the module's rows now.
func TestTheModuleCarriesTheRunsOwnTypes(t *testing.T) {
	var pol runctx.AgentToolPolicy
	pol.Allow = []string{"shell"}
	if len(pol.Allow) != 1 {
		t.Fatal("the module must carry the run's tool policy row")
	}
	var noise runctx.AgentNoisePolicy
	noise.MinNotifySeverity = "warning"
	if noise.MinNotifySeverity != "warning" {
		t.Fatal("the module must carry the run's noise policy row")
	}
	var setting runctx.RunStringSetting
	setting.Value = "v"
	setting.ProfileSlug = "p"
	if setting.Value != "v" || setting.ProfileSlug != "p" {
		t.Fatal("the module must carry the run's resolved setting row")
	}
}

// TestTheProfileDoorBindsTheRun pins the daemon's door: a host that holds a
// kernel handle binds a profile through the door, and the run carries it. The
// door is the daemon's; the vocabulary behind it is the module's.
func TestTheProfileDoorBindsTheRun(t *testing.T) {
	k := &Kernel{}
	ctx := k.WithAgentProfile(context.Background(), agentapi.Profile{
		Slug:            "ops",
		ConfigOverrides: map[string]string{"AGEZT_MODEL": "agent-model"},
	})
	if got := runctx.AgentConfigOverrides(ctx); got["AGEZT_MODEL"] != "agent-model" {
		t.Fatalf("the profile door bound nothing: %+v", got)
	}
	ctx = k.WithMaxCost(ctx, 321)
	if got := runctx.MaxCostFromCtx(ctx); got != 321 {
		t.Fatalf("the cost door bound nothing: %d", got)
	}
}

// hasDaemonDeclaration reports whether the daemon's own package declares the
// given identifier (a type, a const or a func). It reads the daemon's
// production sources — the test files may legitimately mention a name.
func hasDaemonDeclaration(t *testing.T, name string) bool {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the daemon package: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, line := range strings.Split(string(src), "\n") {
			line = strings.TrimSpace(line)
			for _, prefix := range []string{"func ", "type ", "const ", "var "} {
				if !strings.HasPrefix(line, prefix) {
					continue
				}
				rest := strings.TrimSpace(strings.TrimPrefix(line, prefix))
				if strings.HasPrefix(rest, "(") {
					// A method: look past the receiver.
					if idx := strings.Index(rest, ")"); idx >= 0 {
						rest = strings.TrimSpace(rest[idx+1:])
					}
				}
				if rest == name || strings.HasPrefix(rest, name+" ") || strings.HasPrefix(rest, name+"\t") {
					return true
				}
			}
		}
	}
	return false
}
