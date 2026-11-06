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

// TestTheDaemonKeepsNoResumeKeysOfItsOwn pins the other half of the rule, and
// the ratchet changed honestly in W3.76q.
//
// W3.76l and W3.76m kept the resume handshake's keys in the daemon: they were
// "the daemon's boot protocol, not the run's vocabulary". They were the
// RESUME MODULE's vocabulary all along — the owned marker says which ticket
// kind an outer frame holds, the seed carries the conversation that ticket
// saved — and the run engine (the reader) lives next to that module. So the
// keys moved to kernel/modules/runs/resume, the daemon's two readers went
// with them, and the run engine asks the module now.
//
// The check is over the daemon's WHOLE package: the keys were declared in
// resume.go, and a file-scoped check would have watched one file while the
// vocabulary could hide in another.
func TestTheDaemonKeepsNoResumeKeysOfItsOwn(t *testing.T) {
	for _, name := range []string{"ctxKeyResumeOwned", "ctxKeyResumeSeed", "resumeOwnedKind", "resumeSeedFromCtx"} {
		if hasDaemonDeclaration(t, name) {
			t.Fatalf("the daemon declared %s; the resume handshake's vocabulary is the resume module's", name)
		}
	}
}

// TestTheDaemonAsksTheModuleForTheRunsVocabulary pins the direction of the
// move: the daemon READS the run's dispatch through the module, it does not
// re-implement it.
//
// The check is over the daemon's WHOLE package, not one file: W3.76k retired
// the two forwarding copies the daemon kept (RunTimeoutFromCtx and
// MergeAutoApproveCapabilities), so the readers that use the module's names
// are the ones that drive a run — the run's own path and the resume path —
// and a file-scoped check would have pinned a door that no longer exists.
//
// W3.76l shrank the list again, and honestly: the run context's READ
// vocabulary (the wake fields, the images, the json mode, the cost ceiling,
// the model chain, the agent's daily ceiling, the actor) is the MODULE's, and
// the run engine — which lives inside the module — reads it there. What the
// daemon still reads is the host's own side of the run: the model the run
// falls back to, the actor it stamps, the profile and the ceiling it binds,
// the timeout and the auto-approve merge.
//
// W3.76ae shrank it once more: the run's REGISTRATION moved into the module's
// live-run state, and the auto-approve merge went with it — the daemon no
// longer decorates a run's context, the module does. The daemon keeps the
// timeout read (the resume path asks for the run's own deadline) and the
// model's read.
func TestTheDaemonAsksTheModuleForTheRunsVocabulary(t *testing.T) {
	for _, want := range []string{
		"runctx.ModelFromCtx(",
		"runctx.RunTimeoutFromCtx(",
		"runctx.WithActor(",
		"runctx.WithAgentProfile(",
		"runctx.WithMaxCost(",
	} {
		if !daemonPackageContains(t, want) {
			t.Fatalf("the daemon must read the run's dispatch through %s", want)
		}
	}
	// The auto-approve merge is the MODULE's: the live-run state decorates
	// the run's context, and it reads the run context's vocabulary there.
	state, err := os.ReadFile(filepath.Join("..", "modules", "runs", "state", "state.go"))
	if err != nil {
		t.Fatalf("read the live-run state: %v", err)
	}
	for _, want := range []string{
		"runctx.MergeAutoApproveCapabilities(",
		"runctx.RunTimeoutFromCtx(",
	} {
		if !strings.Contains(string(state), want) {
			t.Errorf("the live-run state must read the run's dispatch through %s", want)
		}
	}
}

// TestTheDaemonKeepsTheDoorsAndNothingMore pins the daemon's HALF of the
// move: the Kernel accessors stay (they are the doors), the package-level
// setters are gone (the hosts name the module directly — one spelling, no
// facade).
//
// W3.76l narrowed the doors: the run context's read vocabulary is the
// module's, and the daemon kept only the doors a MODULE needs without
// naming the runs module (the profile and the ceiling — the self-repair
// module binds a run through them).
func TestTheDaemonKeepsTheDoorsAndNothingMore(t *testing.T) {
	src, err := os.ReadFile("accessors_ctx.go")
	if err != nil {
		t.Fatalf("read accessors_ctx.go: %v", err)
	}
	text := string(src)
	for _, door := range []string{
		"func (k *Kernel) WithAgentProfile(",
		"func (k *Kernel) WithMaxCost(",
	} {
		if !strings.Contains(text, door) {
			t.Fatalf("the daemon must keep the door %q", door)
		}
	}
	// The read vocabulary is the module's: the daemon keeps no forwarding
	// copy of it (W3.76l, W3.76m). The check is over the daemon's WHOLE
	// package — the vocabulary's doors were split over two files, and a
	// file-scoped check would only watch one of them.
	for _, gone := range []string{
		"func (k *Kernel) SystemAgentFromCtx(",
		"func (k *Kernel) ActorFromCtx(",
		"func (k *Kernel) WithActorCorrelation(",
		"func (k *Kernel) AgentDailyMcFromCtx(",
		"func (k *Kernel) ModelChainFromCtx(",
		"func (k *Kernel) WakeContextSource(",
		"func (k *Kernel) ImagesFromCtx(",
		"func (k *Kernel) JSONModeFromCtx(",
		"func (k *Kernel) MaxCostFromCtx(",
		"func (k *Kernel) AgentSlugFromCtx(",
		"func (k *Kernel) AgentRetryPolicyFromCtx(",
	} {
		if daemonPackageContains(t, gone) {
			t.Errorf("the daemon kept %q — the run context's vocabulary is the module's", gone)
		}
	} // The daemon's own package-level setter is gone: a host names the module.
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

// daemonPackageContains reports whether the daemon's own production sources
// hold the given text. It reads the daemon's PRODUCTION files — the test
// files may legitimately mention a name.
func daemonPackageContains(t *testing.T, needle string) bool {
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
		if strings.Contains(string(src), needle) {
			return true
		}
	}
	return false
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
