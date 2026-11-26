// SPDX-License-Identifier: MIT

package webhook

// The generic webhook member's request door.
//
// The member's body shape is named in the contract
// (kernel/contract/channelhookapi) and measured before the plugin reads it.
// The refusal carries the operation registry's own words:
//
//	operation webhook input: $.text has wrong type: got number, want string
//
// and the member keeps its own VOICE — the plain text its callers already
// read ("bad json", "bad signature", "channel_id and text are required").
//
// The tests below hold the door to four things:
//
//  1. the contract's schema and the plugin's own struct name the SAME fields
//     with the SAME types — the vocabulary has one home, and the member
//     checks it when it is built;
//  2. a mistyped field — and a null — is refused with the registry's words,
//     for every field of the body;
//  3. a body that fits the contract gets through untouched;
//  4. the member's own refusals are still the member's own.
//
// The guards walk the fields the CONTRACT names, and the contract is held to
// the plugin's own struct by the check the member makes when it is built — so
// a schema that mistyped a field cannot agree with itself and pass.

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/contract/channelhookapi"
)

const doorSecret = "s3cr3t"

// doorChannel is a member with the dependencies the door's tests need: a
// signing secret, an allowlist and a handler.
func doorChannel(t *testing.T) *Channel {
	t.Helper()
	return New(Config{
		Secret:    doorSecret,
		Allowlist: channel.NewAllowlist([]string{"room1"}),
		Handler: func(context.Context, channel.UnifiedMessage, string) (channel.Reply, error) {
			return channel.Reply{Text: "echo"}, nil
		},
	})
}

// doorWrong is the value that does NOT fit each field of the body.
//
// The guards walk the fields the contract names, so a field the contract adds
// without a value here is a field no guard covers — and the coverage test
// says so.
var doorWrong = map[string]any{
	"channel_id": 42,
	"sender":     42,
	"text":       42,
	"id":         42,
	"ts_ms":      "7",
	"images":     "x",
}

// doorValid is a body that fits the contract: an allowlisted channel, a
// sender, a text and the optional fields.
func doorValid() map[string]any {
	return map[string]any{
		"channel_id": "room1",
		"sender":     "u1",
		"text":       "hi",
		"id":         "m1",
		"ts_ms":      0,
		"images":     []string{"https://example.test/a.png"},
	}
}

// doorBodyWith is a body that fits the contract, except for the one field
// named, which carries value.
func doorBodyWith(field string, value any) map[string]any {
	body := doorValid()
	if field != "" {
		body[field] = value
	}
	return body
}

// TestTheContractAndTheDaemonNameTheSameBody is the ratchet: the contract's
// schema and the plugin's own struct name the same fields, with the same
// types, in both directions.
//
// The member makes the same check when it is built — so a contract that
// mistyped a field stops the daemon, not just this test.
func TestTheContractAndTheDaemonNameTheSameBody(t *testing.T) {
	if err := channelhookapi.CheckShape(channelhookapi.MemberWebhook, reflect.TypeOf(inboundEnvelope{})); err != nil {
		t.Fatalf("the contract and the plugin disagree: %v", err)
	}
}

// TestEveryFieldIsGuarded: every field the contract names has a value that
// does not fit it, so no field of the body is left unmeasured.
func TestEveryFieldIsGuarded(t *testing.T) {
	fields := channelhookapi.Fields(channelhookapi.MemberWebhook)
	if len(fields) == 0 {
		t.Fatal("the contract names no field — the door has nothing to guard")
	}
	for _, f := range fields {
		if _, ok := doorWrong[f.Name]; !ok {
			t.Errorf("the contract names %q, but no guard covers it", f.Name)
		}
	}
	for name := range doorWrong {
		found := false
		for _, f := range fields {
			if f.Name == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("the guard covers %q, which the contract does not name", name)
		}
	}
}

