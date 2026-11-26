// SPDX-License-Identifier: MIT

package feishu

// The Feishu member's request door.
//
// The member's body shape is named in the contract
// (kernel/contract/channelhookapi) and measured before the plugin reads it.
// The refusal carries the operation registry's own words:
//
//	operation feishu input: $.event.message.message_id has wrong type: got number, want string
//
// and the member keeps its own VOICE — the plain text its callers already
// read ("forbidden", "bad body") plus one new one: "bad json", the answer to
// a body that is not JSON at all.
//
// Feishu's token is INSIDE the body — the handshake's top-level "token", the
// event's "header.token" — so it is read from the raw bytes and verified
// BEFORE the door: a body is only measured by someone who proved they may
// send it. A body that is not JSON at all carries no token, so there is
// nothing to verify and the door is what refuses it ("bad json").
//
// The tests below hold the door to six things:
//
//  1. the contract's schema and the plugin's own struct name the SAME fields
//     with the SAME types — the vocabulary has one home, and the member
//     checks it when it is built;
//  2. a mistyped field is refused with the registry's words;
//  3. a null is refused too — a null is not a value, and not a default either;
//  4. a body that fits the contract gets through untouched;
//  5. a field the contract does not name is allowed — a body is the stable
//     SUBSET of what the daemon reads, and Feishu carries fields Agezt never
//     looks at;
//  6. a body that is not JSON at all is refused in the member's own words.

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/channelhookapi"
)

// doorToken is the member's verification token — the one the bodies below
// carry, at the top level (the handshake) or in the header (the event).
const doorToken = "t1"

// doorChannel is a member with the dependency the door's tests need: the
// verification token.
func doorChannel(t *testing.T) *Channel {
	t.Helper()
	return New(Config{VerifyToken: doorToken})
}

// doorPost sends a body to the member's own inbound route.
//
// Feishu signs nothing: the token travels inside the body, so the body IS
// the credential.
func doorPost(t *testing.T, c *Channel, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, DefaultPath, strings.NewReader(body))
	rec := httptest.NewRecorder()
	c.Handler().ServeHTTP(rec, req)
	return rec
}

// doorEvent is a body that fits the contract, with the given message_id
// field spliced in.
func doorEvent(messageID string) string {
	return `{"header":{"token":"` + doorToken + `","event_id":"E1","event_type":"im.message.receive_v1"},` +
		`"event":{"sender":{"sender_id":{"open_id":"ou_1"}},` +
		`"message":{"message_id":` + messageID + `,"chat_id":"oc_1","message_type":"text","content":"{\"text\":\"hi\"}"}}}`
}

// TestTheContractAndTheDaemonNameTheSameBody is the ratchet: the contract's
// schema and the plugin's own struct name the same fields, with the same
// types, in both directions.
//
// The member makes the same check when it is built — so a contract that
// mistyped a field stops the daemon, not just this test.
func TestTheContractAndTheDaemonNameTheSameBody(t *testing.T) {
	if err := channelhookapi.CheckShape(channelhookapi.MemberFeishu, reflect.TypeOf(feishuWebhook{})); err != nil {
		t.Fatalf("the contract and the plugin disagree: %v", err)
	}
}

// TestTheDoorRefusesAMistypedField: a body that carries a number where the
// contract names a string is refused with the registry's words.
func TestTheDoorRefusesAMistypedField(t *testing.T) {
	c := doorChannel(t)
	rec := doorPost(t, c, doorEvent("42"))
	got := strings.TrimSpace(rec.Body.String())
	want := "operation feishu input: $.event.message.message_id has wrong type: got number, want string"
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
	rec := doorPost(t, c, doorEvent("null"))
	got := strings.TrimSpace(rec.Body.String())
	want := "operation feishu input: $.event.message.message_id has wrong type: got null, want string"
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
	rec := doorPost(t, c, doorEvent(`"m1"`))
	got := strings.TrimSpace(rec.Body.String())
	if rec.Code == http.StatusBadRequest && strings.Contains(got, "has wrong type") {
		t.Errorf("the member refused a body that fits the contract: %s", got)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("a body that fits the contract answered %d %q, want 200", rec.Code, got)
	}
}

// TestTheDoorAllowsAFieldTheContractDoesNotName keeps the door the registry's
// door: a body is the stable SUBSET of what the daemon reads, so a caller may
// send a field the contract does not declare.
func TestTheDoorAllowsAFieldTheContractDoesNotName(t *testing.T) {
	c := doorChannel(t)
	body := `{"header":{"token":"` + doorToken + `","event_id":"E1","event_type":"im.message.receive_v1"},` +
		`"event":{"sender":{"sender_id":{"open_id":"ou_1"}},` +
		`"message":{"message_id":"m1","chat_id":"oc_1","message_type":"text","content":"{\"text\":\"hi\"}"}},` +
		`"extra_field":1}`
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
// Such a body carries no token, so there is nothing to verify — the door,
// not the token, is what refuses it: "bad json", in plain text.
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

// TestTheTokenIsVerifiedBeforeTheDoor keeps the member's security order: a
// body whose token is wrong is refused with 403 "forbidden" — the token is
// checked FIRST, before the shape is measured.
func TestTheTokenIsVerifiedBeforeTheDoor(t *testing.T) {
	c := doorChannel(t)
	body := `{"header":{"token":"wrong","event_id":"E1","event_type":"im.message.receive_v1"},` +
		`"event":{"sender":{"sender_id":{"open_id":"ou_1"}},` +
		`"message":{"message_id":42,"chat_id":"oc_1","message_type":"text","content":"{\"text\":\"hi\"}"}}}`
	rec := doorPost(t, c, body)
	got := strings.TrimSpace(rec.Body.String())
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a wrong token answered %d %q, want 403", rec.Code, got)
	}
	if got != "forbidden" {
		t.Errorf("a wrong token said %q, want %q", got, "forbidden")
	}
}
