// SPDX-License-Identifier: MIT

package dingtalk

// The DingTalk member's request door.
//
// The member's body shape is named in the contract
// (kernel/contract/channelhookapi) and measured before the plugin reads it.
// The refusal carries the operation registry's own words:
//
//	operation dingtalk input: $.text.content has wrong type: got number, want string
//
// and the member keeps its own VOICE — the plain text its callers already
// read ("forbidden", "bad body"). "bad json" is NEW to this member: DingTalk
// used to answer 200 to a body it could not read, and swallowed it.
//
// The tests below hold the door to six things:
//
//  1. the contract's schema and the plugin's own struct name the SAME fields
//     with the SAME types — the vocabulary has one home, and the member
//     checks it when it is built;
//  2. a mistyped field is refused with the registry's words;
//  3. a null is refused too — a null is not a value, and not a default;
//  4. a body that fits the contract gets through untouched;
//  5. a field the contract does not name is still allowed — a vendor's body
//     carries fields Agezt never looks at;
//  6. a body that is not JSON at all is the member's own "bad json" — and it
//     is a 400 now, not a silent 200.
//
// The signature is checked FIRST, so a body is only ever read by someone who
// proved they may send it.

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/channelhookapi"
)

// doorSecret is the robot secret the door's tests sign with.
const doorSecret = "sek"

// doorChannel is a member with the dependency the door's tests need: the
// secret that signs an inbound.
func doorChannel(t *testing.T) *Channel {
	t.Helper()
	return New(Config{Secret: doorSecret})
}

// doorPost posts a body to the member's door, signed with the member's own
// signature scheme (timestamp + sign headers).
func doorPost(t *testing.T, c *Channel, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, DefaultPath, strings.NewReader(body))
	ts, sign := signNow(doorSecret)
	req.Header.Set("timestamp", ts)
	req.Header.Set("sign", sign)
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
	if err := channelhookapi.CheckShape(channelhookapi.MemberDingTalk, reflect.TypeOf(dingtalkBody{})); err != nil {
		t.Fatalf("the contract and the plugin disagree: %v", err)
	}
}

// TestTheDoorRefusesAMistypedField: a signed body whose field carries the
// wrong type is refused with the registry's words.
func TestTheDoorRefusesAMistypedField(t *testing.T) {
	c := doorChannel(t)
	rec := doorPost(t, c, `{"msgtype":"text","text":{"content":42},"senderStaffId":"S1","msgId":"M1"}`)
	got := strings.TrimSpace(rec.Body.String())
	want := "operation dingtalk input: $.text.content has wrong type: got number, want string"
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
	rec := doorPost(t, c, `{"msgtype":"text","text":{"content":null},"senderStaffId":"S1","msgId":"M1"}`)
	got := strings.TrimSpace(rec.Body.String())
	want := "operation dingtalk input: $.text.content has wrong type: got null, want string"
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a null field answered %d %q, want 400", rec.Code, got)
	}
	if got != want {
		t.Errorf("a null field said %q, want %q", got, want)
	}
}

// TestTheDoorLetsAContractBodyThrough proves the door does not close the
// member: a body that fits the contract reaches the plugin.
func TestTheDoorLetsAContractBodyThrough(t *testing.T) {
	c := doorChannel(t)
	body := `{"msgtype":"text","text":{"content":"hi"},"senderStaffId":"S1","senderNick":"Al","msgId":"M1","sessionWebhook":"https://oapi.dingtalk.com/robot/sendBySession?session=x"}`
	rec := doorPost(t, c, body)
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
	body := `{"msgtype":"text","text":{"content":"hi"},"senderStaffId":"S1","msgId":"M1","extra_field":1}`
	rec := doorPost(t, c, body)
	got := strings.TrimSpace(rec.Body.String())
	if strings.Contains(got, "is not allowed by schema") {
		t.Errorf("the member refused a body carrying an undeclared field: %s", got)
	}
}

// TestTheDoorRefusesABodyThatIsNotJSON: the member's own voice for a body it
// cannot read — "bad json", 400. DingTalk used to answer 200 to such a body
// and swallow it; the door makes it a refusal.
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
