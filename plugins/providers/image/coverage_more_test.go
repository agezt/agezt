// SPDX-License-Identifier: MIT

package image

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestImageCoverageEndpointVariantsAndDefaults(t *testing.T) {
	if got := (&Client{BaseURL: " https://api.example/v1/ "}).endpoint(); got != "https://api.example/v1/images/generations" {
		t.Fatalf("endpoint with /v1 = %q", got)
	}
	if got := (&Client{BaseURL: "https://proxy.example/api/v1/custom"}).endpoint(); got != "https://proxy.example/api/v1/custom/images/generations" {
		t.Fatalf("endpoint containing /v1/ = %q", got)
	}
	if (&Client{BaseURL: " ", Model: "m"}).HasImage() || (&Client{BaseURL: "u", Model: " "}).HasImage() {
		t.Fatal("HasImage should require non-empty base URL and model")
	}

	var gotBody imageRequest
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(b, &gotBody); err != nil {
			t.Fatalf("request body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(imageResponse{Data: []struct {
			B64JSON string `json:"b64_json"`
		}{{B64JSON: base64.StdEncoding.EncodeToString([]byte("png"))}}})
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Model: "img", HTTP: srv.Client()}
	imgs, _, err := c.GenerateImage(context.Background(), "prompt", "", "", 0)
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if gotBody.N != 1 || gotBody.Size != "" || gotBody.Quality != "" || gotBody.ResponseFormat != "b64_json" {
		t.Fatalf("body defaults = %+v", gotBody)
	}
	if gotAuth != "" {
		t.Fatalf("Authorization should be omitted without API key, got %q", gotAuth)
	}
	if len(imgs) != 1 || string(imgs[0]) != "png" {
		t.Fatalf("imgs = %q", imgs)
	}
}

func TestImageCoverageValidationAndErrorBranches(t *testing.T) {
	configured := &Client{BaseURL: "http://example.invalid", Model: "img", HTTP: &http.Client{}}
	if _, _, err := configured.GenerateImage(context.Background(), "   ", "", "", 1); err == nil || !strings.Contains(err.Error(), "prompt required") {
		t.Fatalf("blank prompt error = %v", err)
	}

	cases := []struct {
		name string
		fn   func(http.ResponseWriter)
		want string
	}{
		{name: "status", fn: func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusTeapot)
			_, _ = w.Write([]byte("short and stout"))
		}, want: "status 418"},
		{name: "bad json", fn: func(w http.ResponseWriter) { _, _ = w.Write([]byte("not-json")) }, want: "decode"},
		{name: "no images", fn: func(w http.ResponseWriter) { _, _ = w.Write([]byte(`{"data":[]}`)) }, want: "no images"},
		{name: "empty b64", fn: func(w http.ResponseWriter) { _, _ = w.Write([]byte(`{"data":[{"b64_json":"  "}]}`)) }, want: "had no b64_json"},
		{name: "bad b64", fn: func(w http.ResponseWriter) { _, _ = w.Write([]byte(`{"data":[{"b64_json":"%%%"}]}`)) }, want: "decode image"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { tc.fn(w) }))
			defer srv.Close()
			c := &Client{BaseURL: srv.URL, Model: "img", HTTP: srv.Client()}
			_, _, err := c.GenerateImage(context.Background(), "prompt", "", "", 1)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}
