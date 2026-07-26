// SPDX-License-Identifier: MIT

package standingtool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/standing"
)

type fakeHost struct {
	orders    []standing.Order
	store     *standing.Store
	addErr    error
	removeOK  bool
	removeErr error
	gotAdd    standing.Order
	gotRemove string
}

func (f *fakeHost) AddStanding(o standing.Order) (standing.Order, error) {
	f.gotAdd = o
	if f.addErr != nil {
		return standing.Order{}, f.addErr
	}
	o.ID = "so-1"
	f.orders = append(f.orders, o)
	return o, nil
}

func (f *fakeHost) RemoveStanding(id string) (bool, error) {
	f.gotRemove = id
	return f.removeOK, f.removeErr
}

func (f *fakeHost) Standing() *standing.Store { return f.store }

// fakeRosterHost is a host that also implements rosterHost so we can exercise
// validateActingAgent's roster lookup.
type fakeRosterHost struct {
	fakeHost
	roster *roster.Store
}

func (f *fakeRosterHost) Roster() *roster.Store { return f.roster }

func TestStandingCoverageDefinition(t *testing.T) {
	tool := New()
	def := tool.Definition()
	if def.Name != "standing" {
		t.Fatalf("Name = %q", def.Name)
	}
	if def.Effect.Class != agent.EffectReversible {
		t.Fatalf("Effect.Class = %v, want %v", def.Effect.Class, agent.EffectReversible)
	}
	schema := string(def.InputSchema)
	for _, want := range []string{`"create_event"`, `"create_cron"`, `"list"`, `"remove"`, `"plan"`, `"schedule"`, `"subject"`} {
		if !strings.Contains(schema, want) {
			t.Fatalf("schema should mention %q, got %s", want, schema)
		}
	}
}

func TestStandingCoverageBind(t *testing.T) {
	tool := New()
	if tool.host != nil {
		t.Fatal("New should leave host nil")
	}
	tool.Bind(nil)
	if tool.host != nil {
		t.Fatal("Bind(nil) should not set host")
	}
	h := &fakeHost{}
	tool.Bind(h)
	if tool.host != h {
		t.Fatal("Bind(fakeHost) should set host")
	}
}

