// SPDX-License-Identifier: MIT

package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadImageDataURL reads an image off the operator's disk and returns it as a
// self-describing data: URL the daemon forwards to a vision provider (M241).
func TestLoadImageDataURL(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "pic.png")
	raw := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 1, 2, 3}
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	du, err := loadImageDataURL(p)
	if err != nil {
		t.Fatalf("loadImageDataURL: %v", err)
	}
	want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
	if du != want {
		t.Errorf("data URL = %q\nwant %q", du, want)
	}
}

// A .jpg / .jpeg both map to image/jpeg.
func TestLoadImageDataURL_JPEGExtensions(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.jpg", "b.jpeg"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte{0xff, 0xd8, 0xff}, 0o600); err != nil {
			t.Fatal(err)
		}
		du, err := loadImageDataURL(p)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.HasPrefix(du, "data:image/jpeg;base64,") {
			t.Errorf("%s: media type = %q, want image/jpeg", name, du[:24])
		}
	}
}

// A non-image extension is rejected with a clear error before any send.
func TestLoadImageDataURL_UnsupportedType(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(p, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadImageDataURL(p); err == nil {
		t.Error("want error for unsupported image type, got nil")
	}
}

// An empty image file is rejected rather than sent as an empty payload.
func TestLoadImageDataURL_Empty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "blank.png")
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadImageDataURL(p); err == nil {
		t.Error("want error for empty image, got nil")
	}
}

// A missing file surfaces the read error.
func TestLoadImageDataURL_Missing(t *testing.T) {
	if _, err := loadImageDataURL(filepath.Join(t.TempDir(), "nope.png")); err == nil {
		t.Error("want error for missing file, got nil")
	}
}
