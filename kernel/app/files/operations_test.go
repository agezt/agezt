// SPDX-License-Identifier: MIT

package files

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/platform/rollbackstore"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// fakeRunner records the governed call and either answers or invokes the
// adapter the service handed it.
type fakeRunner struct {
	corr, callID, tool, input string
	result                    toolapi.Result
	err                       error
	invoke                    bool
}

func (f *fakeRunner) RunToolWithLookup(ctx context.Context, corr, callID, tool string, raw json.RawMessage, lookup toolapi.ToolLookup) (toolapi.Result, error) {
	f.corr, f.callID, f.tool, f.input = corr, callID, tool, string(raw)
	if f.invoke {
		t, ok := lookup.LookupTool(tool)
		if !ok {
			return toolapi.Result{}, errors.New("adapter missing")
		}
		return t.Invoke(ctx, raw)
	}
	return f.result, f.err
}

func code(err error) string {
	var domain *Error
	if errors.As(err, &domain) {
		return domain.ErrorCode()
	}
	return ""
}

// The mutation's arguments are the operation's request row, read through the
// contract's Args. The leniency a mistyped argument used to meet here is the
// door's now: the declared schema refuses a wrong-typed field before the
// mutation is applied.
func TestMutateCarriesTheRowsFields(t *testing.T) {
	runner := &fakeRunner{result: toolapi.Result{Output: `{"ok":true,"path":""}`}}
	s := New(Ports{Runner: runner, Correlation: "c1", CallID: "req-1"})
	out, err := s.Mutate(context.Background(), Mkdir, opapi.Args(opapi.FileMkdirRequest{}))
	if err != nil || !out.OK || out.Path == nil || *out.Path != "" || out.From != nil || out.To != nil {
		t.Fatal(out, err)
	}
	if runner.corr != "c1" || runner.callID != "req-1" || runner.tool != Mkdir || runner.input != `{"parents":false,"path":""}` {
		t.Fatalf("%+v", runner)
	}
	for _, c := range []struct {
		op   string
		in   map[string]any
		want string
	}{
		{Rename, opapi.Args(opapi.FileRenameRequest{From: "a"}), `{"from":"a","to":""}`},
		{Delete, opapi.Args(opapi.FileDeleteRequest{Path: "d", Recursive: true}), `{"path":"d","recursive":true}`},
		{Delete, opapi.Args(opapi.FileDeleteRequest{}), `{"path":"","recursive":false}`},
	} {
		if _, err := s.Mutate(context.Background(), c.op, c.in); err != nil || runner.input != c.want {
			t.Fatal(c.op, runner.input, err)
		}
	}
	if _, err := s.Mutate(context.Background(), "file_chmod", map[string]any{}); code(err) != InvalidPath || err.Error() != `unknown file operation "file_chmod"` {
		t.Fatal(err)
	}
}

func TestMutateClassifiesFailures(t *testing.T) {
	for _, c := range []struct {
		runner *fakeRunner
		code   string
	}{
		{&fakeRunner{result: toolapi.Result{Output: "tool call denied by policy: no"}, err: errors.New("denied")}, Denied},
		{&fakeRunner{err: errors.New("invoker down")}, Unavailable},
		{&fakeRunner{err: &Error{Code: Symlink, Cause: errors.New("link")}}, Symlink},
		{&fakeRunner{result: toolapi.Result{Output: "disk full", IsError: true}}, IOFailure},
		{&fakeRunner{result: toolapi.Result{Output: "not json"}}, Unavailable},
	} {
		_, err := New(Ports{Runner: c.runner}).Mutate(context.Background(), Mkdir, map[string]any{})
		if code(err) != c.code {
			t.Fatalf("%+v: %v (%s)", c.runner, err, code(err))
		}
	}
}

func TestMutateRunsTheWorkspaceAdapter(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AGEZT_FILE_ROOT", root)
	s := New(Ports{Runner: &fakeRunner{invoke: true}})
	if out, err := s.Mutate(context.Background(), Mkdir, opapi.Args(opapi.FileMkdirRequest{Path: "a/b", Parents: true})); err != nil || *out.Path != "a/b" {
		t.Fatal(out, err)
	}
	out, err := s.Mutate(context.Background(), Rename, opapi.Args(opapi.FileRenameRequest{From: "a", To: "c"}))
	if raw, _ := json.Marshal(out); err != nil || string(raw) != `{"ok":true,"from":"a","to":"c"}` {
		t.Fatal(string(raw), err)
	}
	if _, err := s.Mutate(context.Background(), Delete, opapi.Args(opapi.FileDeleteRequest{Path: "missing"})); code(err) != NotFound {
		t.Fatal(err)
	}
	if _, err := s.Mutate(context.Background(), Rename, opapi.Args(opapi.FileRenameRequest{From: "c"})); code(err) != InvalidPath || err.Error() != "from and to are required" {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "c", "b")); err != nil {
		t.Fatal(err)
	}
}

