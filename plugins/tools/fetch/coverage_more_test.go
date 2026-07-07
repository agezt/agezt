// SPDX-License-Identifier: MIT

package fetch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/artifact"
)

type failingIndex struct{}

func (f failingIndex) PutEntry(artifact.Entry, []byte, int64) (artifact.Entry, error) {
	return artifact.Entry{}, errors.New("disk full")
}

func TestFetchCoverageDefinitionAndParseErrors(t *testing.T) {
	tool := New()
	def := tool.Definition()
	if def.Name != "fetch" || len(def.InputSchema) == 0 || def.Effect.Confidence == 0 {
		t.Fatalf("Definition = %+v", def)
	}
	_, err := tool.Invoke(context.Background(), json.RawMessage(`{"url":`))
	if err == nil || !strings.Contains(err.Error(), "parse input") {
		t.Fatalf("malformed input error = %v", err)
	}
}

func TestFetchCoverageDefaultUserAgentExplicitNameAndDetectMime(t *testing.T) {
	var sawUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte("hello from fetch"))
	}))
	defer srv.Close()

	idx := &fakeIndex{}
	tool := New()
	tool.HTTP = srv.Client()
	tool.UserAgent = "" // force default fallback branch inside Invoke.
	tool.SetIndex(idx)
	tool.Now = func() int64 { return 99 }

	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"url":"`+srv.URL+`/ignored.bin","name":" saved.txt "}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected soft error: %s", res.Output)
	}
	if sawUA != DefaultUserAgent {
		t.Fatalf("User-Agent = %q, want %q", sawUA, DefaultUserAgent)
	}
	if idx.got.Name != "saved.txt" || idx.got.Kind != "download" || idx.got.CreatedMs != 99 {
		t.Fatalf("stored entry = %+v", idx.got)
	}
	if idx.got.Mime == "" {
		t.Fatal("mime should be detected when Content-Type is absent")
	}
	if res.ObservationSource != srv.URL+"/ignored.bin" {
		t.Fatalf("ObservationSource = %q", res.ObservationSource)
	}
}

func TestFetchCoverageSoftErrorBranches(t *testing.T) {
	t.Run("empty body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer srv.Close()
		tool := New()
		tool.HTTP = srv.Client()
		tool.SetIndex(&fakeIndex{})
		res, err := tool.Invoke(context.Background(), json.RawMessage(`{"url":"`+srv.URL+`"}`))
		if err != nil {
			t.Fatalf("Invoke: %v", err)
		}
		if !res.IsError || !strings.Contains(res.Output, "0 bytes") {
			t.Fatalf("empty body result = %s", res.Output)
		}
	})

	t.Run("save failure", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("data"))
		}))
		defer srv.Close()
		tool := New()
		tool.HTTP = srv.Client()
		tool.SetIndex(failingIndex{})
		res, err := tool.Invoke(context.Background(), json.RawMessage(`{"url":"`+srv.URL+`/file"}`))
		if err != nil {
			t.Fatalf("Invoke: %v", err)
		}
		if !res.IsError || !strings.Contains(res.Output, "save: disk full") {
			t.Fatalf("save failure result = %s", res.Output)
		}
	})
}

func TestFetchCoverageHelpers(t *testing.T) {
	if got := cleanMime(" Text/Plain ; charset=utf-8 "); got != "text/plain" {
		t.Fatalf("cleanMime = %q", got)
	}
	if got := kindForMime("image/jpeg"); got != "image" {
		t.Fatalf("image kind = %q", got)
	}
	if got := kindForMime("application/pdf"); got != "download" {
		t.Fatalf("download kind = %q", got)
	}
	cases := map[string]string{
		"https://example.com/path/file.pdf": "file.pdf",
		"https://example.com/":              "example.com",
		"%zz":                               "download",
	}
	for raw, want := range cases {
		if got := nameFromURL(raw); got != want {
			t.Fatalf("nameFromURL(%q) = %q, want %q", raw, got, want)
		}
	}
	if got := errResult("boom"); !got.IsError || got.Output != "fetch: boom" {
		t.Fatalf("errResult = %+v", got)
	}
}
