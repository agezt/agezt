// SPDX-License-Identifier: MIT

package slack

// Slack's request door.
//
// The member's body shape is named in the contract
// (kernel/contract/channelhookapi) and measured before the plugin reads it.
// The refusal carries the operation registry's own words:
//
//	operation slack input: $.event.text has wrong type: got number, want string
//
// and the member keeps its own VOICE — the plain text its callers already
// read ("bad json", "bad signature").
//
// The tests below hold the door to four things:
//
//  1. the contract's schema and the plugin's own struct name the SAME fields
//     with the SAME types — the vocabulary has one home, and the member
//     checks it when it is built;
//  2. a mistyped field — and a null — is refused with the registry's words;
//  3. a body that fits the contract gets through untouched;
//  4. a field the contract does not name is still allowed (a body is the
//     stable SUBSET of what the daemon reads).
//
// The bodies here are SIGNED with the member's own scheme (the helper
// postEvent in slack_test.go), so a refusal is the door's answer and not the
// signature check's.

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/channelhookapi"
)

// doorChannel is a member with the dependencies the door's tests need: a
// signing secret.
func doorChannel() *Channel {
	return New(Config{SigningSecret: secret})
}

// TestTheContractAndTheDaemonNameTheSameBody is the ratchet: the contract's
// schema and the plugin's own struct name the same fields, with the same
// types, in both directions.
//
// The member makes the same check when it is built — so a contract that
// mistyped a field stops the daemon, not just this test.
func TestTheContractAndTheDaemonNameTheSameBody(t *testing.T) {
	if err := channelhookapi.CheckShape(channelhookapi.MemberSlack, reflect.TypeOf(slackEnvelope{})); err != nil {
		t.Fatalf("the contract and the plugin disagree: %v", err)
	}
}

// TestTheDoorRefusesAMistypedField: a signed body whose field carries the
// wrong type is refused with the registry's words.
func TestTheDoorRefusesAMistypedField(t *testing.T) {
	c := doorChannel()
	rec := postEvent(t, c, []byte(`{"type":"event_callback","event":{"text":42}}`), false, "")
	got := strings.TrimSpace(rec.Body.String())
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a mistyped field answered %d %s, want 400", rec.Code, got)
	}
	want := "operation slack input: $.event.text has wrong type: got number, want string"
	if !strings.Contains(got, want) {
		t.Errorf("a mistyped field said %q, want it to contain %q", got, want)
	}
}

// TestTheDoorRefusesANullField covers the arm the plugin's own decoder used
// to swallow: a null is not a value, and it is not a default either.
func TestTheDoorRefusesANullField(t *testing.T) {
	c := doorChannel()
	rec := postEvent(t, c, []byte(`{"type":"event_callback","event":{"text":null}}`), false, "")
	got := strings.TrimSpace(rec.Body.String())
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a null field answered %d %s, want 400", rec.Code, got)
	}
	want := "operation slack input: $.event.text has wrong type: got null, want string"
	if !strings.Contains(got, want) {
		t.Errorf("a null field said %q, want it to contain %q", got, want)
	}
}

// TestTheDoorLetsAContractBodyThrough proves the door does not close the
// member: a body that fits the contract is answered the member's own way.
//
// The event is not a plain message, so the member ACKs it and stops there —
// the door test never drives the agent.
func TestTheDoorLetsAContractBodyThrough(t *testing.T) {
	c := doorChannel()
	rec := postEvent(t, c, []byte(`{"type":"event_callback","event":{"type":"message_changed","channel":"C1","user":"U1","text":"hi","ts":"1.0"}}`), false, "")
	got := strings.TrimSpace(rec.Body.String())
	if rec.Code == http.StatusBadRequest {
		t.Fatalf("the member refused a body that fits the contract: %s", got)
	}
	if strings.Contains(got, "has wrong type") {
		t.Errorf("the member refused a body that fits the contract: %s", got)
	}
}

// TestTheDoorAllowsAFieldTheContractDoesNotName keeps the door the registry's
// door: a body is the stable SUBSET of what the daemon reads, so a caller may
// send a field the contract does not declare.
func TestTheDoorAllowsAFieldTheContractDoesNotName(t *testing.T) {
	c := doorChannel()
	rec := postEvent(t, c, []byte(`{"type":"event_callback","event":{"text":"hi"},"unknown":"x"}`), false, "")
	got := strings.TrimSpace(rec.Body.String())
	if rec.Code == http.StatusBadRequest {
		t.Fatalf("the member refused a body carrying an undeclared field: %s", got)
	}
	if strings.Contains(got, "has wrong type") {
		t.Errorf("the member refused a body carrying an undeclared field: %s", got)
	}
}

// TestTheMemberKeepsItsOwnRefusals proves the door did not take the member's
// own answers with it: an unsigned body is still a bad signature, and a body
// that is not JSON at all is still the member's own "bad json".
func TestTheMemberKeepsItsOwnRefusals(t *testing.T) {
	c := doorChannel()

	// An unsigned body: the signature is checked BEFORE the body is read.
	rec := postEvent(t, c, []byte(`{"type":"event_callback","event":{"text":"hi"}}`), true, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("an unsigned body answered %d %s, want 401", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "bad signature") {
		t.Errorf("an unsigned body answered %q, want the member's own \"bad signature\"", rec.Body.String())
	}

	// A body that is not JSON at all: the member's own words, not the
	// registry's shape sentence.
	rec = postEvent(t, c, []byte(`not json`), false, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a body that is not JSON answered %d %s, want 400", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "bad json") {
		t.Errorf("a body that is not JSON answered %q, want the member's own \"bad json\"", rec.Body.String())
	}
}
