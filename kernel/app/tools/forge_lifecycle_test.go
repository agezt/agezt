// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/toolforge"
	"reflect"
	"testing"
	"time"
)

type forgeWriterProbe struct {
	st, received, edited              toolforge.ScriptTool
	calls                             []string
	corr, ref, sample, reason, output string
	ctx                               context.Context
	found, removed                    bool
	err                               error
}

func (p *forgeWriterProbe) note(cmd, corr, ref string) {
	p.calls = append(p.calls, cmd)
	p.corr = corr
	p.ref = ref
}
func (p *forgeWriterProbe) DraftScriptTool(corr string, st toolforge.ScriptTool) (toolforge.ScriptTool, error) {
	p.note("draft", corr, "")
	p.received = st
	return p.st, p.err
}
func (p *forgeWriterProbe) UpdateScriptTool(corr, ref string, mutate func(*toolforge.ScriptTool)) (toolforge.ScriptTool, bool, error) {
	p.note("edit", corr, ref)
	p.edited = p.st
	mutate(&p.edited)
	return p.st, p.found, p.err
}
func (p *forgeWriterProbe) TestScriptTool(ctx context.Context, corr, ref, input string) (toolforge.ScriptTool, string, error) {
	p.note("test", corr, ref)
	p.ctx = ctx
	p.sample = input
	return p.st, p.output, p.err
}
func (p *forgeWriterProbe) PromoteScriptTool(corr, ref string) (toolforge.ScriptTool, error) {
	p.note("promote", corr, ref)
	return p.st, p.err
}
func (p *forgeWriterProbe) QuarantineScriptTool(corr, ref, reason string) (toolforge.ScriptTool, error) {
	p.note("quarantine", corr, ref)
	p.reason = reason
	return p.st, p.err
}
func (p *forgeWriterProbe) RemoveScriptTool(corr, ref string) (bool, error) {
	p.note("remove", corr, ref)
	return p.removed, p.err
}
func forgeLifeCalls(s *ForgeLifecycle, ctx context.Context) []func() (any, error) {
	return []func() (any, error){
		func() (any, error) {
			return s.Draft(ctx, ForgeDraftInput{Tool: toolforge.ScriptTool{Name: " raw ", Code: "body", ID: "untrusted"}, CorrelationID: "explicit"})
		},
		func() (any, error) { return s.Edit(ctx, ForgeEditInput{Ref: " raw-ref ", CorrelationID: "explicit"}) },
		func() (any, error) {
			return s.Test(ctx, ForgeTestInput{Ref: " raw-ref ", Sample: " raw-input ", CorrelationID: "explicit"})
		},
		func() (any, error) { return s.Promote(ctx, ForgeRefInput{Ref: " raw-ref ", CorrelationID: "explicit"}) },
		func() (any, error) {
			return s.Quarantine(ctx, ForgeQuarantineInput{Ref: " raw-ref ", Reason: " raw-reason ", CorrelationID: "explicit"})
		},
		func() (any, error) { return s.Remove(ctx, ForgeRefInput{Ref: " raw-ref ", CorrelationID: "explicit"}) },
	}
}
func TestForgeLifecyclePortsIdentityContextAndTypedOutputs(t *testing.T) {
	for _, host := range []string{"", "host-owned"} {
		p := &forgeWriterProbe{st: toolforge.ScriptTool{ID: "id", Name: "named", Status: toolforge.StatusActive, TestedOK: true, CreatedMS: 9007199254740993, Code: "private"}, found: true, removed: true, output: " raw output ö \n"}
		s := NewForgeLifecycle(p)
		ctx, cancel := context.WithTimeout(opapi.WithCorrelation(context.Background(), host), time.Second)
		defer cancel()
		wantCorr := "explicit"
		if host != "" {
			wantCorr = host
		}
		for i, call := range forgeLifeCalls(s, ctx) {
			out, err := call()
			if err != nil {
				t.Fatal(i, err)
			}
			if len(p.calls) != i+1 || p.corr != wantCorr || (i > 0 && p.ref != " raw-ref ") {
				t.Fatal(i, p)
			}
			switch v := out.(type) {
			case ForgeMutationOutput:
				if v.Tool.ID != "id" || v.Tool.CallableAs != "forge_named" || v.Tool.CreatedMS != p.st.CreatedMS {
					t.Fatal(v)
				}
			case ForgeTestOutput:
				if !v.OK || v.Output != p.output || v.Tool.ID != "id" || p.ctx != ctx || p.sample != " raw-input " {
					t.Fatal(v, p)
				}
			case ForgeRemoveOutput:
				if !v.Removed {
					t.Fatal(v)
				}
			default:
				t.Fatal(out)
			}
		}
		if p.received.Name != " raw " || p.received.ID != "untrusted" || p.reason != " raw-reason " || p.received.Code != "body" {
			t.Fatal(p)
		}
		if !reflect.DeepEqual(p.calls, []string{"draft", "edit", "test", "promote", "quarantine", "remove"}) {
			t.Fatal(p.calls)
		}
	}
}
func TestForgeLifecycleEditOnlyFourNonemptyMutableFields(t *testing.T) {
	original := toolforge.ScriptTool{ID: "id", Name: "name", Description: "old desc", Language: "python", Code: "old code", InputSchema: "old schema", Status: toolforge.StatusActive, TestedOK: true, TestedMS: 9, CreatedMS: 1, UpdatedMS: 2}
	inputs := []toolforge.ScriptTool{{}, {Description: " \t ", Language: " \n ", InputSchema: " "}, {Description: " raw new desc ", Language: " raw lang ", Code: " ", InputSchema: " raw schema "}, {Description: "other", Language: "node", Code: "new code", InputSchema: "{}", ID: "poison", Name: "poison", Status: toolforge.StatusDraft, TestedOK: false, TestedMS: 100, CreatedMS: 100, UpdatedMS: 100}}
	wants := []toolforge.ScriptTool{original, original, original, original}
	wants[2].Description = inputs[2].Description
	wants[2].Language = inputs[2].Language
	wants[2].Code = inputs[2].Code
	wants[2].InputSchema = inputs[2].InputSchema
	wants[3].Description = "other"
	wants[3].Language = "node"
	wants[3].Code = "new code"
	wants[3].InputSchema = "{}"
	for i, in := range inputs {
		p := &forgeWriterProbe{st: original, found: true}
		if _, err := NewForgeLifecycle(p).Edit(context.Background(), ForgeEditInput{Ref: "id", Tool: in}); err != nil || p.edited != wants[i] || p.st != original {
			t.Fatal(i, err, p.edited, wants[i])
		}
	}
}
func TestForgeLifecycleCausePrecedenceUnknownRawRefAndZeroOnFailure(t *testing.T) {
	cause := errors.New("owned failure")
	for _, missing := range []bool{false, true} {
		p := &forgeWriterProbe{st: toolforge.ScriptTool{ID: "leaked"}, found: !missing, removed: true, output: "leaked", err: cause}
		for i, call := range forgeLifeCalls(NewForgeLifecycle(p), context.Background()) {
			out, err := call()
			if err != cause {
				t.Fatal(i, err)
			}
			switch v := out.(type) {
			case ForgeMutationOutput:
				if v != (ForgeMutationOutput{}) {
					t.Fatal(v)
				}
			case ForgeTestOutput:
				if v != (ForgeTestOutput{}) {
					t.Fatal(v)
				}
			case ForgeRemoveOutput:
				if v != (ForgeRemoveOutput{}) {
					t.Fatal(v)
				}
			}
		}
	}
	p := &forgeWriterProbe{err: fmt.Errorf("wrapped: %w", toolforge.ErrNotFound)}
	calls := forgeLifeCalls(NewForgeLifecycle(p), context.Background())
	for _, i := range []int{2, 3, 4} {
		out, err := calls[i]()
		if err == nil || err.Error() != "unknown script tool:  raw-ref " {
			t.Fatal(i, out, err)
		}
	}
	// Draft and remove retain backend causes, including not-found; edit uses the found flag.
	if _, err := calls[0](); !errors.Is(err, toolforge.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := calls[5](); !errors.Is(err, toolforge.ErrNotFound) {
		t.Fatal(err)
	}
	p.err = nil
	p.found = false
	if out, err := calls[1](); err == nil || err.Error() != "unknown script tool:  raw-ref " || out != (ForgeMutationOutput{}) {
		t.Fatal(out, err)
	}
	p.removed = false
	if out, err := calls[5](); err != nil || out != (ForgeRemoveOutput{}) {
		t.Fatal(out, err)
	}
}
func TestForgeLifecycleCanceledContextNeverEntersWriter(t *testing.T) {
	p := &forgeWriterProbe{found: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for i, call := range forgeLifeCalls(NewForgeLifecycle(p), ctx) {
		if _, err := call(); err != context.Canceled {
			t.Fatal(i, err)
		}
	}
	if len(p.calls) != 0 {
		t.Fatal(p.calls)
	}
}
func TestForgeLifecycleTestUsesPersistedVerdictAndEmptyOutput(t *testing.T) {
	p := &forgeWriterProbe{st: toolforge.ScriptTool{TestedOK: false}, output: ""}
	out, err := NewForgeLifecycle(p).Test(context.Background(), ForgeTestInput{})
	if err != nil || out.OK || out.Output != "" {
		t.Fatal(out, err)
	}
	m := forgeJSON(t, out)
	if m["ok"] != false || m["output"] != "" {
		t.Fatal(m)
	}
	if _, ok := m["tool"]; !ok {
		t.Fatal(m)
	}
}

func TestForgeLifecycleRemoveFalseRemainsRequiredOnWire(t *testing.T) {
	out, err := NewForgeLifecycle(&forgeWriterProbe{removed: false}).Remove(context.Background(), ForgeRefInput{})
	m := forgeJSON(t, out)
	if err != nil || len(m) != 1 || m["removed"] != false {
		t.Fatal(out, m, err)
	}
}
