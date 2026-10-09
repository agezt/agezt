// SPDX-License-Identifier: MIT

package system

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

type fakeKernel struct {
	calls  []string
	verify error
}

func (f *fakeKernel) HaltWith(r string)   { f.calls = append(f.calls, "halt:"+r) }
func (f *fakeKernel) ResumeWith(r string) { f.calls = append(f.calls, "resume:"+r) }
func (f *fakeKernel) Verify() error       { f.calls = append(f.calls, "verify"); return f.verify }

func reasonRequest(t *testing.T, raw string) ReasonRequest {
	t.Helper()
	var in ReasonRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(err)
	}
	return in
}

func TestHaltResume(t *testing.T) {
	ctx := context.Background()
	k := &fakeKernel{}
	l := NewLifecycle(k, nil)
	if out, err := l.Halt(ctx, reasonRequest(t, `{"reason":" maintenance "}`)); err != nil || out != (GateOutput{OK: true, Halted: true, Reason: " maintenance "}) {
		t.Fatal(out, err)
	}
	if out, err := l.Resume(ctx, reasonRequest(t, `{}`)); err != nil || out != (GateOutput{OK: true}) {
		t.Fatal(out, err)
	}
	for _, raw := range []string{`{"reason":3}`, `{"reason":null}`, `{"reason":["x"]}`} {
		if _, err := l.Halt(ctx, reasonRequest(t, raw)); err == nil || err.Error() != "args.reason must be a string" {
			t.Fatal(raw, err)
		}
		if _, err := l.Resume(ctx, reasonRequest(t, raw)); err == nil || err.Error() != "args.reason must be a string" {
			t.Fatal(raw, err)
		}
	}
	if !reflect.DeepEqual(k.calls, []string{"halt: maintenance ", "resume:"}) {
		t.Fatal("the reason passes untrimmed and a bad one changes nothing", k.calls)
	}
}

func TestVerifyShutdown(t *testing.T) {
	ctx := context.Background()
	scheduled := 0
	l := NewLifecycle(&fakeKernel{}, func() { scheduled++ })
	if out, err := l.Verify(ctx, EmptyRequest{}); err != nil || out != (OKOutput{OK: true}) {
		t.Fatal(out, err)
	}
	boom := errors.New("journal: hash chain broken at seq 7")
	if _, err := NewLifecycle(&fakeKernel{verify: boom}, nil).Verify(ctx, EmptyRequest{}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if out, err := l.Shutdown(ctx, EmptyRequest{}); err != nil || out != (OKOutput{OK: true}) || scheduled != 1 {
		t.Fatal("shutdown acknowledges and schedules the exit once", out, err, scheduled)
	}
}

func TestLifecycleOperations(t *testing.T) {
	if _, err := LifecycleOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	ops, err := LifecycleOperations(func(context.Context) *Lifecycle { return NewLifecycle(&fakeKernel{}, func() {}) })
	if err != nil || len(ops) != 4 {
		t.Fatal(ops, err)
	}
	for i, w := range []struct {
		name     string
		readOnly bool
		http     opapi.HTTP
		out      any
	}{
		{"halt", false, opapi.HTTP{Method: "POST", Path: "/api/halt"}, GateOutput{OK: true, Halted: true}},
		{"resume", false, opapi.HTTP{Method: "POST", Path: "/api/resume"}, GateOutput{OK: true}},
		{"journal_verify", true, opapi.HTTP{}, OKOutput{OK: true}},
		{"shutdown", false, opapi.HTTP{}, OKOutput{OK: true}},
	} {
		spec := ops[i].Spec()
		if spec.Name != w.name || spec.ReadOnly != w.readOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput || spec.HTTP != w.http || spec.Output != reflect.TypeOf(w.out) {
			t.Fatal(spec)
		}
		raw, _ := json.Marshal(w.out)
		if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
			t.Fatal(w.name, err, string(raw))
		}
	}
}
