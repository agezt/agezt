// SPDX-License-Identifier: MIT

package builtintools

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/reasoningapi"
	"github.com/agezt/agezt/kernel/toolreg"
	conductortool "github.com/agezt/agezt/plugins/tools/conductor"
	counciltool "github.com/agezt/agezt/plugins/tools/council"
	"github.com/agezt/agezt/plugins/tools/research"
)

// fakeReasoning is the daemon's reasoning surface, faked: it answers all
// three verbs so it can be carried by the registry's single reasoning row.
//
// It also RECORDS that it was called, because "the tool did not say it is
// unavailable" is not enough: a tool could answer "unavailable" for a
// different reason and still look wired.
type fakeReasoning struct {
	counciled    bool
	conducted    bool
	researched   bool
	lastQuestion string
}

func (f *fakeReasoning) Council(_ context.Context, _, question string, _ []reasoningapi.CouncilMember, _ int) (reasoningapi.CouncilResult, error) {
	f.counciled = true
	f.lastQuestion = question
	return reasoningapi.CouncilResult{
		Question:  question,
		Consensus: "the fake panel reached a consensus",
		AsOf:      "2026-10-24",
	}, nil
}

func (f *fakeReasoning) Conduct(_ context.Context, _ string, _ reasoningapi.ConductorConfig) (reasoningapi.ConductorResult, error) {
	f.conducted = true
	return reasoningapi.ConductorResult{}, nil
}

func (f *fakeReasoning) Research(_ context.Context, _, question string, _ reasoningapi.ResearchOptions) (reasoningapi.ResearchReport, error) {
	f.researched = true
	f.lastQuestion = question
	return reasoningapi.ResearchReport{Question: question}, nil
}

// TestTheReasoningSpecsBindThePort: all three reasoning tools drive the
// daemon's reasoning surface through the contract's port, and the registry's
// row carries that port (W3.39b) — not the live kernel.
//
// The fake records the call, so the test proves the tools REACHED the
// daemon's reasoning surface rather than merely that they did not complain.
func TestTheReasoningSpecsBindThePort(t *testing.T) {
	for _, tc := range []struct {
		name   string
		spec   func() toolreg.Spec
		input  string
		called func(*fakeReasoning) bool
	}{
		{"council", specCouncil, `{"question":"should we ship?"}`, func(f *fakeReasoning) bool { return f.counciled }},
		{"conductor", specConductor, `{"task":"ship the release"}`, func(f *fakeReasoning) bool { return f.conducted }},
		{"research", specResearch, `{"question":"should we ship?"}`, func(f *fakeReasoning) bool { return f.researched }},
	} {
		fake := &fakeReasoning{}
		spec := tc.spec()
		built, err := spec.Build(toolreg.BuildDeps{})
		if err != nil {
			t.Fatalf("%s Build: %v", tc.name, err)
		}
		if err := spec.Configure(built.Tool, toolreg.KernelDeps{Reasoning: fake}); err != nil {
			t.Fatalf("%s Configure: %v", tc.name, err)
		}
		res, err := built.Tool.Invoke(context.Background(), json.RawMessage(tc.input))
		if err != nil {
			t.Fatalf("%s Invoke: %v", tc.name, err)
		}
		if res.IsError {
			t.Fatalf("%s errored on a wired reasoning port: %s", tc.name, res.Output)
		}
		if !tc.called(fake) {
			t.Fatalf("%s must reach the daemon's reasoning surface", tc.name)
		}
	}
}

// TestTheToolsRunnersAreTheContractsPorts: the three tools' runner types
// ARE the contract's ports — the same type, not a copy with the same
// method set.
//
// An assignment would accept a copy (a copy has the same method set, so it
// satisfies the port), so the check is on the TYPE ITSELF: two interface
// types with identical method sets are still different types, and only an
// alias is the same type. A tool that declared its own runner would keep
// working — and the contract's port would quietly stop being the thing the
// registry hands over.
func TestTheToolsRunnersAreTheContractsPorts(t *testing.T) {
	for _, tc := range []struct {
		name string
		tool reflect.Type
		want reflect.Type
	}{
		{"council", reflect.TypeOf((*counciltool.Runner)(nil)).Elem(), reflect.TypeOf((*reasoningapi.CouncilRunner)(nil)).Elem()},
		{"conductor", reflect.TypeOf((*conductortool.Runner)(nil)).Elem(), reflect.TypeOf((*reasoningapi.ConductorRunner)(nil)).Elem()},
		{"research", reflect.TypeOf((*research.Runner)(nil)).Elem(), reflect.TypeOf((*reasoningapi.ResearchRunner)(nil)).Elem()},
	} {
		if tc.tool != tc.want {
			t.Errorf("the %s tool's runner must BE the contract's port, got %v want %v", tc.name, tc.tool, tc.want)
		}
	}
}

// TestTheReasoningSpecsLeaveTheToolsUnwiredWithoutThePort: without the port
// the three tools must say they are unavailable — they must not call into
// nothing.
//
// The nil port is the trap the daemon's rule exists for: a port that wraps a
// nil implementation is NOT nil, so the tool would bind a runner that is not
// there and fail in a way the operator could not read.
func TestTheReasoningSpecsLeaveTheToolsUnwiredWithoutThePort(t *testing.T) {
	for _, tc := range []struct {
		name string
		spec func() toolreg.Spec
		want string
	}{
		{"council", specCouncil, "council unavailable"},
		{"conductor", specConductor, "conductor unavailable"},
		{"research", specResearch, "research unavailable"},
	} {
		spec := tc.spec()
		built, err := spec.Build(toolreg.BuildDeps{})
		if err != nil {
			t.Fatalf("%s Build: %v", tc.name, err)
		}
		if err := spec.Configure(built.Tool, toolreg.KernelDeps{}); err != nil {
			t.Fatalf("%s Configure: %v", tc.name, err)
		}
		res, err := built.Tool.Invoke(context.Background(), json.RawMessage(`{"question":"should we ship?"}`))
		if err != nil {
			t.Fatalf("%s Invoke: %v", tc.name, err)
		}
		if !res.IsError || !strings.Contains(res.Output, tc.want) {
			t.Fatalf("%s must report %q without the port, got %+v", tc.name, tc.want, res)
		}
	}
}
