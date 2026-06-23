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

func TestGenerateImage(t *testing.T) {
	want := []byte("\x89PNG fake bytes")
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		resp := map[string]any{"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(want)}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New(srv.URL, "dall-e-3", "k")
	imgs, mime, err := c.GenerateImage(context.Background(), "a red cube", "1024x1024", "hd", 1)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/images/generations" {
		t.Fatalf("path = %s", gotPath)
	}
	if !strings.Contains(gotBody, `"response_format":"b64_json"`) {
		t.Fatalf("body missing response_format: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"size":"1024x1024"`) {
		t.Fatalf("size not passed: %s", gotBody)
	}
	if len(imgs) != 1 || string(imgs[0]) != string(want) {
		t.Fatalf("decoded wrong: %q", imgs)
	}
	if mime != "image/png" {
		t.Fatalf("mime = %s", mime)
	}
}

func TestGenerateImage_NotConfigured(t *testing.T) {
	c := &Client{}
	if c.HasImage() {
		t.Fatal("empty client should not be configured")
	}
	if _, _, err := c.GenerateImage(context.Background(), "x", "", "", 1); err == nil {
		t.Fatal("expected error for unconfigured client")
	}
}
