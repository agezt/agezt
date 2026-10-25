// SPDX-License-Identifier: MIT

package skillapi_test

// Provenance: the contract's own tests for the W3.55 slice — the widths of
//             the ports, the exact signatures of the curator, the rules that
//             travel with the rows, and the sentinels the layers compare
//             against.
//
// The widths are checked STATICALLY, on the interface types: a narrowed port
// would still answer the read side, so only the declared method set can tell
// the two apart.

import (
	"context"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/skillapi"
)

// TestTheCuratorPortIsExactlyTwentyVerbsWide: the curator is the module's
// WHOLE public surface as the layers above it see it — and no more.
//
// The width is a promise in both directions: too narrow and the layers above
// cannot do their work, too wide and the port grows verbs nobody calls (and
// every plugin fake has to implement).
func TestTheCuratorPortIsExactlyTwentyVerbsWide(t *testing.T) {
	got := reflect.TypeOf((*skillapi.Curator)(nil)).Elem().NumMethod()
	if got != 20 {
		t.Fatalf("the curator port has %d verbs, want 20", got)
	}
}

// TestTheNarrowPortsKeepTheirWidths: the curator is COMPOSED of narrow ports,
// each with a single meaning. Their widths are pinned too: a port that grows
// silently is how a five-verb seam becomes a twenty-verb one.
func TestTheNarrowPortsKeepTheirWidths(t *testing.T) {
	cases := []struct {
		name string
		port any
		want int
	}{
		{"forge", (*skillapi.Forge)(nil), 5},
		{"store", (*skillapi.Store)(nil), 5},
		{"publisher", (*skillapi.Publisher)(nil), 1},
		{"lifecycle", (*skillapi.Lifecycle)(nil), 4},
		{"activator", (*skillapi.Activator)(nil), 2},
		{"outcome", (*skillapi.Outcome)(nil), 1},
		{"proposer", (*skillapi.Proposer)(nil), 2},
		{"observer", (*skillapi.Observer)(nil), 2},
		{"bundler", (*skillapi.Bundler)(nil), 1},
		{"tuner", (*skillapi.Tuner)(nil), 3},
	}
	for _, tc := range cases {
		typ := reflect.TypeOf(tc.port).Elem()
		if got := typ.NumMethod(); got != tc.want {
			t.Errorf("the %s port has %d verbs, want %d", tc.name, got, tc.want)
		}
	}
}

// TestTheCuratorNamesOnlyContractRows: the curator's exact signatures. A port
// that answers a row the contract does not have is a port the contract layer
// cannot describe — and a shape the modules above would have to reach back
// into the implementation for.
func TestTheCuratorNamesOnlyContractRows(t *testing.T) {
	want := map[string]func(){
		"Create":              func() { var p skillapi.Curator; _, _, _ = p.Create("", skillapi.CreateSpec{}) },
		"Promote":             func() { var p skillapi.Curator; _, _ = p.Promote("", "") },
		"Quarantine":          func() { var p skillapi.Curator; _ = p.Quarantine("", "", "") },
		"Get":                 func() { var p skillapi.Curator; _, _, _ = p.Get("") },
		"List":                func() { var p skillapi.Curator; _, _ = p.List() },
		"Archive":             func() { var p skillapi.Curator; _ = p.Archive("", "", "") },
		"Revert":              func() { var p skillapi.Curator; _, _ = p.Revert("", "") },
		"RestoreStatus":       func() { var p skillapi.Curator; _, _, _ = p.RestoreStatus("", "", skillapi.StatusDraft, "") },
		"Reassign":            func() { var p skillapi.Curator; _, _, _ = p.Reassign("", "", "") },
		"ActivateFor":         func() { var p skillapi.Curator; _, _ = p.ActivateFor("", "", "", 0) },
		"ActivateExplicitFor": func() { var p skillapi.Curator; _, _, _ = p.ActivateExplicitFor("", "", "", nil, 0) },
		"RecordOutcome":       func() { var p skillapi.Curator; p.RecordOutcome("", nil, true) },
		"Propose":             func() { var p skillapi.Curator; _, _ = p.Propose(context.TODO(), "", nil, "", "", "") },
		"ShadowEvaluate":      func() { var p skillapi.Curator; _ = p.ShadowEvaluate(context.TODO(), "", nil, "", "", "", 0) },
		"Count":               func() { var p skillapi.Curator; _ = p.Count() },
		"Hygiene":             func() { var p skillapi.Curator; _, _ = p.Hygiene(0) },
		"Bundles":             func() { var p skillapi.Curator; _ = p.Bundles() },
		"SetAutoQuarantine":   func() { var p skillapi.Curator; p.SetAutoQuarantine(0, 0) },
		"SetAutoShadow":       func() { var p skillapi.Curator; p.SetAutoShadow(true) },
		"SetAutoPromote":      func() { var p skillapi.Curator; p.SetAutoPromote(0, 0) },
	}
	typ := reflect.TypeOf((*skillapi.Curator)(nil)).Elem()
	for name := range want {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the curator port is missing %s", name)
		}
	}
	if typ.NumMethod() != len(want) {
		t.Fatalf("the curator port has %d verbs, want %d", typ.NumMethod(), len(want))
	}
}

