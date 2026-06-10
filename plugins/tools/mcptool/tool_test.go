// SPDX-License-Identifier: MIT

package mcptool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/mcp"
)

// fakeKernel backs the tool with a real registry store and a canned
// attachment table — the kernel's journaling/spawning is covered by runtime
// tests.
type fakeKernel struct {
	store    *mcp.Store
	attached map[string]int
}

func (f *fakeKernel) AddMCPServer(_ string, srv mcp.Server) (mcp.Server, error) {
	return f.store.Add(srv)
}

func (f *fakeKernel) AttachMCPServer(_ context.Context, _, ref string) (mcp.Server, []string, error) {
	srv, found := f.store.Get(ref)
	if !found {
		return mcp.Server{}, nil, mcp.ErrNotFound
	}
	f.attached[srv.Name] = 2
	return srv, []string{"mcp_" + srv.Name + "_greet", "mcp_" + srv.Name + "_read"}, nil
}

func (f *fakeKernel) DetachMCPServer(_, ref string) error {
	delete(f.attached, ref)
	return nil
}

func (f *fakeKernel) RemoveMCPServer(_, ref string) (bool, error) {
	_, ok, err := f.store.Remove(ref)
	return ok, err
}

func (f *fakeKernel) MCPStore() *mcp.Store        { return f.store }
func (f *fakeKernel) MCPAttached() map[string]int { return f.attached }

func newBound(t *testing.T) (*Tool, *fakeKernel) {
	t.Helper()
	store, err := mcp.OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	fk := &fakeKernel{store: store, attached: map[string]int{}}
	tool := New()
	tool.Bind(fk)
	return tool, fk
}

func invoke(t *testing.T, tool *Tool, in map[string]any) (string, bool) {
	t.Helper()
	raw, _ := json.Marshal(in)
	res, err := tool.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke(%v): %v", in, err)
	}
	return res.Output, res.IsError
}

// TestSelfInstallLoop: the agent's whole surface — add → attach (tool names
// come back) → list shows live status → detach → remove.
func TestSelfInstallLoop(t *testing.T) {
	tool, fk := newBound(t)

	out, isErr := invoke(t, tool, map[string]any{
		"op": "add", "name": "fake", "command": "python", "args": []string{"server.py"},
	})
	if isErr || !strings.Contains(out, "registered") {
		t.Fatalf("add: err=%v out=%s", isErr, out)
	}

	out, isErr = invoke(t, tool, map[string]any{"op": "attach", "ref": "fake"})
	if isErr || !strings.Contains(out, "mcp_fake_greet") {
		t.Fatalf("attach: err=%v out=%s", isErr, out)
	}

	out, _ = invoke(t, tool, map[string]any{"op": "list"})
	if !strings.Contains(out, `"attached": true`) || !strings.Contains(out, `"tool_count": 2`) {
		t.Fatalf("list: %s", out)
	}

	out, isErr = invoke(t, tool, map[string]any{"op": "detach", "ref": "fake"})
	if isErr || !strings.Contains(out, "detached") {
		t.Fatalf("detach: err=%v out=%s", isErr, out)
	}
	if len(fk.attached) != 0 {
		t.Fatal("detach did not clear the attachment")
	}

	out, isErr = invoke(t, tool, map[string]any{"op": "remove", "ref": "fake"})
	if isErr || !strings.Contains(out, "removed") {
		t.Fatalf("remove: err=%v out=%s", isErr, out)
	}
	if fk.store.Count() != 0 {
		t.Fatal("remove did not delete the registration")
	}
}

func TestErrors(t *testing.T) {
	tool, _ := newBound(t)
	if out, isErr := invoke(t, tool, map[string]any{"op": "attach", "ref": "ghost"}); !isErr || !strings.Contains(out, "not found") {
		t.Fatalf("ghost attach: err=%v out=%s", isErr, out)
	}
	if out, isErr := invoke(t, tool, map[string]any{"op": "add", "name": "Bad-Name", "command": "x"}); !isErr {
		t.Fatalf("bad name accepted: %s", out)
	}
	if out, isErr := invoke(t, New(), map[string]any{"op": "list"}); !isErr || !strings.Contains(out, "not available") {
		t.Fatalf("unbound: err=%v out=%s", isErr, out)
	}
}
