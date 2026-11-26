// SPDX-License-Identifier: MIT

package zalo

// The Zalo member's request door.
//
// The member's body shape is named in the contract
// (kernel/contract/channelhookapi) and measured before the plugin reads it.
// The refusal carries the operation registry's own words:
//
//	operation zalo input: $.sender.id has wrong type: got number, want string
//
// and the member keeps its own VOICE — the plain text its callers already
// read ("forbidden", "bad body") plus one new one: "bad json", the answer to
// a body that is not JSON at all.
//
// Zalo's signature covers the timestamp INSIDE the body, so the body is read
// (for the timestamp) before it is decoded, and the signature is verified
// BEFORE the door: a body is only measured by someone who proved they may
// send it.
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
//     SUBSET of what the daemon reads, and Zalo carries fields Agezt never
//     looks at.

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/channelhookapi"
)

const (
	// doorAppID is the OA app id — part of the signature.
	doorAppID = "app"
	// doorSecret is the OA secret key — the other part of the signature.
	doorSecret = "sek"
)

// doorChannel is a member with the dependencies the door's tests need: the
// app id and the signing secret.
func doorChannel(t *testing.T) *Channel {
	t.Helper()
	return New(Config{AppID: doorAppID, Secret: doorSecret})
}

// doorSign is the signature Zalo sends for a body: sha256(appId + body +
// timestamp + secret), hex, prefixed "mac=" — the recipe the plugin verifies.
func doorSign(body, timestamp string) string {
	h := sha256.New()
	h.Write([]byte(doorAppID))
	h.Write([]byte(body))
	h.Write([]byte(timestamp))
	h.Write([]byte(doorSecret))
	return "mac=" + hex.EncodeToString(h.Sum(nil))
}

// doorPost sends a signed body to the member's own inbound route.
//
// The timestamp is part of the signature, so the caller says which one it
// signed with.
func doorPost(t *testing.T, c *Channel, body, timestamp string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, DefaultPath, strings.NewReader(body))
	req.Header.Set("X-ZEvent-Signature", doorSign(body, timestamp))
	rec := httptest.NewRecorder()
	c.Handler().ServeHTTP(rec, req)
	return rec
}

// doorNow is a timestamp inside the replay window, for the bodies that must
// reach past the door.
func doorNow() string {
	return strconv.FormatInt(time.Now().UnixMilli(), 10)
}

// TestTheContractAndTheDaemonNameTheSameBody is the ratchet: the contract's
// schema and the plugin's own struct name the same fields, with the same
// types, in both directions.
//
// The member makes the same check when it is built — so a contract that
// mistyped a field stops the daemon, not just this test.
func TestTheContractAndTheDaemonNameTheSameBody(t *testing.T) {
	if err := channelhookapi.CheckShape(channelhookapi.MemberZalo, reflect.TypeOf(zaloWebhook{})); err != nil {
		t.Fatalf("the contract and the plugin disagree: %v", err)
	}
}

// TestTheDoorRefusesAMistypedField: a body that carries a number where the
// contract names a string is refused with the registry's words.
func TestTheDoorRefusesAMistypedField(t *testing.T) {
	c := doorChannel(t)
	body := `{"event_name":"user_send_text","sender":{"id":42},"message":{"msg_id":"m","text":"hi"},"timestamp":"1700000000000"}`
	rec := doorPost(t, c, body, "1700000000000")
	got := strings.TrimSpace(rec.Body.String())
	want := "operation zalo input: $.sender.id has wrong type: got number, want string"
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
	body := `{"event_name":"user_send_text","sender":{"id":null},"message":{"msg_id":"m","text":"hi"},"timestamp":"1700000000000"}`
	rec := doorPost(t, c, body, "1700000000000")
	got := strings.TrimSpace(rec.Body.String())
	want := "operation zalo input: $.sender.id has wrong type: got null, want string"
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
	ts := doorNow()
	body := `{"event_name":"user_send_text","sender":{"id":"u1"},"message":{"msg_id":"m1","text":"hi"},"timestamp":"` + ts + `"}`
	rec := doorPost(t, c, body, ts)
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
	ts := doorNow()
	body := `{"event_name":"user_send_text","sender":{"id":"u1"},"message":{"msg_id":"m1","text":"hi"},"timestamp":"` + ts + `","extra_field":1}`
	rec := doorPost(t, c, body, ts)
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
// A body that is not JSON carries no timestamp, so the signature is made over
// an empty one — and the door, not the signature, is what refuses the body:
// "bad json", in plain text.
func TestTheDoorRefusesABodyThatIsNotJSON(t *testing.T) {
	c := doorChannel(t)
	rec := doorPost(t, c, "not json", "")
	got := strings.TrimSpace(rec.Body.String())
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a body that is not JSON answered %d %q, want 400", rec.Code, got)
	}
	if got != "bad json" {
		t.Errorf("a body that is not JSON said %q, want %q", got, "bad json")
	}
}
