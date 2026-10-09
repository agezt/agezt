// SPDX-License-Identifier: MIT

package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

func write(t *testing.T, path string, data string, mod time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func fileRequest(t *testing.T, raw string) FileRequest {
	t.Helper()
	var in FileRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(err)
	}
	return in
}

func deleteRequest(t *testing.T, raw string) DeleteRequest {
	t.Helper()
	var in DeleteRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(err)
	}
	return in
}

func TestList(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "projects")
	out, err := New(root).List(ctx, ListRequest{})
	if raw, _ := json.Marshal(out); err != nil || string(raw) != `{"projects":[],"count":0}` {
		t.Fatal("no projects directory is an empty list", string(raw), err)
	}
	old, newer := time.Unix(1_700_000_000, 0), time.Unix(1_800_000_000, 0)
	write(t, filepath.Join(root, "calc", "z.py"), "zz", old)
	write(t, filepath.Join(root, "calc", "src", "a.py"), "a", old)
	write(t, filepath.Join(root, "web", "index.html"), "<h1>", newer)
	write(t, filepath.Join(root, "loose.txt"), "not a project", newer)
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err = New(root).List(ctx, ListRequest{})
	if err != nil || out.Count != 3 {
		t.Fatal(out, err)
	}
	want := []Project{
		{Name: "web", Files: []FileRow{{Name: "index.html", Bytes: 4}}, FileCount: 1, TotalBytes: 4, ModifiedUnix: newer.Unix()},
		{Name: "calc", Files: []FileRow{{Name: "src/a.py", Bytes: 1}, {Name: "z.py", Bytes: 2}}, FileCount: 2, TotalBytes: 3, ModifiedUnix: old.Unix()},
		{Name: "empty", Files: []FileRow{}, FileCount: 0, TotalBytes: 0, ModifiedUnix: 0},
	}
	if !reflect.DeepEqual(out.Projects, want) {
		t.Fatalf("directories only, newest first, files in slash-name order: %+v", out.Projects)
	}
	raw, _ := json.Marshal(out.Projects[2])
	if !strings.Contains(string(raw), `"files":[]`) {
		t.Fatal("an empty project lists no files as an array", string(raw))
	}
}

