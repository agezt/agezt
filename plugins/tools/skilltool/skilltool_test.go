// SPDX-License-Identifier: MIT

package skilltool

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/skill"
)

// fakeForge is an in-memory Forge stand-in: enough of the lifecycle to assert the
// tool's op → Forge-method mapping without a real store + bus.
type fakeForge struct {
	byID     map[string]skill.Skill
	lastCorr string
	created  []skill.CreateSpec
	promoted string
	retired  string
	retErr   error
	bundles  *skill.BundleStore
}

func newFake() *fakeForge { return &fakeForge{byID: map[string]skill.Skill{}} }

func (f *fakeForge) Create(corr string, spec skill.CreateSpec) (skill.Skill, bool, error) {
	if f.retErr != nil {
		return skill.Skill{}, false, f.retErr
	}
	f.lastCorr = corr
	f.created = append(f.created, spec)
	id := skill.ContentID(spec.Name, spec.Body)
	if existing, ok := f.byID[id]; ok {
		return existing, false, nil
	}
	sk := skill.Skill{ID: id, Name: spec.Name, Description: spec.Description, Body: spec.Body, Triggers: spec.Triggers, Version: skill.DefaultVersion, Status: skill.StatusDraft}
	f.byID[id] = sk
	return sk, true, nil
}

func (f *fakeForge) Promote(corr, id string) (skill.Status, error) {
	f.lastCorr, f.promoted = corr, id
	sk := f.byID[id]
	sk.Status = skill.StatusShadow
	f.byID[id] = sk
	return skill.StatusShadow, nil
}

func (f *fakeForge) Quarantine(corr, id, reason string) error {
	f.lastCorr, f.retired = corr, id
	sk := f.byID[id]
	sk.Status = skill.StatusQuarantined
	f.byID[id] = sk
	return nil
}

func (f *fakeForge) Get(id string) (skill.Skill, bool, error) {
	sk, ok := f.byID[id]
	return sk, ok, nil
}

func (f *fakeForge) List() ([]skill.Skill, error) {
	out := make([]skill.Skill, 0, len(f.byID))
	for _, sk := range f.byID {
		out = append(out, sk)
	}
	return out, nil
}

func (f *fakeForge) Bundles() *skill.BundleStore { return f.bundles }

func newTool(f forge) *Tool {
	t := New()
	t.Bind(f)
	return t
}

func invoke(t *testing.T, tool *Tool, in map[string]any) (map[string]any, bool) {
	t.Helper()
	return invokeCtx(t, context.Background(), tool, in)
}

func invokeCtx(t *testing.T, ctx context.Context, tool *Tool, in map[string]any) (map[string]any, bool) {
	t.Helper()
	raw, _ := json.Marshal(in)
	res, err := tool.Invoke(ctx, raw)
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}
	var out map[string]any
	_ = json.Unmarshal([]byte(res.Output), &out)
	return out, res.IsError
}

func TestDefinitionValid(t *testing.T) {
	d := New().Definition()
	if d.Name != "skill" || !json.Valid(d.InputSchema) {
		t.Fatalf("bad definition: %+v", d)
	}
}

func TestLearn_AuthorsADraft(t *testing.T) {
	f := newFake()
	out, isErr := invoke(t, newTool(f), map[string]any{
		"op": "learn", "name": "diagnose-ci", "description": "when CI fails", "body": "1. read logs 2. find the failing step",
	})
	if isErr {
		t.Fatalf("learn errored: %v", out)
	}
	if len(f.created) != 1 || f.created[0].Name != "diagnose-ci" {
		t.Fatalf("Create not called as expected: %+v", f.created)
	}
	if out["status"] != "draft" {
		t.Errorf("new skill status = %v, want draft", out["status"])
	}
}

func TestLearn_CorrelationFromContextIsPassedThrough(t *testing.T) {
	f := newFake()
	ctx := agent.WithCorrelation(context.Background(), "run-XYZ")
	invokeCtx(t, ctx, newTool(f), map[string]any{"op": "learn", "name": "n", "body": "b"})
	if f.lastCorr != "run-XYZ" {
		t.Errorf("Create corr = %q, want run-XYZ (the run that authored it)", f.lastCorr)
	}
}

func TestLearn_NeedsNameAndBody(t *testing.T) {
	f := newFake()
	for _, c := range []map[string]any{
		{"op": "learn", "body": "b"}, // no name
		{"op": "learn", "name": "n"}, // no body
	} {
		if _, isErr := invoke(t, newTool(f), c); !isErr {
			t.Errorf("expected error for %v", c)
		}
	}
}

func TestPromote_AdvancesAndResolvesByPrefix(t *testing.T) {
	f := newFake()
	tool := newTool(f)
	out, _ := invoke(t, tool, map[string]any{"op": "learn", "name": "x", "body": "do x"})
	full := skill.ContentID("x", "do x")
	prefix := full[:10]

	pout, isErr := invoke(t, tool, map[string]any{"op": "promote", "id": prefix})
	if isErr {
		t.Fatalf("promote errored: %v", pout)
	}
	if f.promoted != full {
		t.Errorf("promoted id = %q, want full id %q (prefix should resolve)", f.promoted, full)
	}
	if pout["status"] != "shadow" {
		t.Errorf("status after promote = %v, want shadow", pout["status"])
	}
	_ = out
}