func TestStandingCoverageValidateActingAgent(t *testing.T) {
	// Host doesn't implement rosterHost → no validation, no error.
	tool := New()
	tool.Bind(&fakeHost{})
	res, blocked := tool.validateActingAgent("alice")
	if blocked {
		t.Fatalf("non-roster host should not block, got %+v", res)
	}

	// rosterHost without a roster store → no validation, no block.
	tool = New()
	tool.Bind(&fakeRosterHost{})
	res, blocked = tool.validateActingAgent("alice")
	if blocked {
		t.Fatalf("nil roster should not block, got %+v", res)
	}

	// Build a real roster with one profile (alice) for the cases below.
	r, err := roster.Open(t.TempDir())
	if err != nil {
		t.Fatalf("roster.Open: %v", err)
	}
	if _, err := r.Add(roster.Profile{Slug: "alice"}); err != nil {
		t.Fatalf("r.Add: %v", err)
	}

	tool = New()
	tool.Bind(&fakeRosterHost{roster: r})
	res, blocked = tool.validateActingAgent("ghost")
	if !blocked || !strings.Contains(res.Output, "is not in the roster") {
		t.Fatalf("unknown agent: %+v", res)
	}

	// Mark alice as retired via SetRetired.
	if _, err := r.SetRetired("alice", true); err != nil {
		t.Fatalf("SetRetired: %v", err)
	}
	res, blocked = tool.validateActingAgent("alice")
	if !blocked || !strings.Contains(res.Output, "retired") {
		t.Fatalf("retired agent: %+v", res)
	}

	// Revive then disable (paused).
	if _, err := r.SetRetired("alice", false); err != nil {
		t.Fatalf("SetRetired(revive): %v", err)
	}
	if _, err := r.SetEnabled("alice", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	res, blocked = tool.validateActingAgent("alice")
	if !blocked || !strings.Contains(res.Output, "paused") {
		t.Fatalf("paused agent: %+v", res)
	}

	// Re-enable and convert to a managed sub-agent (DirectCallable=false +
	// a parent).
	if _, err := r.SetEnabled("alice", true); err != nil {
		t.Fatalf("SetEnabled(resume): %v", err)
	}
	direct := false
	if _, err := r.Update("alice", func(p *roster.Profile) {
		p.ParentAgent = "boss"
		p.DirectCallable = &direct
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	res, blocked = tool.validateActingAgent("alice")
	if !blocked || !strings.Contains(res.Output, "managed sub-agent") || !strings.Contains(res.Output, "boss") {
		t.Fatalf("managed sub-agent: %+v", res)
	}

	// A healthy, enabled and directly callable roster agent is accepted.
	direct = true
	if _, err := r.Update("alice", func(p *roster.Profile) {
		p.DirectCallable = &direct
	}); err != nil {
		t.Fatalf("Update(direct): %v", err)
	}
	res, blocked = tool.validateActingAgent("alice")
	if blocked {
		t.Fatalf("directly callable agent should pass validation: %+v", res)
	}
}

func TestStandingCoverageManagedSubAgentHint(t *testing.T) {
	cases := []struct {
		p    roster.Profile
		want []string
	}{
		{p: roster.Profile{Slug: "x", ParentAgent: "boss"}, want: []string{"boss", "managed sub-agent"}},
		{p: roster.Profile{Slug: "x", OwnerAgent: "owner"}, want: []string{"owner", "managed sub-agent"}},
		{p: roster.Profile{Slug: "x"}, want: []string{"managed sub-agent", "route the work"}},
	}
	for _, tc := range cases {
		got := managedSubAgentStandingHint(tc.p)
		for _, want := range tc.want {
			if !strings.Contains(got, want) {
				t.Fatalf("hint = %q, want substring %q", got, want)
			}
		}
	}
}

func TestStandingCoverageHelpersAndJSON(t *testing.T) {
	// max64.
	if max64(0, 0) != 0 {
		t.Fatal("max64(0,0)")
	}
	if max64(5, 3) != 5 {
		t.Fatal("max64 should prefer larger first arg")
	}
	if max64(3, 5) != 5 {
		t.Fatal("max64 should prefer larger second arg")
	}

	// okJSON marshal-fail fallback.
	r := okJSON(make(chan int))
	if !r.IsError || !strings.Contains(r.Output, "marshal:") {
		t.Fatalf("okJSON marshal fail = %+v", r)
	}
}

func TestStandingCoverageInvokeValidation(t *testing.T) {
	// Parse error: hard.
	_, err := New().Invoke(context.Background(), json.RawMessage(`{`))
	if err == nil || !strings.Contains(err.Error(), "parse input") {
		t.Fatalf("parse error = %v", err)
	}

	// Unbound: soft error.
	res, err := New().Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("Invoke unbound: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "not available") {
		t.Fatalf("unbound result = %+v", res)
	}

	// Bound with empty op and unknown op.
	h := &fakeHost{}
	tool := New()
	tool.Bind(h)
	cases := map[string]string{
		`{"op":""}`:        "op required",
		`{"op":"unknown"}`: "unknown op",
	}
	for input, want := range cases {
		res, err := tool.Invoke(context.Background(), json.RawMessage(input))
		if err != nil {
			t.Fatalf("Invoke %q: %v", input, err)
		}
		if !res.IsError || !strings.Contains(res.Output, want) {
			t.Fatalf("Invoke %q output = %q, want substring %q", input, res.Output, want)
		}
	}

	// create_event with no subject → soft error.
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"op":"create_event","name":"n","plan":"p"}`))
	if err != nil {
		t.Fatalf("Invoke no subject: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "needs a \"subject\"") {
		t.Fatalf("no subject = %+v", res)
	}

	// create_cron with no schedule → soft error.
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"op":"create_cron","name":"n","plan":"p"}`))
	if err != nil {
		t.Fatalf("Invoke no schedule: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "needs a \"schedule\"") {
		t.Fatalf("no schedule = %+v", res)
	}

	// create with missing name.
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"op":"create_event","subject":"x","plan":"p"}`))
	if err != nil {
		t.Fatalf("Invoke no name: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "\"name\" is required") {
		t.Fatalf("no name = %+v", res)
	}

	// create with missing plan.
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"op":"create_event","subject":"x","name":"n"}`))
	if err != nil {
		t.Fatalf("Invoke no plan: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "\"plan\" is required") {
		t.Fatalf("no plan = %+v", res)
	}

	// remove without id.
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"op":"remove"}`))
	if err != nil {
		t.Fatalf("Invoke remove no id: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "needs an \"id\"") {
		t.Fatalf("remove no id = %+v", res)
	}

	// remove with id but not found.
	h.removeOK = false
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"op":"remove","id":"missing"}`))
	if err != nil {
		t.Fatalf("Invoke remove missing: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "no standing order") {
		t.Fatalf("remove missing = %+v", res)
	}

	// remove success.
	h.removeOK = true
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"op":"remove","id":"so-1"}`))
	if err != nil {
		t.Fatalf("Invoke remove success: %v", err)
	}
	if res.IsError || !strings.Contains(res.Output, `"removed"`) {
		t.Fatalf("remove success = %+v", res)
	}

	// Host-side remove failures are returned as soft tool errors.
	h.removeErr = errors.New("remove failed")
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"op":"remove","id":"so-1"}`))
	if err != nil || !res.IsError || !strings.Contains(res.Output, "remove failed") {
		t.Fatalf("remove failure = %+v, %v", res, err)
	}
}

func TestStandingCoverageInvokeCreateEvent(t *testing.T) {
	h := &fakeHost{}
	tool := New()
	tool.Bind(h)
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"create_event","subject":"task.failed","name":"watch","plan":"triage"}`))
	if err != nil {
		t.Fatalf("Invoke create: %v", err)
	}
	if res.IsError {
		t.Fatalf("create failed: %+v", res)
	}
	if h.gotAdd.Name != "watch" || h.gotAdd.Plan != "triage" || len(h.gotAdd.Triggers) != 1 || h.gotAdd.Triggers[0].Subject != "task.failed" {
		t.Fatalf("gotAdd = %+v", h.gotAdd)
	}
	if !strings.Contains(res.Output, `"standing order created"`) {
		t.Fatalf("output missing message: %s", res.Output)
	}

	h.addErr = errors.New("add failed")
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"op":"create_event","subject":"task.failed","name":"watch","plan":"triage"}`))
	if err != nil || !res.IsError || !strings.Contains(res.Output, "add failed") {
		t.Fatalf("create host failure = %+v, %v", res, err)
	}
}

func TestStandingCoverageInvokeList(t *testing.T) {
	store, err := standing.Open(t.TempDir())
	if err != nil {
		t.Fatalf("standing.Open: %v", err)
	}
	h := &fakeHost{store: store}
	tool := New()
	tool.Bind(h)
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil || res.IsError {
		t.Fatalf("Invoke list = %+v, %v", res, err)
	}
	if !strings.Contains(res.Output, `"count": 0`) || !strings.Contains(res.Output, `"orders": []`) {
		t.Fatalf("empty list output = %s", res.Output)
	}
}