func TestListCaps(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	mod := time.Unix(1_700_000_000, 0)
	for i := range MaxFilesPerProject + 5 {
		write(t, filepath.Join(root, "big", fmt.Sprintf("f%04d", i)), "x", mod)
	}
	out, _ := New(root).List(context.Background(), ListRequest{})
	if out.Projects[0].FileCount != MaxFilesPerProject || out.Projects[0].TotalBytes != MaxFilesPerProject {
		t.Fatal("a project lists at most MaxFilesPerProject files", out.Projects[0].FileCount)
	}
	many := filepath.Join(t.TempDir(), "projects")
	for i := range MaxProjects + 3 {
		if err := os.MkdirAll(filepath.Join(many, fmt.Sprintf("p%04d", i)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	out, _ = New(many).List(context.Background(), ListRequest{})
	if out.Count != MaxProjects || len(out.Projects) != MaxProjects || out.Projects[MaxProjects-1].Name != fmt.Sprintf("p%04d", MaxProjects-1) {
		t.Fatal("at most MaxProjects projects are listed, in directory order before sorting", out.Count)
	}
}

func TestFile(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "projects")
	mod := time.Unix(1_700_000_000, 0)
	write(t, filepath.Join(root, "calc", "src", "add.py"), "def add(a, b): return a + b", mod)
	write(t, filepath.Join(root, "calc", "big.bin"), strings.Repeat("x", MaxFileBytes+10), mod)
	svc := New(root)
	for raw, want := range map[string]string{
		`{}`:                                            "args.project required",
		`{"project":"  ","file":"a"}`:                   "args.project required",
		`{"project":3,"file":"a"}`:                      "args.project must be a string",
		`{"project":null,"file":"a"}`:                   "args.project must be a string",
		`{"project":"calc"}`:                            "args.file required",
		`{"project":"calc","file":7}`:                   "args.file must be a string",
		`{"project":"../x","file":"a"}`:                 "illegal project name",
		`{"project":"calc","file":"../../etc"}`:         "illegal file path",
		`{"project":"calc","file":"src"}`:               "no such file",
		`{"project":"calc","file":"../web/index.html"}`: "illegal file path",
	} {
		if _, err := svc.File(ctx, fileRequest(t, raw)); err == nil || err.Error() != want {
			t.Fatal(raw, err)
		}
	}
	if _, err := svc.File(ctx, fileRequest(t, `{"project":"calc","file":"ghost.py"}`)); err == nil || !strings.HasPrefix(err.Error(), "resolve sandbox path: ") {
		t.Fatal("a missing file fails link resolution", err)
	}
	// A backslash separates only on Windows; elsewhere it is part of a name.
	nested := `src/add.py`
	if runtime.GOOS == "windows" {
		nested = `src\\add.py`
	}
	out, err := svc.File(ctx, fileRequest(t, `{"project":" calc","file":"`+nested+`"}`))
	if err != nil || out != (FileOutput{Project: " calc", File: "src/add.py", Bytes: 27, Content: "def add(a, b): return a + b"}) {
		t.Fatalf("the project echoes untrimmed and the file in slash form: %+v %v", out, err)
	}
	big, err := svc.File(ctx, fileRequest(t, `{"project":"calc","file":"big.bin"}`))
	if err != nil || !big.Truncated || len(big.Content) != MaxFileBytes || big.Bytes != MaxFileBytes+10 {
		t.Fatal("a large file is capped but reports its full size", big.Truncated, len(big.Content), big.Bytes, err)
	}
	outside := filepath.Join(filepath.Dir(root), "secret.txt")
	write(t, outside, "TOPSECRET", mod)
	if err := os.Symlink(outside, filepath.Join(root, "calc", "leak.txt")); err == nil {
		if out, err := svc.File(ctx, fileRequest(t, `{"project":"calc","file":"leak.txt"}`)); err == nil || !strings.HasPrefix(err.Error(), "sandbox escape: resolved path ") || strings.Contains(out.Content, "TOPSECRET") {
			t.Fatal("a planted link fails closed", out, err)
		}
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "projects")
	mod := time.Unix(1_700_000_000, 0)
	write(t, filepath.Join(root, "calc", "src", "add.py"), "x", mod)
	write(t, filepath.Join(root, "file.txt"), "x", mod)
	svc := New(root)
	for raw, want := range map[string]string{
		`{}`:                     "args.project required",
		`{"project":false}`:      "args.project must be a string",
		`{"project":"../calc"}`:  "illegal project name",
		`{"project":"calc/src"}`: "illegal project name",
		`{"project":"."}`:        "illegal project name",
		`{"project":"ghost"}`:    "no such project",
		`{"project":"file.txt"}`: "no such project",
	} {
		if _, err := svc.Delete(ctx, deleteRequest(t, raw)); err == nil || err.Error() != want {
			t.Fatal(raw, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "calc", "src", "add.py")); err != nil {
		t.Fatal("rejected deletes change nothing", err)
	}
	out, err := svc.Delete(ctx, deleteRequest(t, `{"project":" calc "}`))
	if err != nil || out.Deleted != " calc " {
		t.Fatal("the project echoes untrimmed", out, err)
	}
	if _, err := os.Stat(filepath.Join(root, "calc")); !os.IsNotExist(err) {
		t.Fatal("the whole project is removed", err)
	}
	if _, err := os.Stat(filepath.Join(root, "file.txt")); err != nil {
		t.Fatal("siblings stay", err)
	}
}

func TestOperations(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("nil provider")
	}
	root := filepath.Join(t.TempDir(), "projects")
	write(t, filepath.Join(root, "calc", "a.py"), "x", time.Unix(1_700_000_000, 0))
	svc := New(root)
	ops, err := Operations(func(context.Context) *Service { return svc })
	if err != nil || len(ops) != 3 {
		t.Fatal(ops, err)
	}
	for i, want := range []struct {
		name, method, path string
		readOnly           bool
	}{{"sandbox_list", "GET", "/api/sandbox", true}, {"sandbox_file", "GET", "/api/sandbox_file", true}, {"sandbox_delete", "POST", "/api/sandbox/delete", false}} {
		spec := ops[i].Spec()
		if spec.Name != want.name || spec.ReadOnly != want.readOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput || spec.HTTP != (opapi.HTTP{Method: want.method, Path: want.path}) {
			t.Fatal(spec)
		}
	}
	list, _ := svc.List(context.Background(), ListRequest{})
	file, _ := svc.File(context.Background(), fileRequest(t, `{"project":"calc","file":"a.py"}`))
	for i, out := range []any{list, file, DeleteOutput{Deleted: "calc"}} {
		raw, _ := json.Marshal(out)
		if err := schema.ValidateJSON(ops[i].Spec().OutputSchema, raw); err != nil {
			t.Fatal(i, err, string(raw))
		}
	}
}
