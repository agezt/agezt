// SPDX-License-Identifier: MIT

package reflection

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	kreflect "github.com/agezt/agezt/kernel/reflect"
)

type fakeEngine struct {
	report kreflect.Report
	err    error
	found  bool
	corrs  []string
	ctxErr error
}

func (f *fakeEngine) Reflect(ctx context.Context, corr string) (kreflect.Report, error) {
	f.corrs = append(f.corrs, corr)
	f.ctxErr = ctx.Err()
	return f.report, f.err
}

func (f *fakeEngine) Latest() (kreflect.Report, bool) { return f.report, f.found }

func encode(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestRun(t *testing.T) {
	f := &fakeEngine{report: kreflect.Report{GeneratedMS: 42, EntitiesDecayed: 3}}
	s := New(f, func() string { return "01ABC" })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, err := s.Run(ctx, RunRequest{})
	if err != nil || out.CorrelationID != "reflect-01ABC" || len(f.corrs) != 1 || f.corrs[0] != "reflect-01ABC" || f.ctxErr != nil {
		t.Fatal("the pass runs under a minted correlation, independent of the caller's context", out, err, f.corrs, f.ctxErr)
	}
	raw := encode(t, out)
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil || m["correlation_id"] != "reflect-01ABC" || m["generated_ms"] != float64(42) || m["entities_decayed"] != float64(3) {
		t.Fatal("the report's fields sit beside the correlation", raw)
	}
	if _, nested := m["Report"]; nested {
		t.Fatal("the report is not nested", raw)
	}
	direct, _ := json.Marshal(f.report)
	var want map[string]any
	_ = json.Unmarshal(direct, &want)
	want["correlation_id"] = "reflect-01ABC"
	if !reflect.DeepEqual(m, want) {
		t.Fatal("every report field is kept", raw, string(direct))
	}
	f.err = errors.New("world decay failed")
	if _, err := s.Run(context.Background(), RunRequest{}); err == nil || err.Error() != "world decay failed" {
		t.Fatal(err)
	}
}

func TestShow(t *testing.T) {
	f := &fakeEngine{}
	s := New(f, func() string { return "x" })
	if out, _ := s.Show(context.Background(), ShowRequest{}); encode(t, out) != `{"found":false}` {
		t.Fatal("no pass yet", encode(t, out))
	}
	f.found, f.report = true, kreflect.Report{GeneratedMS: 7}
	out, _ := s.Show(context.Background(), ShowRequest{})
	direct, _ := json.Marshal(f.report)
	if got := encode(t, out); got != `{"found":true,"report":`+string(direct)+`}` {
		t.Fatal(got)
	}
	if len(f.corrs) != 0 {
		t.Fatal("reading never runs a pass")
	}
}

func TestOperations(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("provider is required")
	}
	ops, err := Operations(func(context.Context) *Service { return nil })
	if err != nil || len(ops) != 2 {
		t.Fatal(len(ops), err)
	}
	for i, want := range []struct {
		name          string
		read          bool
		input, output reflect.Type
	}{
		{"reflect_run", false, reflect.TypeFor[RunRequest](), reflect.TypeFor[RunOutput]()},
		{"reflect_show", true, reflect.TypeFor[ShowRequest](), reflect.TypeFor[ShowOutput]()},
	} {
		s := ops[i].Spec()
		out, err := schema.FromType(want.output, false)
		if err != nil {
			t.Fatal(err)
		}
		if s.Name != want.name || s.ReadOnly != want.read || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamNone || !s.AllowUnknownInput || s.HTTP.Method != "" || s.HTTP.Path != "" || s.Input != want.input || s.Output != want.output || string(s.OutputSchema) != string(out) {
			t.Fatalf("%s spec: %+v", want.name, s)
		}
		if err := schema.ValidateJSON(s.InputSchema, json.RawMessage(`{"tenant":"t","x":1}`)); err != nil {
			t.Fatal(want.name, err)
		}
	}
}
