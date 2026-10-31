// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
	"github.com/agezt/agezt/kernel/modules/runs/resume"
)

// The durable run tickets moved to the runs module in W3.76a. These tests pin
// the daemon's half of the move: the daemon ASKS the module's rules, and it
// does not grow its own copy of them.

// TestTheTicketRulesLiveInTheRunsModule is the ratchet: the daemon's resume
// hook must not creep the ticket rules back. The scope rule, the resumability
// rule and the keep/delete rule are the module's (kernel/modules/runs/resume);
// the daemon keeps the doors — it reads the run's context, hands over the
// facts, and publishes the events.
func TestTheTicketRulesLiveInTheRunsModule(t *testing.T) {
	src, err := os.ReadFile("resume.go")
	if err != nil {
		t.Fatalf("read resume.go: %v", err)
	}
	text := string(src)
	for _, required := range []string{
		"resume.CarriesTicket(",
		"resume.Build(",
		"resume.TerminalOutcome(",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("resume.go must ask the module's rule %q", required)
		}
	}
	for _, forbidden := range []string{
		`ParentCorrelation == ""`,
		"errors.Is(",
		"Resumable = false",
		"Resumable:    true",
		"StatusActive",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("resume.go grew the ticket rule %q back; the rules are the module's", forbidden)
		}
	}
}

// TestTheScopeRuleIsTheModulesRule pins the daemon's use of the module's scope
// rule: a ROOT run carries a ticket, a delegated run does not.
func TestTheScopeRuleIsTheModulesRule(t *testing.T) {
	store, err := resume.Open(filepath.Join(t.TempDir(), "resume"), 0)
	if err != nil {
		t.Fatalf("resume.Open: %v", err)
	}
	k := &Kernel{cfg: Config{ResumeEnabled: true}, resume: store}

	root := context.Background()
	if !k.resumeActive(root) {
		t.Fatal("a root run must carry a ticket")
	}
	delegated := WithWakeContext(root, WakeContext{ParentCorrelation: "corr-parent"})
	if k.resumeActive(delegated) {
		t.Fatal("a delegated run must not carry its own ticket")
	}
}

// TestTheScopeRuleNeedsTheFeatureAndTheStore pins the other two doors: resume
// off, or no store, means no ticket.
func TestTheScopeRuleNeedsTheFeatureAndTheStore(t *testing.T) {
	store, err := resume.Open(filepath.Join(t.TempDir(), "resume"), 0)
	if err != nil {
		t.Fatalf("resume.Open: %v", err)
	}
	if (&Kernel{cfg: Config{ResumeEnabled: false}, resume: store}).resumeActive(context.Background()) {
		t.Fatal("resume disabled must mean no ticket")
	}
	if (&Kernel{cfg: Config{ResumeEnabled: true}}).resumeActive(context.Background()) {
		t.Fatal("no store must mean no ticket")
	}
}

// TestTheTicketCapturesTheGovernanceCeilings pins the daemon's half of the
// capture: the daemon READS the run's context and hands the facts over, and
// the module turns them into the ticket. A tightened trust ceiling, a cost cap
// and a per-run timeout must all survive into the ticket — resume must never
// silently regain authority or guess a cap.
func TestTheTicketCapturesTheGovernanceCeilings(t *testing.T) {
	profile := roster.Profile{Slug: "ops", Soul: "ops soul", Model: "ops-model", TrustCeiling: "L2"}
	ctx := WithAgentProfile(context.Background(), profile)
	ctx = WithMaxCost(ctx, 321)
	ctx = WithRunTimeout(ctx, 2*time.Second)

	ticket := (&Kernel{}).buildResumeTicket(ctx, "corr-1", "task", resume.KindRun, 0)
	if ticket.TrustCeiling == nil || *ticket.TrustCeiling != int(edict.LevelAskFirst) {
		t.Fatalf("trust ceiling lost: %+v", ticket.TrustCeiling)
	}
	if ticket.MaxCostMc != 321 {
		t.Fatalf("cost cap lost: %d", ticket.MaxCostMc)
	}
	if ticket.RunTimeoutMs != 2000 {
		t.Fatalf("run timeout lost: %d", ticket.RunTimeoutMs)
	}
	if ticket.AgentSlug != "ops" {
		t.Fatalf("agent slug lost: %q", ticket.AgentSlug)
	}
}

// TestTheKeepDeleteRuleIsTheModulesRule pins the daemon's use of the module's
// termination rule: a run interrupted by a shutdown keeps its ticket, every
// other terminal outcome deletes it.
func TestTheKeepDeleteRuleIsTheModulesRule(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "resume")
	store, err := resume.Open(dir, 0)
	if err != nil {
		t.Fatalf("resume.Open: %v", err)
	}
	k := &Kernel{cfg: Config{ResumeEnabled: true}, resume: store}

	if err := store.Put(&resume.Ticket{Corr: "corr-1", Intent: "task", Kind: resume.KindRun}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	k.finalizeResumeTicket("corr-1", context.Canceled)
	if _, ok, _ := store.Get("corr-1"); ok {
		t.Fatal("an ordinary cancellation deletes the ticket")
	}

	if err := store.Put(&resume.Ticket{Corr: "corr-2", Intent: "task", Kind: resume.KindRun}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	k.suspending.Store(true)
	k.finalizeResumeTicket("corr-2", context.Canceled)
	if _, ok, _ := store.Get("corr-2"); !ok {
		t.Fatal("a run interrupted by a shutdown KEEPS its ticket")
	}
}
