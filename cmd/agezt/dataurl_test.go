// SPDX-License-Identifier: MIT

package main

import (
	"encoding/base64"
	"testing"
)

func TestDecodeDataURL(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("PNGDATA"))

	mime, data, ok := decodeDataURL("data:image/png;base64," + b64)
	if !ok || mime != "image/png" || string(data) != "PNGDATA" {
		t.Fatalf("base64 png = %q,%q,%v", mime, data, ok)
	}

	// No mime → empty mime, still decodes.
	if _, d, ok := decodeDataURL("data:;base64," + b64); !ok || string(d) != "PNGDATA" {
		t.Errorf("no-mime base64 failed: %q,%v", d, ok)
	}

	// Not a data URL.
	if _, _, ok := decodeDataURL("https://x/y.png"); ok {
		t.Error("non-data URL should be rejected")
	}
	// No comma.
	if _, _, ok := decodeDataURL("data:image/png;base64"); ok {
		t.Error("missing comma should be rejected")
	}
	// Bad base64.
	if _, _, ok := decodeDataURL("data:image/png;base64,!!!notb64!!!"); ok {
		t.Error("invalid base64 should be rejected")
	}
}

func TestExtForMime(t *testing.T) {
	cases := map[string]string{
		"image/jpeg": ".jpg", "image/jpg": ".jpg", "image/png": ".png",
		"image/gif": ".gif", "image/webp": ".webp", "application/zip": "",
	}
	for mime, want := range cases {
		if got := extForMime(mime); got != want {
			t.Errorf("extForMime(%q) = %q, want %q", mime, got, want)
		}
	}
}