func TestRestore(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "checkpoints.json")
	target := filepath.Join(dir, "target.txt")
	cp := rollbackstore.Checkpoint{ID: "c1", Kind: rollbackstore.KindFile, SubjectID: target, CreatedMS: 1, Before: map[string]any{"abs_path": target, "exists": true, "content_b64": base64.StdEncoding.EncodeToString([]byte("prior"))}}
	if err := rollbackstore.WriteAt(catalog, rollbackstore.Catalog{Checkpoints: []rollbackstore.Checkpoint{cp, {ID: "agent", Kind: "agent"}}}); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{invoke: true}
	s := New(Ports{Runner: runner, Correlation: "c", CallID: "r", CatalogPath: catalog})
	// The id is TEXT now: a number used to read as no id at all, and the door
	// refuses a mistyped id before the handler sees it.
	for id, want := range map[string]string{"": InvalidPath, "5": NotFound, " ": InvalidPath, "nope": NotFound, "agent": InvalidPath} {
		if _, err := s.Restore(context.Background(), opapi.FileRestoreRequest{ID: id}); code(err) != want {
			t.Fatal(id, err)
		}
	}
	out, err := s.Restore(context.Background(), opapi.FileRestoreRequest{ID: " c1 "})
	if err != nil || !out.Applied || out.Reason != "" || out.Checkpoint.AppliedMS <= 0 || out.Result["restored"] != "content" || runner.tool != RestoreOperation || runner.callID != "r" {
		t.Fatal(out, err)
	}
	if data, _ := os.ReadFile(target); string(data) != "prior" {
		t.Fatal(string(data))
	}
	again, err := s.Restore(context.Background(), opapi.FileRestoreRequest{ID: "c1"})
	if err != nil || again.Applied || again.Reason != "already applied" || again.Result != nil {
		t.Fatal(again, err)
	}
	raw, _ := json.Marshal(again)
	if !strings.HasPrefix(string(raw), `{"checkpoint":{"id":"c1","kind":"file.snapshot",`) || !strings.HasSuffix(string(raw), `},"applied":false,"reason":"already applied"}`) {
		t.Fatal(string(raw))
	}
	if err := os.WriteFile(catalog, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Restore(context.Background(), opapi.FileRestoreRequest{ID: "c1"}); code(err) != IOFailure {
		t.Fatal(err)
	}
}

func TestClassified(t *testing.T) {
	if classified(nil) != nil {
		t.Fatal("nil stays nil")
	}
	domain := &Error{Code: Denied, Cause: errors.New("no")}
	if classified(domain) != domain {
		t.Fatal("a classified error passes through")
	}
	if err := classified(errors.New("boom")); code(err) != Unavailable || err.Error() != "boom" {
		t.Fatal(err)
	}
}

func TestOperations(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("provider is required")
	}
	ops, err := Operations(func(context.Context) *Service { return nil })
	if err != nil || len(ops) != 4 {
		t.Fatal(len(ops), err)
	}
	for i, want := range []struct {
		name, props   string
		input, output reflect.Type
	}{
		{Mkdir, `{"parents":{"type":"boolean"},"path":{"type":"string"}}`, reflect.TypeFor[opapi.FileMkdirRequest](), reflect.TypeFor[MutationOutput]()},
		{Rename, `{"from":{"type":"string"},"to":{"type":"string"}}`, reflect.TypeFor[opapi.FileRenameRequest](), reflect.TypeFor[MutationOutput]()},
		{Delete, `{"path":{"type":"string"},"recursive":{"type":"boolean"}}`, reflect.TypeFor[opapi.FileDeleteRequest](), reflect.TypeFor[MutationOutput]()},
		{RestoreOperation, `{"id":{"type":"string"}}`, reflect.TypeFor[opapi.FileRestoreRequest](), reflect.TypeFor[RestoreOutput]()},
	} {
		s := ops[i].Spec()
		out, err := schema.FromType(want.output, false)
		if err != nil {
			t.Fatal(err)
		}
		if s.Name != want.name || s.ReadOnly || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamNone || !s.AllowUnknownInput || s.Input != want.input || s.Output != want.output || string(s.OutputSchema) != string(out) {
			t.Fatalf("%s spec: %+v", want.name, s)
		}
		if !strings.Contains(string(s.InputSchema), `"properties":`+want.props) {
			t.Fatal(want.name, string(s.InputSchema))
		}
		if err := schema.ValidateJSON(s.InputSchema, json.RawMessage(`{"tenant":"t","path":"p"}`)); err != nil {
			t.Fatal(want.name, err)
		}
	}
	// The typed door owns the type error now: a wrong-typed argument is
	// refused before the mutation is applied.
	schemas := map[string]json.RawMessage{}
	for _, op := range ops {
		schemas[op.Spec().Name] = op.Spec().InputSchema
	}
	for _, tc := range []struct{ name, in, want string }{
		{Mkdir, `{"path":5}`, "$.path has wrong type: got number, want string"},
		{Mkdir, `{"parents":"yes"}`, "$.parents has wrong type: got string, want boolean"},
		{Rename, `{"from":5}`, "$.from has wrong type: got number, want string"},
		{Delete, `{"recursive":"yes"}`, "$.recursive has wrong type: got string, want boolean"},
		{RestoreOperation, `{"id":5}`, "$.id has wrong type: got number, want string"},
	} {
		if err := schema.ValidateJSON(schemas[tc.name], json.RawMessage(tc.in)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatal(tc.name, tc.in, err)
		}
	}
}
