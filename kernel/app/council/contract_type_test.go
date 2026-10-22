// SPDX-License-Identifier: MIT

package council

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/types"
	"github.com/agezt/agezt/kernel/runtime"
)

// TestMemberIsTheContractType: the application's council row IS the contract
// type, not a copy of it. The host therefore hands its membership over without
// a field-by-field projection, and the two sides cannot drift apart.
func TestMemberIsTheContractType(t *testing.T) {
	if reflect.TypeOf(Member{}) != reflect.TypeOf(types.CouncilMember{}) {
		t.Fatalf("the council row drifted from the contract type: %v vs %v", reflect.TypeOf(Member{}), reflect.TypeOf(types.CouncilMember{}))
	}
	// A member is usable as the contract type without a conversion...
	var member types.CouncilMember = Member{Seat: "Chair", Model: "m1"}
	if member.Seat != "Chair" || member.Model != "m1" {
		t.Fatal(member)
	}
	// ...and so is a whole slice, which is what the host hands over.
	var members []Member = []types.CouncilMember{{Seat: "Chair", Model: "m1"}}
	if len(members) != 1 || members[0].Seat != "Chair" || members[0].Model != "m1" {
		t.Fatal(members)
	}
}

// TestAskMembersAreTheContractTypeWithoutAProjection: the answer's member
// list is the panel's own rows — the application does not rebuild them field
// by field — and it stays an empty array (never null) when the panel has no
// members.
func TestAskMembersAreTheContractTypeWithoutAProjection(t *testing.T) {
	engines := &askEngines{council: runtime.CouncilResult{
		Question: "why?",
		Members:  []runtime.CouncilMember{{Seat: "Chair", Model: "m1"}, {Seat: "Scribe", Model: "m2"}},
	}}
	out, err := engines.engines().Ask(context.Background(), AskRequest{Question: json.RawMessage(`"why?"`)})
	if err != nil {
		t.Fatal(err)
	}
	want := []Member{{Seat: "Chair", Model: "m1"}, {Seat: "Scribe", Model: "m2"}}
	if !reflect.DeepEqual(out.Members, want) {
		t.Fatal("the answer's members are not the panel's rows", out.Members)
	}
	raw, err := json.Marshal(out.Members)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `[{"seat":"Chair","model":"m1"},{"seat":"Scribe","model":"m2"}]` {
		t.Fatal(string(raw))
	}
	empty := &askEngines{council: runtime.CouncilResult{Question: "why?"}}
	emptyOut, err := empty.engines().Ask(context.Background(), AskRequest{Question: json.RawMessage(`"why?"`)})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(emptyOut.Members)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "[]" {
		t.Fatal("an empty panel is an empty array, never null", string(raw))
	}
}

// TestOpinionRowIsTheContractType: the answer's opinion row IS the contract
// type, not a copy of it. The host's opinions therefore reach the operator
// without a field-by-field projection.
func TestOpinionRowIsTheContractType(t *testing.T) {
	if reflect.TypeOf(OpinionRow{}) != reflect.TypeOf(types.Opinion{}) {
		t.Fatalf("the opinion row drifted from the contract type: %v vs %v", reflect.TypeOf(OpinionRow{}), reflect.TypeOf(types.Opinion{}))
	}
	if reflect.TypeOf(runtime.Opinion{}) != reflect.TypeOf(types.Opinion{}) {
		t.Fatalf("the runtime's opinion drifted from the contract type: %v vs %v", reflect.TypeOf(runtime.Opinion{}), reflect.TypeOf(types.Opinion{}))
	}
	// The runtime's opinion and the application's row are the same type, so a
	// slice of one is a slice of the other.
	var opinions []OpinionRow = []types.Opinion{{Seat: "Chair", Model: "m1", Round: 1, Text: "yes"}}
	if len(opinions) != 1 || opinions[0].Seat != "Chair" {
		t.Fatal(opinions)
	}
}

// TestAskOpinionsAreTheContractTypeWithoutAProjection: the answer's opinion
// list is the panel's own rows — the application does not rebuild them field
// by field — and it stays an empty array (never null) when the panel said
// nothing.
func TestAskOpinionsAreTheContractTypeWithoutAProjection(t *testing.T) {
	engines := &askEngines{council: runtime.CouncilResult{
		Question: "why?",
		Opinions: []runtime.Opinion{
			{Seat: "Chair", Model: "m1", Round: 1, Text: "yes"},
			{Seat: "Scribe", Model: "m2", Round: 1, Error: "provider failed"},
		},
	}}
	out, err := engines.engines().Ask(context.Background(), AskRequest{Question: json.RawMessage(`"why?"`)})
	if err != nil {
		t.Fatal(err)
	}
	want := []runtime.Opinion{
		{Seat: "Chair", Model: "m1", Round: 1, Text: "yes"},
		{Seat: "Scribe", Model: "m2", Round: 1, Error: "provider failed"},
	}
	if !reflect.DeepEqual(out.Opinions, want) {
		t.Fatal("the answer's opinions are not the panel's rows", out.Opinions)
	}
	raw, err := json.Marshal(out.Opinions)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `[{"seat":"Chair","model":"m1","round":1,"text":"yes"},{"seat":"Scribe","model":"m2","round":1,"text":"","error":"provider failed"}]` {
		t.Fatal(string(raw))
	}
	empty := &askEngines{council: runtime.CouncilResult{Question: "why?"}}
	emptyOut, err := empty.engines().Ask(context.Background(), AskRequest{Question: json.RawMessage(`"why?"`)})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(emptyOut.Opinions)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "[]" {
		t.Fatal("an empty deliberation is an empty array, never null", string(raw))
	}
}
