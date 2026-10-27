// SPDX-License-Identifier: MIT

package agentgw

// Provenance: The gateway's own contract with the daemon it serves (W3.67) —
//             the two places where the transport used to lose what it was
//             asked to carry: the audit record (dropped, because its actor
//             was empty) and the memory's type vocabulary (refused, because
//             the transport did not canonicalise the spelling).

import (
	"net/http/httptest"
	"testing"

	"github.com/agezt/agezt/kernel/journal"
)

// TestTheAuditRecordIsChargedToTheTokenThatMadeTheRequest: the audit trail
// answers "who used which capability", so its actor must name a token.
//
// A SUBPROCESS token is charged to its parent; a TOP-LEVEL token — the kind
// `agt token create` mints — has no parent and is charged to itself. The
// empty case is the one that matters: an event without an actor is REJECTED
// by the journal, so an empty actor does not record an anonymous access, it
// records nothing at all.
func TestTheAuditRecordIsChargedToTheTokenThatMadeTheRequest(t *testing.T) {
	cases := []struct {
		name   string
		claims *TokenClaims
		want   string
	}{
		{
			name:   "a subprocess token is charged to its parent",
			claims: &TokenClaims{TokenID: "tok-child", ParentTokenID: "tok-parent"},
			want:   "tok-parent",
		},
		{
			name:   "a top-level token is charged to itself",
			claims: &TokenClaims{TokenID: "tok-top"},
			want:   "tok-top",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := auditActor(tc.claims)
			if got != tc.want {
				t.Fatalf("the audit actor = %q, want %q", got, tc.want)
			}
			if got == "" {
				t.Fatal("the audit actor must never be empty — an event without an actor is rejected by the journal, so the access would not be recorded at all")
			}
		})
	}
}

// TestTheAuditTrailReachesTheJournalForATopLevelToken: the whole point of
// wiring a journal into the gateway is that the trail LANDS. A top-level
// token is the only kind the `agt` CLI mints, so a trail that dropped those
// records was an empty trail.
func TestTheAuditTrailReachesTheJournalForATopLevelToken(t *testing.T) {
	dir := t.TempDir()
	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	defer j.Close()

	g := NewGateway(GatewayConfig{TokenSecret: []byte("test-secret-key-32-chars-minimum!!")})
	g.SetAuditJournal(j)

	req := httptest.NewRequest("GET", "/v1/memory/search?q=x", nil)
	g.auditAccess(req, &TokenClaims{
		RunID:   "run-audit",
		TokenID: "tok-top",
		Caps:    []string{string(CapMemorySearch)},
	})
	// The trail is buffered and flushed on Close.
	if err := g.Close(); err != nil {
		t.Fatalf("close the gateway: %v", err)
	}

	events, err := j.Tail(20)
	if err != nil {
		t.Fatalf("read the journal: %v", err)
	}
	for _, ev := range events {
		if ev.Subject != "agentgw.audit" {
			continue
		}
		if ev.Actor != "tok-top" {
			t.Fatalf("the audit record's actor = %q, want the token that made the request", ev.Actor)
		}
		return
	}
	t.Fatal("the audit record never reached the journal")
}

// TestTheMemoryTypeIsCanonicalised: the contract's types are upper-case and
// the memory validates them strictly, so the transport accepts the spelling
// the type's NAME uses and canonicalises it. A client that wrote "fact" was
// refused with a 500 for a type that exists.
func TestTheMemoryTypeIsCanonicalised(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{raw: "fact", want: "FACT"},
		{raw: "FACT", want: "FACT"},
		{raw: "Summary", want: "SUMMARY"},
		{raw: "  observation  ", want: "OBSERVATION"},
		{raw: "", want: "FACT"},
		{raw: "   ", want: "FACT"},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			got := string(memoryRecordType(tc.raw))
			if got != tc.want {
				t.Fatalf("the canonical type of %q = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestTheGatewayRecordsTheAccessItAuthorised: the audit entry names the
// operation and the path it let through — the two facts an operator reads
// when asking "what did this token do?".
func TestTheGatewayRecordsTheAccessItAuthorised(t *testing.T) {
	dir := t.TempDir()
	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	defer j.Close()

	g := NewGateway(GatewayConfig{TokenSecret: []byte("test-secret-key-32-chars-minimum!!")})
	g.SetAuditJournal(j)

	req := httptest.NewRequest("POST", "/v1/memory/write", nil)
	g.auditAccess(req, &TokenClaims{
		RunID:        "run-audit",
		TokenID:      "tok-top",
		SubprocessID: "sub-1",
		Caps:         []string{string(CapMemoryWrite)},
	})
	if err := g.Close(); err != nil {
		t.Fatalf("close the gateway: %v", err)
	}

	events, err := j.Tail(20)
	if err != nil {
		t.Fatalf("read the journal: %v", err)
	}
	for _, ev := range events {
		if ev.Subject != "agentgw.audit" {
			continue
		}
		if ev.CorrelationID != "run-audit" {
			t.Fatalf("the audit record's run = %q, want the run that made the request", ev.CorrelationID)
		}
		if len(ev.Payload) == 0 {
			t.Fatal("the audit record carries no payload — the operation and the path must be readable")
		}
		return
	}
	t.Fatal("the audit record never reached the journal")
}
