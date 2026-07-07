// SPDX-License-Identifier: MIT

package telegram

import "testing"

func TestTgMediaType_JPEG(t *testing.T) {
	if got := tgMediaType("photo.jpg"); got != "image/jpeg" {
		t.Errorf("tgMediaType(.jpg) = %q, want image/jpeg", got)
	}
}

func TestTgMediaType_PNG(t *testing.T) {
	if got := tgMediaType("photo.png"); got != "image/png" {
		t.Errorf("tgMediaType(.png) = %q, want image/png", got)
	}
}

func TestTgMediaType_GIF(t *testing.T) {
	if got := tgMediaType("anim.gif"); got != "image/gif" {
		t.Errorf("tgMediaType(.gif) = %q, want image/gif", got)
	}
}

func TestTgMediaType_WebP(t *testing.T) {
	if got := tgMediaType("sticker.webp"); got != "image/webp" {
		t.Errorf("tgMediaType(.webp) = %q, want image/webp", got)
	}
}

func TestTgMediaType_Audio(t *testing.T) {
	cases := []struct{ path, want string }{
		{"voice.oga", "audio/ogg"},
		{"voice.ogg", "audio/ogg"},
		{"song.mp3", "audio/mpeg"},
		{"audio.m4a", "audio/mp4"},
		{"recording.wav", "audio/wav"},
	}
	for _, tc := range cases {
		got := tgMediaType(tc.path)
		if got != tc.want {
			t.Errorf("tgMediaType(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestTgMediaType_Unknown(t *testing.T) {
	// No extension, or unknown extension → defaults to JPEG.
	if got := tgMediaType("somefile"); got != "image/jpeg" {
		t.Errorf("tgMediaType(no-ext) = %q, want image/jpeg", got)
	}
	if got := tgMediaType("file.mp4"); got != "image/jpeg" {
		t.Errorf("tgMediaType(.mp4) = %q, want image/jpeg (not an audio type)", got)
	}
}
