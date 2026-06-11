// SPDX-License-Identifier: MIT

package artifacts

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/artifact"
)

// fakeIndex is an in-memory stand-in for *artifact.Index.
type fakeIndex struct {
	entries map[string]artifact.Entry
	blobs   map[string][]byte // id -> bytes
	deleted []string
}

func (f *fakeIndex) List(flt artifact.Filter) []artifact.Entry {
	var out []artifact.Entry
	for _, e := range f.entries {
		if flt.Kind != "" && e.Kind != flt.Kind {
			continue
		}
		if flt.Source != "" && e.Source != flt.Source {
			continue
		}
		out = append(out, e)
	}
	return out
}

func (f *fakeIndex) Bytes(id string) ([]byte, artifact.Entry, error) {
	e, ok := f.entries[id]
	if !ok {
		return nil, artifact.Entry{}, artifact.ErrNotFound
	}
	return f.blobs[id], e, nil
}

func (f *fakeIndex) Delete(id string) error {
	if _, ok := f.entries[id]; !ok {
		return artifact.ErrNotFound
	}
	delete(f.entries, id)
	f.deleted = append(f.deleted, id)
	return nil
}

func newTool() (*Tool, *fakeIndex) {
	idx := &fakeIndex{
		entries: map[string]artifact.Entry{
			"art-img": {ID: "art-img", Name: "cat.png", Mime: "image/png", Kind: "image", Source: "fetch", Size: 8},
			"art-txt": {ID: "art-txt", Name: "notes.md", Mime: "text/markdown", Kind: "download", Source: "fetch", Size: 5},
		},
		blobs: map[string][]byte{
			"art-img": {0x89, 'P', 'N', 'G', 0, 1, 2, 3}, // contains NUL → binary
			"art-txt": []byte("hello"),
		},
	}
	t := New()
	t.SetIndex(idx)
	return t, idx
}

func invoke(t *testing.T, tool *Tool, in string) (string, bool) {
	t.Helper()
	r, err := tool.Invoke(context.Background(), json.RawMessage(in))
	if err != nil {
		t.Fatalf("Invoke(%s): %v", in, err)
	}
	return r.Output, r.IsError
}

func TestArtifacts_List(t *testing.T) {
	tool, _ := newTool()
	out, isErr := invoke(t, tool, `{"op":"list"}`)
	if isErr {
		t.Fatalf("list error: %s", out)
	}
	var got struct {
		Count     int              `json:"count"`
		Artifacts []map[string]any `json:"artifacts"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal list: %v (%s)", err, out)
	}
	if got.Count != 2 {
		t.Fatalf("count = %d, want 2", got.Count)
	}

	// Filter by kind.
	out, _ = invoke(t, tool, `{"op":"list","kind":"image"}`)
	_ = json.Unmarshal([]byte(out), &got)
	if got.Count != 1 || got.Artifacts[0]["id"] != "art-img" {
		t.Fatalf("kind filter = %s", out)
	}
}

func TestArtifacts_ReadText(t *testing.T) {
	tool, _ := newTool()
	out, isErr := invoke(t, tool, `{"op":"read","id":"art-txt"}`)
	if isErr {
		t.Fatalf("read error: %s", out)
	}
	if !strings.Contains(out, "hello") || !strings.Contains(out, "notes.md") {
		t.Fatalf("text read missing content: %s", out)
	}
}

func TestArtifacts_ReadBinaryReportsMetadata(t *testing.T) {
	tool, _ := newTool()
	out, isErr := invoke(t, tool, `{"op":"read","id":"art-img"}`)
	if isErr {
		t.Fatalf("read error: %s", out)
	}
	if !strings.Contains(out, `"binary": true`) || !strings.Contains(out, "Files view") {
		t.Fatalf("binary read should report metadata + note: %s", out)
	}
	// The raw PNG bytes must NOT be dumped inline.
	if strings.Contains(out, "PNG\x00") {
		t.Fatalf("binary bytes leaked into output: %s", out)
	}
}

func TestArtifacts_Delete(t *testing.T) {
	tool, idx := newTool()
	out, isErr := invoke(t, tool, `{"op":"delete","id":"art-txt"}`)
	if isErr {
		t.Fatalf("delete error: %s", out)
	}
	if len(idx.deleted) != 1 || idx.deleted[0] != "art-txt" {
		t.Fatalf("delete not propagated: %v", idx.deleted)
	}
	// Deleting a missing id is a soft error.
	if _, isErr := invoke(t, tool, `{"op":"delete","id":"nope"}`); !isErr {
		t.Error("deleting unknown id should be a soft error")
	}
}

func TestArtifacts_Rejections(t *testing.T) {
	tool, _ := newTool()
	if _, isErr := invoke(t, tool, `{"op":"frobnicate"}`); !isErr {
		t.Error("unknown op should be rejected")
	}
	if _, isErr := invoke(t, tool, `{"op":"read"}`); !isErr {
		t.Error("read without id should be rejected")
	}
	// No index configured.
	noIdx := New()
	r, _ := noIdx.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if !r.IsError || !strings.Contains(r.Output, "unavailable") {
		t.Errorf("missing index should report unavailable: %s", r.Output)
	}
}
