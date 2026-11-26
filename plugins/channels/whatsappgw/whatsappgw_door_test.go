// SPDX-License-Identifier: MIT

package whatsappgw

// The WhatsApp gateway member's request door.
//
// The member's body shape is named in the contract
// (kernel/contract/channelhookapi) and measured before the plugin reads it.
// The refusal carries the operation registry's own words:
//
//	operation whatsappgw input: $.payload.body has wrong type: got number, want string
//
// and the member keeps its own VOICE — the plain text its callers already
// read ("forbidden", "bad body") plus one new one: "bad json", the answer to
// a body that is not JSON at all. That body used to be answered 200 and
// swallowed; it is now refused before the member writes anything.
//
// One body type holds the gateway's TWO dialects: WAHA posts its message
// under "payload", Evolution under "data" — and one route carries both, so
// the door measures them alike.
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
//     SUBSET of what the daemon reads, and a vendor's body carries fields
//     Agezt never looks at;
//  6. a body that is not JSON at all is the member's own "bad json" — and it
//     is a 400 now, not a silent 200.
//
// The shared secret is checked FIRST, so a body is only ever read by someone
// who proved they may send it.

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/channelhookapi"
)

// doorSecret is the shared secret the member verifies inbound against.
const doorSecret = "gw-secret"

// doorChannel is a member with the dependency the door's tests need: the
// shared secret.
func doorChannel(t *testing.T) *Channel {
	t.Helper()
	return New(Config{Backend: BackendWAHA, Secret: doorSecret})
}

// doorPost sends a body to the member's own inbound route, carrying the
// shared secret the way the gateway carries it (X-Webhook-Secret).
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
	if err := channelhookapi.CheckShape(channelhookapi.MemberWhatsAppGW, reflect.TypeOf(whatsappgwBody{})); err != nil {
		t.Fatalf("the contract and the plugin disagree: %v", err)
	}
}

// TestTheDoorRefusesAMistypedField: a body whose field carries the wrong type
// is refused with the registry's words.
func TestTheDoorRefusesAMistypedField(t *testing.T) {
	c := doorChannel(t)
	body := `{"event":"message","payload":{"from":"12345@c.us","body":42,"id":"AAA","fromMe":false}}`
	rec := doorPost(t, c, body)
	got := strings.TrimSpace(rec.Body.String())
	want := "operation whatsappgw input: $.payload.body has wrong type: got number, want string"
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
	body := `{"event":"message","payload":{"from":"12345@c.us","body":null,"id":"AAA","fromMe":false}}`
	rec := doorPost(t, c, body)
	got := strings.TrimSpace(rec.Body.String())
	want := "operation whatsappgw input: $.payload.body has wrong type: got null, want string"
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
	body := `{"event":"message","session":"default","payload":{"from":"12345@c.us","body":"hi there","id":"AAA","fromMe":false}}`
	rec := doorPost(t, c, body)
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
	body := `{"event":"message","payload":{"from":"12345@c.us","body":"hi there","id":"AAA","fromMe":false},"timestamp":1700000000}`
	rec := doorPost(t, c, body)
	got := strings.TrimSpace(rec.Body.String())
	if strings.Contains(got, "is not allowed by schema") {
		t.Errorf("the member refused a body carrying an undeclared field: %s", got)
	}
	if rec.Code == http.StatusBadRequest && strings.Contains(got, "has wrong type") {
		t.Errorf("the member refused a body carrying an undeclared field: %s", got)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("a body carrying an undeclared field answered %d %q, want 200", rec.Code, got)
	}
}

// TestTheDoorRefusesABodyThatIsNotJSON: the member's own voice for a body it
// cannot read at all.
//
// The body carries the shared secret like any other — and the door, not the
// secret, is what refuses it: "bad json", in plain text.
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
