// SPDX-License-Identifier: MIT

package imessage

// The iMessage member's request door.
//
// The member's body shape is named in the contract
// (kernel/contract/channelhookapi) and measured before the plugin reads it.
// The refusal carries the operation registry's own words:
//
//	operation imessage input: $.data.text has wrong type: got number, want string
//
// and the member keeps its own VOICE — the plain text its callers already
// read ("forbidden", "bad body") plus one new one: "bad json", the answer to
// a body that is not JSON at all. That body used to be answered 200 and
// swallowed; it is now refused before the member writes anything.
//
// The tests below hold the door to five things:
//
//  1. the contract's schema and the plugin's own struct name the SAME fields
//     with the SAME types — the vocabulary has one home, and the member
//     checks it when it is built;
//  2. a mistyped field is refused with the registry's words;
//  3. a null is refused too — a null is not a value, and not a default either;
//  4. a body that fits the contract gets through untouched;
//  5. a field the contract does not name is allowed — a body is the stable
//     SUBSET of what the daemon reads, and BlueBubbles carries fields Agezt
//     never looks at.

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/channelhookapi"
)

// doorSecret is the shared secret the member verifies inbound against.
const doorSecret = "s3cr3t"

// doorChannel is a member with the dependency the door's tests need: the
// signing secret.
func doorChannel(t *testing.T) *Channel {
	t.Helper()
	return New(Config{Secret: doorSecret})
}

// doorPost sends a body to the member's own inbound route, signed the way
// BlueBubbles signs it: the shared secret in a header.
func doorPost(t *testing.T, c *Channel, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, DefaultPath, strings.NewReader(body))
	req.Header.Set("X-Webhook-Secret", doorSecret)
	rec := httptest.NewRecorder()
	c.Handler().ServeHTTP(rec, req)
	return rec
}

// TestTheContractAndTheDaemonNameTheSameBody is the ratchet: the contract's
// schema and the plugin's own struct name the same fields, with the same
// types, in both directions.
//
// The member makes the same check when it is built — so a contract that
// mistyped a field stops the daemon, not just this test.
func TestTheContractAndTheDaemonNameTheSameBody(t *testing.T) {
	if err := channelhookapi.CheckShape(channelhookapi.MemberIMessage, reflect.TypeOf(imessageWebhook{})); err != nil {
		t.Fatalf("the contract and the plugin disagree: %v", err)
	}
}

// TestTheDoorRefusesAMistypedField: a body that carries a number where the
// contract names a string is refused with the registry's words.
func TestTheDoorRefusesAMistypedField(t *testing.T) {
	c := doorChannel(t)
	rec := doorPost(t, c, `{"type":"new-message","data":{"text":42,"handle":{"address":"a@b"}}}`)
	got := strings.TrimSpace(rec.Body.String())
	want := "operation imessage input: $.data.text has wrong type: got number, want string"
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a mistyped field answered %d %q, want 400", rec.Code, got)
	}
	if got != want {
		t.Errorf("a mistyped field said %q, want %q", got, want)
	}
}

// TestTheDoorRefusesANullField covers the arm the plugin's own decoder used
// to swallow: a null is not a value, and it is not a default either.
func TestTheDoorRefusesANullField(t *testing.T) {
	c := doorChannel(t)
	rec := doorPost(t, c, `{"type":"new-message","data":{"text":null,"handle":{"address":"a@b"}}}`)
	got := strings.TrimSpace(rec.Body.String())
	want := "operation imessage input: $.data.text has wrong type: got null, want string"
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a null field answered %d %q, want 400", rec.Code, got)
	}
	if got != want {
		t.Errorf("a null field said %q, want %q", got, want)
	}
}

// TestTheDoorLetsAContractBodyThrough proves the door does not close the
// member: a body that fits the contract reaches the member.
func TestTheDoorLetsAContractBodyThrough(t *testing.T) {
	c := doorChannel(t)
	rec := doorPost(t, c, `{"type":"new-message","data":{"guid":"g1","text":"hi","handle":{"address":"a@b"},"chats":[{"guid":"c1"}]}}`)
	got := strings.TrimSpace(rec.Body.String())
	if rec.Code == http.StatusBadRequest && strings.Contains(got, "has wrong type") {
		t.Errorf("the member refused a body that fits the contract: %s", got)
	}
}

// TestTheDoorAllowsAFieldTheContractDoesNotName keeps the door the registry's
// door: a body is the stable SUBSET of what the daemon reads, so a caller may
// send a field the contract does not declare.
func TestTheDoorAllowsAFieldTheContractDoesNotName(t *testing.T) {
	c := doorChannel(t)
	body := `{"type":"new-message","data":{"guid":"g1","text":"hi","handle":{"address":"a@b"},"chats":[{"guid":"c1"}]},"extra_field":1}`
	rec := doorPost(t, c, body)
	got := strings.TrimSpace(rec.Body.String())
	if strings.Contains(got, "is not allowed by schema") {
		t.Errorf("the member refused a body carrying an undeclared field: %s", got)
	}
	if rec.Code == http.StatusBadRequest && strings.Contains(got, "has wrong type") {
		t.Errorf("the member refused a body carrying an undeclared field: %s", got)
	}
}

// TestTheDoorRefusesABodyThatIsNotJSON: the member's own voice for a body it
// cannot read at all.
//
// This case used to live in the parse test, because the parser used to be the
// one that dropped such a body — silently, and after the member had already
// answered 200. It is the DOOR's business now, and the answer is the
// member's plain text: "bad json".
func TestTheDoorRefusesABodyThatIsNotJSON(t *testing.T) {
	c := doorChannel(t)
	rec := doorPost(t, c, "not json")
	got := strings.TrimSpace(rec.Body.String())
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a body that is not JSON answered %d %q, want 400", rec.Code, got)
	}
	if got != "bad json" {
		t.Errorf("a body that is not JSON said %q, want %q", got, "bad json")
	}
}
