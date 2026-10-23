// SPDX-License-Identifier: MIT

package artifacts

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/remoteapi"
)

// contractRowKeys reads the artifact row's own wire keys from the contract
// row's tags — so the tool's row is checked against the DECLARATION and not
// against a copy of it that could drift with it.
func contractRowKeys(t *testing.T) map[string]bool {
	t.Helper()
	rt := reflect.TypeOf(remoteapi.Artifact{})
	keys := map[string]bool{}
	for i := 0; i < rt.NumField(); i++ {
		name := strings.Split(rt.Field(i).Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		keys[name] = true
	}
	return keys
}

// captionIndex is a fake index that holds one artifact WITH a caption, so the
// optional key can be observed.
type captionIndex struct{ entry remoteapi.Artifact }

func (f captionIndex) List(flt remoteapi.Filter) []remoteapi.Artifact {
	if flt.Kind != "" && f.entry.Kind != flt.Kind {
		return nil
	}
	return []remoteapi.Artifact{f.entry}
}

func (f captionIndex) Bytes(string) ([]byte, remoteapi.Artifact, error) {
	return nil, remoteapi.Artifact{}, nil
}

func (f captionIndex) Delete(string) error { return nil }

// recordingIndex remembers the filter it was asked for.
type recordingIndex struct{ filter remoteapi.Filter }

func (r *recordingIndex) List(flt remoteapi.Filter) []remoteapi.Artifact {
	r.filter = flt
	return nil
}

func (r *recordingIndex) Bytes(string) ([]byte, remoteapi.Artifact, error) {
	return nil, remoteapi.Artifact{}, nil
}

func (r *recordingIndex) Delete(string) error { return nil }

// TestTheListRowIsTheContractRow: the tool's row must be the artifact row's
// own wire — every key it emits is a key the contract row carries, the value
// is the row's own, and the exact key set is pinned. The keys are read BY
// NAME (the model reads them, the Web UI reads them), so a renamed key is a
// silent break for every consumer outside this package — and no test in this
// package would notice, because the package writes and reads the same map.
func TestTheListRowIsTheContractRow(t *testing.T) {
	tool := New()
	tool.SetIndex(captionIndex{entry: remoteapi.Artifact{
		ID: "a1", Name: "cat.png", Mime: "image/png", Kind: "image",
		Source: "fetch", Size: 8, Caption: "a cat",
	}})

	out, isErr := invoke(t, tool, `{"op":"list"}`)
	if isErr {
		t.Fatalf("list error: %s", out)
	}
	var got struct {
		Artifacts []map[string]any `json:"artifacts"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal list: %v (%s)", err, out)
	}
	if len(got.Artifacts) != 1 {
		t.Fatalf("listed %d artifacts, want 1: %s", len(got.Artifacts), out)
	}
	row := got.Artifacts[0]
	keys := contractRowKeys(t)
	for _, want := range []string{"id", "name", "mime", "kind", "source", "size", "caption"} {
		if _, ok := row[want]; !ok {
			t.Errorf("the tool's row lost the key %q: %s", want, out)
		}
		if !keys[want] {
			t.Errorf("the key %q is not a key of the contract row", want)
		}
	}
	for k := range row {
		if !keys[k] {
			t.Errorf("the tool's row grew the key %q, which the contract row does not carry", k)
		}
	}
	// The values must be the row's own.
	if row["id"] != "a1" || row["name"] != "cat.png" || row["mime"] != "image/png" ||
		row["kind"] != "image" || row["source"] != "fetch" || row["caption"] != "a cat" {
		t.Fatalf("the row does not carry the artifact's own values: %+v", row)
	}
	if row["size"] != float64(8) {
		t.Fatalf("size = %v, want 8", row["size"])
	}
}

// TestTheFilterCarriesEveryField: the tool maps kind, source and corr onto
// the filter. Dropping one of them would silently widen the query — the tool
// would answer with artifacts the caller did not ask for.
func TestTheFilterCarriesEveryField(t *testing.T) {
	idx := &recordingIndex{}
	tool := New()
	tool.SetIndex(idx)
	if _, isErr := invoke(t, tool, `{"op":"list","kind":"image","source":"fetch","corr":"c1"}`); isErr {
		t.Fatal("list should not error")
	}
	if idx.filter.Kind != "image" {
		t.Errorf("filter Kind = %q, want image", idx.filter.Kind)
	}
	if idx.filter.Source != "fetch" {
		t.Errorf("filter Source = %q, want fetch", idx.filter.Source)
	}
	if idx.filter.Corr != "c1" {
		t.Errorf("filter Corr = %q, want c1", idx.filter.Corr)
	}
}