func TestRetire_Quarantines(t *testing.T) {
	f := newFake()
	tool := newTool(f)
	invoke(t, tool, map[string]any{"op": "learn", "name": "bad", "body": "oops"})
	id := skill.ContentID("bad", "oops")
	out, isErr := invoke(t, tool, map[string]any{"op": "retire", "id": id, "reason": "kept failing"})
	if isErr {
		t.Fatalf("retire errored: %v", out)
	}
	if f.retired != id {
		t.Errorf("retired = %q, want %q", f.retired, id)
	}
	if out["status"] != "quarantined" {
		t.Errorf("status = %v, want quarantined", out["status"])
	}
}

func TestList_And_Show(t *testing.T) {
	f := newFake()
	tool := newTool(f)
	invoke(t, tool, map[string]any{"op": "learn", "name": "a", "body": "alpha steps"})

	lst, _ := invoke(t, tool, map[string]any{"op": "list"})
	if lst["count"].(float64) != 1 {
		t.Fatalf("list count = %v, want 1", lst["count"])
	}

	id := skill.ContentID("a", "alpha steps")
	sh, isErr := invoke(t, tool, map[string]any{"op": "show", "id": id})
	if isErr {
		t.Fatalf("show errored: %v", sh)
	}
	if sh["body"] != "alpha steps" {
		t.Errorf("show body = %v, want full body", sh["body"])
	}
}

func TestResolve_AmbiguousAndMissing(t *testing.T) {
	f := newFake()
	tool := newTool(f)
	// Missing id.
	if _, isErr := invoke(t, tool, map[string]any{"op": "show", "id": "deadbeef"}); !isErr {
		t.Error("expected error for unknown id")
	}
	// No id at all.
	if _, isErr := invoke(t, tool, map[string]any{"op": "promote"}); !isErr {
		t.Error("expected error when id missing")
	}
}

func TestLearn_SurfacesForgeError(t *testing.T) {
	f := newFake()
	f.retErr = errors.New("forge boom")
	if _, isErr := invoke(t, newTool(f), map[string]any{"op": "learn", "name": "n", "body": "b"}); !isErr {
		t.Error("a Forge error should be surfaced as an error result")
	}
}

func TestBadOps(t *testing.T) {
	f := newFake()
	for _, c := range []map[string]any{{"op": ""}, {"op": "bogus"}} {
		if _, isErr := invoke(t, newTool(f), c); !isErr {
			t.Errorf("expected error for %v", c)
		}
	}
}

func TestFilesAndRead_Bundle(t *testing.T) {
	f := newFake()
	bs, err := skill.OpenBundles(t.TempDir())
	if err != nil {
		t.Fatalf("OpenBundles: %v", err)
	}
	f.bundles = bs
	tool := newTool(f)

	// Author a skill, then attach a bundle out-of-band (the daemon would do this
	// during import; here we drive the store directly and register the skill).
	invoke(t, tool, map[string]any{"op": "learn", "name": "pdf-fill", "body": "fill a pdf"})
	if _, err := bs.Write("pdf-fill", map[string][]byte{
		"reference/fields.md": []byte("the fields"),
		"scripts/run.py":      []byte("print('hi')"),
	}); err != nil {
		t.Fatalf("bundle Write: %v", err)
	}
	id := skill.ContentID("pdf-fill", "fill a pdf")

	// op=files lists the bundle + reports the dir.
	fl, isErr := invoke(t, tool, map[string]any{"op": "files", "id": id})
	if isErr {
		t.Fatalf("files errored: %v", fl)
	}
	if fl["count"].(float64) != 2 {
		t.Fatalf("files count = %v, want 2", fl["count"])
	}
	if fl["dir"] == "" {
		t.Error("files did not report a bundle dir")
	}

	// op=read returns one resource's content.
	rd, isErr := invoke(t, tool, map[string]any{"op": "read", "id": id, "path": "scripts/run.py"})
	if isErr {
		t.Fatalf("read errored: %v", rd)
	}
	if rd["content"] != "print('hi')" {
		t.Errorf("read content = %v, want print('hi')", rd["content"])
	}

	// op=read needs a path; an escaping path is refused by the store.
	if _, isErr := invoke(t, tool, map[string]any{"op": "read", "id": id}); !isErr {
		t.Error("op=read without a path should error")
	}
	if _, isErr := invoke(t, tool, map[string]any{"op": "read", "id": id, "path": "../escape"}); !isErr {
		t.Error("op=read with an escaping path should error")
	}
}

func TestFiles_NoBundleStore(t *testing.T) {
	f := newFake() // bundles nil
	tool := newTool(f)
	invoke(t, tool, map[string]any{"op": "learn", "name": "x", "body": "y"})
	id := skill.ContentID("x", "y")
	if _, isErr := invoke(t, tool, map[string]any{"op": "files", "id": id}); !isErr {
		t.Error("op=files without a bundle store should error gracefully")
	}
}

func TestUnboundIsSafe(t *testing.T) {
	res, err := New().Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("an unbound tool should return an error result")
	}
}
