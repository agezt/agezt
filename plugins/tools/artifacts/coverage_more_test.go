// SPDX-License-Identifier: MIT

package artifacts

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/artifact"
)

// covIndex is a tiny per-test fake. The existing artifacts_test.go already
// declares a fakeIndex; we use a distinct name to avoid duplicate symbols.
type covIndex struct {
	entries []artifact.Entry
	bytes   map[string][]byte
	deleted []string
	getErr  error
	delErr  error
}

func (f *covIndex) List(_ artifact.Filter) []artifact.Entry { return f.entries }
func (f *covIndex) Bytes(id string) ([]byte, artifact.Entry, error) {
	if f.getErr != nil {
		return nil, artifact.Entry{}, f.getErr
	}
	for i, e := range f.entries {
		if e.ID == id {
			return f.bytes[id], f.entries[i], nil
		}
	}
	return nil, artifact.Entry{}, errors.New("not found")
}
func (f *covIndex) Delete(id string) error {
	if f.delErr != nil {
		return f.delErr
	}
	f.deleted = append(f.deleted, id)
	return nil
}

func TestArtifactsCoverageDefinition(t *testing.T) {
	tool := New()
	def := tool.Definition()
	if def.Name != "artifacts" {
		t.Fatalf("Name = %q", def.Name)
	}
	if def.Effect.Class != agent.EffectReversible {
		t.Fatalf("Effect.Class = %v, want %v", def.Effect.Class, agent.EffectReversible)
	}
	schema := string(def.InputSchema)
	for _, want := range []string{`"list"`, `"read"`, `"delete"`, `"id"`, `"kind"`} {
		if !strings.Contains(schema, want) {
			t.Fatalf("schema should include %q, got %s", want, schema)
		}
	}
}

func TestArtifactsCoverageIsTextMime(t *testing.T) {
	cases := map[string]bool{
		"text/plain":               true,
		"TEXT/PLAIN":               true,
		"application/json":         true,
		"application/xml":          true,
		"application/vnd.api+json": true,
		"image/png":                false,
	}
	for mime, want := range cases {
		if got := isTextMime(mime, nil); got != want {
			t.Fatalf("isTextMime(%q) = %v, want %v", mime, got, want)
		}
	}
	// Empty mime: without NUL → text, with NUL → binary.
	if !isTextMime("", []byte("plain text")) {
		t.Fatal("empty mime + plain bytes should be text")
	}
	if !isTextMime("", []byte{0x01, 0x02}) {
		t.Fatal("empty mime + non-NUL should be text")
	}
	if isTextMime("", []byte{0, 'a'}) {
		t.Fatal("empty mime + NUL should be binary")
	}
}

func TestArtifactsCoverageInvokeValidation(t *testing.T) {
	_, err := New().Invoke(context.Background(), json.RawMessage(`{`))
	if err == nil || !strings.Contains(err.Error(), "parse input") {
		t.Fatalf("parse error = %v", err)
	}

	res, _ := New().Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if !res.IsError || !strings.Contains(res.Output, "unavailable") {
		t.Fatalf("unavailable = %+v", res)
	}

	tool := New()
	tool.SetIndex(&covIndex{})
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"wat"}`))
	if !res.IsError || !strings.Contains(res.Output, "op must be one of") {
		t.Fatalf("unknown op = %+v", res)
	}
}

func TestArtifactsCoverageInvokeOps(t *testing.T) {
	idx := &covIndex{
		entries: []artifact.Entry{
			{ID: "a", Name: "a.txt", Mime: "text/plain", Size: 5},
			{ID: "b", Name: "b.bin", Mime: "application/octet-stream", Size: 4},
		},
		bytes: map[string][]byte{
			"a": []byte("hello"),
			"b": {0, 1, 2, 3},
		},
	}
	tool := New()
	tool.SetIndex(idx)

	// list with limit > 0 and > entries → no truncation.
	res, _ := tool.Invoke(context.Background(), json.RawMessage(`{"op":"list","limit":5}`))
	if res.IsError {
		t.Fatalf("list = %+v", res)
	}
	if !strings.Contains(res.Output, `"a.txt"`) || !strings.Contains(res.Output, `"b.bin"`) {
		t.Fatalf("list missing entries: %s", res.Output)
	}
	if !strings.Contains(res.Output, `"truncated": false`) {
		t.Fatalf("list should not be truncated with limit>entries: %s", res.Output)
	}

	// list with default limit (0) and truncation triggered.
	big := make([]artifact.Entry, 0, 60)
	for i := 0; i < 60; i++ {
		big = append(big, artifact.Entry{ID: "e", Name: "n"})
	}
	idx.entries = big
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if !strings.Contains(res.Output, `"truncated": true`) {
		t.Fatalf("list should be truncated: %s", res.Output)
	}
	idx.entries = []artifact.Entry{{ID: "a", Name: "a.txt", Mime: "text/plain", Size: 5}, {ID: "b", Name: "b.bin", Mime: "application/octet-stream", Size: 4}}

	// read text: header + body.
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"read","id":"a"}`))
	if res.IsError {
		t.Fatalf("read text = %+v", res)
	}
	if !strings.Contains(res.Output, "a.txt") || !strings.Contains(res.Output, "hello") {
		t.Fatalf("read text output = %s", res.Output)
	}

	// read binary: returns metadata, not body.
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"read","id":"b"}`))
	if res.IsError {
		t.Fatalf("read binary = %+v", res)
	}
	if !strings.Contains(res.Output, `"binary": true`) {
		t.Fatalf("read binary should report binary: %s", res.Output)
	}

	// read no id.
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"read"}`))
	if !res.IsError || !strings.Contains(res.Output, "id required") {
		t.Fatalf("read no id = %+v", res)
	}

	// read get error.
	idx.getErr = errors.New("disk")
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"read","id":"a"}`))
	if !res.IsError || !strings.Contains(res.Output, "disk") {
		t.Fatalf("read error = %+v", res)
	}
	idx.getErr = nil

	// delete ok.
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"delete","id":"a"}`))
	if res.IsError {
		t.Fatalf("delete = %+v", res)
	}
	if len(idx.deleted) != 1 || idx.deleted[0] != "a" {
		t.Fatalf("delete recorded: %v", idx.deleted)
	}

	// delete no id.
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"delete"}`))
	if !res.IsError || !strings.Contains(res.Output, "id required") {
		t.Fatalf("delete no id = %+v", res)
	}

	// delete error.
	idx.delErr = errors.New("nope")
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"delete","id":"a"}`))
	if !res.IsError || !strings.Contains(res.Output, "nope") {
		t.Fatalf("delete error = %+v", res)
	}
}