// TestThePublisherPortIsOneVerb: the publisher answers the event's ID and
// nothing else. The bus answers with the durable envelope, and the envelope
// carries the hash chain the contract layer may not name.
func TestThePublisherPortIsOneVerb(t *testing.T) {
	typ := reflect.TypeOf((*skillapi.Publisher)(nil)).Elem()
	if typ.NumMethod() != 1 {
		t.Fatalf("the publisher port has %d verbs, want 1", typ.NumMethod())
	}
	m, ok := typ.MethodByName("Publish")
	if !ok {
		t.Fatal("the publisher port has no Publish")
	}
	if got, want := m.Type.NumOut(), 2; got != want {
		t.Fatalf("Publish answers %d values, want %d (the id and the error)", got, want)
	}
	if got := m.Type.Out(0).String(); got != "string" {
		t.Fatalf("Publish answers %s, want the event's id (string)", got)
	}
}

// TestTheRetrieveRuleTravelsWithTheRows: the ranking rule lives with the rows
// it judges. It decides which skill is offered to a run — a rule that quietly
// returns nothing would leave every run without a skill and no structural
// check would see it.
func TestTheRetrieveRuleTravelsWithTheRows(t *testing.T) {
	active := skillapi.Skill{
		ID: "a1", Name: "diagnose-ci", Description: "diagnose failing CI",
		Triggers: []string{"ci"}, Body: "steps", Status: skillapi.StatusActive,
	}
	draft := skillapi.Skill{
		ID: "d1", Name: "diagnose-ci-draft", Description: "diagnose failing CI",
		Triggers: []string{"ci"}, Body: "steps", Status: skillapi.StatusDraft,
	}
	hits := skillapi.Retrieve([]skillapi.Skill{active, draft}, "failing ci", 5, 1_000)
	if len(hits) != 1 {
		t.Fatalf("only the ACTIVE skill may be retrieved, got %d hits", len(hits))
	}
	if hits[0].Skill.ID != "a1" {
		t.Fatalf("the retrieved skill is %q, want a1", hits[0].Skill.ID)
	}
	if hits[0].Score <= 0 {
		t.Fatalf("the retrieved skill scored %v, want a positive score", hits[0].Score)
	}
	// A non-matching intent retrieves nothing — an empty answer, not an error.
	if got := skillapi.Retrieve([]skillapi.Skill{active}, "unrelated topic", 5, 1_000); len(got) != 0 {
		t.Fatalf("a non-matching intent must retrieve nothing, got %d hits", len(got))
	}
	// The shadow pool is ranked by the same rule but is never injected.
	shadow := active
	shadow.ID = "s1"
	shadow.Status = skillapi.StatusShadow
	if got := skillapi.RetrieveShadow([]skillapi.Skill{shadow}, "failing ci", 5, 1_000); len(got) != 1 {
		t.Fatalf("the shadow pool must be ranked, got %d hits", len(got))
	}
	if got := skillapi.Retrieve([]skillapi.Skill{shadow}, "failing ci", 5, 1_000); len(got) != 0 {
		t.Fatalf("a shadow skill must never be injected, got %d hits", len(got))
	}
}

// TestTheDirectiveRuleTravelsWithTheRow: the directive parser decides what the
// run's intent really says once the control line is stripped. A parser that
// quietly keeps the control line would send "/skill x" to the model as a task.
func TestTheDirectiveRuleTravelsWithTheRow(t *testing.T) {
	d := skillapi.ParseActivationDirective("/skill diagnose-ci\nplease look at the logs")
	if !d.Explicit {
		t.Fatal("the directive must be explicit")
	}
	if len(d.Refs) != 1 || d.Refs[0] != "diagnose-ci" {
		t.Fatalf("the directive refs are %v, want [diagnose-ci]", d.Refs)
	}
	if d.CleanIntent != "please look at the logs" {
		t.Fatalf("the clean intent is %q", d.CleanIntent)
	}
	plain := skillapi.ParseActivationDirective("just a task")
	if plain.Explicit || plain.CleanIntent != "just a task" {
		t.Fatalf("a plain intent must stay plain, got %+v", plain)
	}
	// A bare directive with no task keeps the original input, so a run is not
	// created with an empty task.
	bare := skillapi.ParseActivationDirective("/skill diagnose-ci")
	if !bare.Explicit || bare.CleanIntent != "/skill diagnose-ci" {
		t.Fatalf("a bare directive must keep the original intent, got %+v", bare)
	}
}

// TestTheSentinelsAreTheContracts: the sentinels the layers compare against
// live in the contract, so `errors.Is` matches them wherever they are raised.
func TestTheSentinelsAreTheContracts(t *testing.T) {
	for name, err := range map[string]error{
		"ErrIllegalTransition": skillapi.ErrIllegalTransition,
		"ErrNotFound":          skillapi.ErrNotFound,
		"ErrEmptyBody":         skillapi.ErrEmptyBody,
	} {
		if err == nil {
			t.Errorf("%s is nil", name)
			continue
		}
		if err.Error() == "" {
			t.Errorf("%s has no message", name)
		}
	}
	// The three sentinels are distinct — a layer that compares against the
	// wrong one would misreport the reason.
	if skillapi.ErrIllegalTransition == skillapi.ErrNotFound ||
		skillapi.ErrNotFound == skillapi.ErrEmptyBody ||
		skillapi.ErrIllegalTransition == skillapi.ErrEmptyBody {
		t.Fatal("the skill sentinels must be distinct")
	}
}
