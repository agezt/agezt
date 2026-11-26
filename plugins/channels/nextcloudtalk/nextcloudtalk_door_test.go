// SPDX-License-Identifier: MIT

package nextcloudtalk

// The Nextcloud Talk member's request door.
//
// The member's body shape is named in the contract
// (kernel/contract/channelhookapi) and measured before the plugin reads it.
// The refusal carries the operation registry's own words:
//
//	operation nextcloudtalk input: $.object.content has wrong type: got number, want string
//
// and the member keeps its own VOICE — the plain text its callers already
// read ("bad signature") plus one new one: "bad json", the answer to a body
// that is not JSON at all. Such a body used to be answered 200 and swallowed.
//
// Nextcloud signs the body with an HMAC over the raw bytes, so the signature
// is verified BEFORE the door: a body is only measured by someone who proved
// they may send it.
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
//     SUBSET of what the daemon reads, and Nextcloud carries fields Agezt
//     never looks at;
//  6. a body that is not JSON at all is refused in the member's own words.

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/channelhookapi"
)

// doorChannel is a member with the dependency the door's tests need: the
// bot secret the inbound signature is made with.
func doorChannel(t *testing.T) *Channel {
	t.Helper()
	return New(Config{Secret: secret})
}

// doorPost sends a signed body to the member's own inbound route.
//
// The signature covers the random header value and the raw body, so the
// caller signs the very bytes the member measures.
func doorPost(t *testing.T, c *Channel, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, DefaultPath, strings.NewReader(body))
	req.Header.Set("X-Nextcloud-Talk-Random", "rnd")
	req.Header.Set("X-Nextcloud-Talk-Signature", sigFor("rnd", []byte(body)))
	rec := httptest.NewRecorder()
	c.Handler().ServeHTTP(rec, req)
	return rec
}

// doorActivity is a body that fits the contract, with the given object.content
// field spliced in.
func doorActivity(content string) string {
	return `{"type":"Create","actor":{"id":"users/alice"},"object":{"id":"msg-1","content":` + content + `},"target":{"id":"tok123"}}`
}

// TestTheContractAndTheDaemonNameTheSameBody is the ratchet: the contract's
// schema and the plugin's own struct name the same fields, with the same
// types, in both directions.
//
// The member makes the same check when it is built — so a contract that
// mistyped a field stops the daemon, not just this test.
func TestTheContractAndTheDaemonNameTheSameBody(t *testing.T) {
	if err := channelhookapi.CheckShape(channelhookapi.MemberNextcloudTalk, reflect.TypeOf(nextcloudtalkBody{})); err != nil {
		t.Fatalf("the contract and the plugin disagree: %v", err)
	}
}

// TestTheDoorRefusesAMistypedField: a body that carries a number where the
// contract names a string is refused with the registry's words.
func TestTheDoorRefusesAMistypedField(t *testing.T) {
	c := doorChannel(t)
	rec := doorPost(t, c, doorActivity("42"))
	got := strings.TrimSpace(rec.Body.String())
	want := "operation nextcloudtalk input: $.object.content has wrong type: got number, want string"
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
	rec := doorPost(t, c, doorActivity("null"))
	got := strings.TrimSpace(rec.Body.String())
	want := "operation nextcloudtalk input: $.object.content has wrong type: got null, want string"
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
	rec := doorPost(t, c, doorActivity(`"{\"message\":\"hello bot\"}"`))
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
	body := `{"type":"Create","actor":{"id":"users/alice"},"object":{"id":"msg-1","content":"hi"},"target":{"id":"tok123"},"extra_field":1}`
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
// The body is signed like any other, so the signature passes and the door,
// not the signature, is what refuses it: "bad json", in plain text.
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

// TestTheSignatureIsVerifiedBeforeTheDoor keeps the member's security order:
// a body whose signature is wrong is refused with 401 "bad signature" — the
// signature is checked FIRST, before the shape is measured.
func TestTheSignatureIsVerifiedBeforeTheDoor(t *testing.T) {
	c := doorChannel(t)
	body := doorActivity("42")
	req := httptest.NewRequest(http.MethodPost, DefaultPath, strings.NewReader(body))
	req.Header.Set("X-Nextcloud-Talk-Random", "rnd")
	req.Header.Set("X-Nextcloud-Talk-Signature", "deadbeef")
	rec := httptest.NewRecorder()
	c.Handler().ServeHTTP(rec, req)
	got := strings.TrimSpace(rec.Body.String())
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("a bad signature answered %d %q, want 401", rec.Code, got)
	}
	if got != "bad signature" {
		t.Errorf("a bad signature said %q, want %q", got, "bad signature")
	}
}
