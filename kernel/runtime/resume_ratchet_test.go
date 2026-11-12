// SPDX-License-Identifier: MIT

package runtime

// The daemon's TICKET LIFECYCLE door (W3.76ah).
//
// Who owns the durable resume ticket, when it is refreshed and when it is
// deleted is the RUNS MODULE's own (kernel/modules/runs/resume) — the tickets,
// the store and the rules that judge them live there. The daemon held three
// verbs over the operations (ClaimResumeTicket, FinalizeResumeTicket,
// ResumeCheckpointFn) and the run engine — which lives INSIDE that module —
// asked the HOST for them through its port: a round trip inside ONE module,
// three times.
//
// The daemon keeps TWO duties over the tickets and only two: the boot-time
// resumer (which enumerates the suspended tickets and re-dispatches them) and
// the shutdown protocol (which flips the live tickets to "suspended"). Both
// ask the MODULE — the daemon holds no operation of its own.
//
// These tests pin that: the daemon holds no verb per ticket operation, no
// ticket-lifecycle body and no copy of the module's rules, and it asks the
// module for the operations it still needs.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// theTicketVerbsAreNotTheDaemons are the ticket-lifecycle verbs the daemon
// must not hold: the ticket is the run's own durable state, and a verb per
// operation over it is a door the host never had to keep.
var theTicketVerbsAreNotTheDaemons = regexp.MustCompile(`(?m)^func \(k \*Kernel\) (ClaimResumeTicket|FinalizeResumeTicket|ResumeCheckpointFn)\(`)

// theTicketBodiesAreNotTheDaemons are the ticket-lifecycle bodies the daemon
// must not hold: the claim, the capture, the finalization and the checkpoint
// belong to the module's service, which owns the store they work on.
var theTicketBodiesAreNotTheDaemons = regexp.MustCompile(`\b(claimResumeTicket|buildResumeTicket|finalizeResumeTicket|resumeCheckpointFn|resumeActive)\(`)

// theTicketRulesAreNotTheDaemons are the module's rules: the scope rule (a
// root run carries a ticket), the capture (the facts a ticket holds) and the
// keep/delete rule (a shutdown-interrupted run keeps its ticket).
var theTicketRulesAreNotTheDaemons = regexp.MustCompile(`resume\.(CarriesTicket|TerminalOutcome|Build|Facts)\b`)

// TestTheDaemonHoldsNoVerbPerTicketOperation pins the SURFACE: the daemon
// answers for no ticket operation.
func TestTheDaemonHoldsNoVerbPerTicketOperation(t *testing.T) {
	for _, f := range daemonProductionFiles(t) {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if theTicketVerbsAreNotTheDaemons.Match(src) {
			t.Errorf("%s declares a ticket-lifecycle verb — the ticket's lifecycle is the module's (W3.76ah)", f)
		}
	}
}

// TestTheDaemonHoldsNoTicketLifecycleBody pins the BODY: the operations live
// in the module's service, next to the store they work on.
func TestTheDaemonHoldsNoTicketLifecycleBody(t *testing.T) {
	for _, f := range daemonProductionFiles(t) {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if theTicketBodiesAreNotTheDaemons.Match(src) {
			t.Errorf("%s grew a ticket-lifecycle body back — the lifecycle is the module's (W3.76ah)", f)
		}
	}
}

// TestTheDaemonHoldsNoCopyOfTheTicketRules pins the RULES: the scope rule, the
// capture and the keep/delete rule are the module's, and the daemon asks.
func TestTheDaemonHoldsNoCopyOfTheTicketRules(t *testing.T) {
	for _, f := range daemonProductionFiles(t) {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if theTicketRulesAreNotTheDaemons.Match(src) {
			t.Errorf("%s grew a copy of the ticket rules — the rules are the module's (W3.76ah)", f)
		}
	}
}

// TestTheDaemonAsksTheModuleForTheTicketOperations pins the WIRING: the two
// duties the daemon keeps — the boot-time finalization and the shutdown
// transition — go through the module's service, not through a copy of it.
func TestTheDaemonAsksTheModuleForTheTicketOperations(t *testing.T) {
	src, err := os.ReadFile("resume.go")
	if err != nil {
		t.Fatalf("read resume.go: %v", err)
	}
	text := string(src)
	for _, required := range []string{
		"k.tickets.Store()",     // the boot-time resumer enumerates the module's store
		"k.tickets.Finalize(",   // the resumer finalizes a re-dispatched run's ticket
		"k.tickets.SuspendAll(", // the shutdown protocol flips the live tickets
	} {
		if !strings.Contains(text, required) {
			t.Errorf("resume.go must ask the module for %q — the operations are the module's (W3.76ah)", required)
		}
	}
}