// TestTheDoorRefusesAMistypedField: for every field of the body, a body that
// carries the wrong type is refused with the registry's words.
func TestTheDoorRefusesAMistypedField(t *testing.T) {
	c := doorChannel(t)
	for _, f := range channelhookapi.Fields(channelhookapi.MemberWebhook) {
		wrong, ok := doorWrong[f.Name]
		if !ok {
			continue
		}
		rec := post(t, c, doorBodyWith(f.Name, wrong), doorSecret, true)
		got := strings.TrimSpace(rec.Body.String())
		want := "operation webhook input: $." + f.Name + " has wrong type: got "
		if rec.Code != http.StatusBadRequest {
			t.Errorf("a mistyped %q answered %d %s, want 400", f.Name, rec.Code, got)
			continue
		}
		if !strings.Contains(got, want) {
			t.Errorf("a mistyped %q said %q, want it to contain %q", f.Name, got, want)
			continue
		}
		if !strings.Contains(got, "want "+strings.Join(f.Types, "|")) {
			t.Errorf("a mistyped %q said %q, want it to name the type %q", f.Name, got, strings.Join(f.Types, "|"))
		}
	}
}

// TestTheDoorRefusesANullField covers the arm the plugin's own decoder used
// to swallow: a null is not a value, and it is not a default either.
func TestTheDoorRefusesANullField(t *testing.T) {
	c := doorChannel(t)
	for _, f := range channelhookapi.Fields(channelhookapi.MemberWebhook) {
		if _, ok := doorWrong[f.Name]; !ok {
			continue
		}
		rec := post(t, c, doorBodyWith(f.Name, nil), doorSecret, true)
		got := strings.TrimSpace(rec.Body.String())
		if rec.Code != http.StatusBadRequest {
			t.Errorf("a null %q answered %d %s, want 400", f.Name, rec.Code, got)
			continue
		}
		want := "operation webhook input: $." + f.Name + " has wrong type: got null, want " +
			strings.Join(f.Types, "|")
		if got != want {
			t.Errorf("a null %q said %q, want %q", f.Name, got, want)
		}
	}
}

// TestTheDoorLetsAContractBodyThrough proves the door does not close the
// member: a body that fits the contract reaches the handler.
func TestTheDoorLetsAContractBodyThrough(t *testing.T) {
	c := doorChannel(t)
	rec := post(t, c, doorValid(), doorSecret, true)
	got := strings.TrimSpace(rec.Body.String())
	if rec.Code == http.StatusBadRequest && strings.Contains(got, "has wrong type") {
		t.Errorf("the member refused a body that fits the contract: %s", got)
	}
}

// TestTheDoorAllowsAFieldTheContractDoesNotName keeps the door the registry's
// door: a body is the stable SUBSET of what the daemon reads, so a caller may
// send a field the contract does not declare.
//
// The refusal is looked for by its WORDS ("is not allowed by schema"), not by
// the status code: the member answers 400 for its own reasons too (an empty
// channel), and that is the member's answer, not the door's.
func TestTheDoorAllowsAFieldTheContractDoesNotName(t *testing.T) {
	c := doorChannel(t)
	body := doorValid()
	body["extra_field"] = 1
	rec := post(t, c, body, doorSecret, true)
	got := strings.TrimSpace(rec.Body.String())
	if strings.Contains(got, "is not allowed by schema") {
		t.Errorf("the member refused a body carrying an undeclared field: %s", got)
	}
}

// TestTheMemberKeepsItsOwnRefusals proves the door did not take the member's
// own answers with it: an unsigned body is still a bad signature, a body
// without a channel is still the member's own "channel_id and text are
// required", and a channel that is not allowlisted is still a 403.
func TestTheMemberKeepsItsOwnRefusals(t *testing.T) {
	c := doorChannel(t)

	// An unsigned body: the signature is checked BEFORE the body is read.
	rec := post(t, c, doorValid(), doorSecret, false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("an unsigned body answered %d %s, want 401", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "bad signature") {
		t.Errorf("an unsigned body answered %q, want the member's own \"bad signature\"", rec.Body.String())
	}

	// A body that fits the contract but names no channel: the member's own
	// presence rule, not a schema error.
	rec = post(t, c, map[string]any{"channel_id": "", "text": "hi"}, doorSecret, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("an empty channel answered %d %s, want 400", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "channel_id and text are required") {
		t.Errorf("an empty channel answered %q, want the member's own \"channel_id and text are required\"", rec.Body.String())
	}

	// A channel that is not allowlisted: still a 403.
	rec = post(t, c, map[string]any{"channel_id": "intruder", "text": "hi"}, doorSecret, true)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a non-allowlisted channel answered %d %s, want 403", rec.Code, rec.Body.String())
	}
}
