// SPDX-License-Identifier: MIT

package fetch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/artifact"
)

// fakeIndex captures PutEntry calls.
type fakeIndex struct {
	got  artifact.Entry
	data []byte
}

func (f *fakeIndex) PutEntry(meta artifact.Entry, data []byte, createdMs int64) (artifact.Entry, error) {
	meta.ID = "art-test"
	meta.Ref = "deadbeef"
	meta.Size = int64(len(data))
	meta.CreatedMs = createdMs
	f.got = meta
	f.data = data
	return meta, nil
}

func TestFetch_SavesImageAsArtifact(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png; charset=binary")
		_, _ = w.Write([]byte("PNGBYTES"))
	}))
	defer srv.Close()

	idx := &fakeIndex{}
	tool := New()
	tool.HTTP = srv.Client()
	tool.SetIndex(idx)
	tool.Now = func() int64 { return 7 }

	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"url":"`+srv.URL+`/cat.png"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", res.Output)
	}
	// Stored with the sniffed/declared mime, image kind, name from the URL path.
	if idx.got.Mime != "image/png" || idx.got.Kind != "image" || idx.got.Name != "cat.png" || idx.got.Source != "fetch" {
		t.Fatalf("stored entry = %+v", idx.got)
	}
	if string(idx.data) != "PNGBYTES" || idx.got.CreatedMs != 7 {
		t.Fatalf("data/time = %q,%d", idx.data, idx.got.CreatedMs)
	}
	// The result reports the saved artifact id.
	if !strings.Contains(res.Output, "art-test") || !strings.Contains(res.Output, `"saved": true`) {
		t.Fatalf("result missing id/saved: %s", res.Output)
	}
}

func TestFetch_Rejections(t *testing.T) {
	tool := New()
	tool.SetIndex(&fakeIndex{})

	// Non-http URL.
	if r, _ := tool.Invoke(context.Background(), json.RawMessage(`{"url":"ftp://x/y"}`)); !r.IsError {
		t.Error("ftp URL should be rejected")
	}
	// Empty URL.
	if r, _ := tool.Invoke(context.Background(), json.RawMessage(`{"url":""}`)); !r.IsError {
		t.Error("empty URL should be rejected")
	}
	// No index configured.
	noIdx := New()
	if r, _ := noIdx.Invoke(context.Background(), json.RawMessage(`{"url":"https://x/y"}`)); !r.IsError || !strings.Contains(r.Output, "unavailable") {
		t.Errorf("missing index should report unavailable: %s", r.Output)
	}
}

func TestFetch_ServerErrorIsSoftError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	tool := New()
	tool.HTTP = srv.Client()
	tool.SetIndex(&fakeIndex{})
	r, err := tool.Invoke(context.Background(), json.RawMessage(`{"url":"`+srv.URL+`/missing"}`))
	if err != nil {
		t.Fatalf("Invoke returned a hard error: %v", err)
	}
	if !r.IsError || !strings.Contains(r.Output, "404") {
		t.Errorf("HTTP 404 should be a soft error result: %s", r.Output)
	}
}
