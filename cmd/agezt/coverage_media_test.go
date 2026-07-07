// SPDX-License-Identifier: MIT

package main

import (
	"encoding/base64"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestVoiceReplyEnabled exercises the default-on behaviour and every off token.
func TestVoiceReplyEnabled(t *testing.T) {
	// Default (unset) → enabled.
	t.Setenv("AGEZT_VOICE_REPLY", "")
	if !voiceReplyEnabled() {
		t.Error("voiceReplyEnabled should default to true when unset")
	}
	for _, off := range []string{"off", "0", "false", "no", "OFF", " No "} {
		t.Setenv("AGEZT_VOICE_REPLY", off)
		if voiceReplyEnabled() {
			t.Errorf("voiceReplyEnabled(%q) = true, want false", off)
		}
	}
	// A non-off value keeps it on.
	t.Setenv("AGEZT_VOICE_REPLY", "on")
	if !voiceReplyEnabled() {
		t.Error("voiceReplyEnabled(\"on\") should be true")
	}
}

// TestAudioExt maps every recognised MIME family plus the default fallback.
func TestAudioExt(t *testing.T) {
	cases := map[string]string{
		"audio/ogg":         ".ogg",
		"audio/opus":        ".ogg",
		"audio/mpeg":        ".mp3",
		"audio/mp3":         ".mp3",
		"audio/wav":         ".wav",
		"audio/aac":         ".m4a",
		"audio/m4a":         ".m4a",
		"audio/mp4":         ".m4a",
		"application/octet": ".ogg", // unknown → default
		"":                  ".ogg", // empty → default
	}
	for mime, want := range cases {
		if got := audioExt(mime); got != want {
			t.Errorf("audioExt(%q) = %q, want %q", mime, got, want)
		}
	}
}

func dataURL(mime string, body string) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString([]byte(body))
}

// TestPersistInboundAudio drives the real ArtifactIndex path: a nil-audio/no-op
// guard, a bad clip that's skipped, and a good clip that's stored.
func TestPersistInboundAudio(t *testing.T) {
	k, err := kernelruntime.Open(kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("unused")),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	// No audio → returns early, stores nothing.
	persistInboundAudio(k, channel.UnifiedMessage{ChannelKind: "telegram"}, "corr-1")

	// One valid clip + one undecodable clip (skipped).
	msg := channel.UnifiedMessage{
		ChannelKind: "telegram",
		Sender:      "user-1",
		Audio: []string{
			dataURL("audio/ogg", "VOICE"),
			"not-a-data-url",
		},
	}
	persistInboundAudio(k, msg, "corr-2")
}

// TestPersistInboundImages mirrors the audio path for image attachments.
func TestPersistInboundImages(t *testing.T) {
	k, err := kernelruntime.Open(kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("unused")),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	// No images → no-op (returns nil).
	if ids := persistInboundImages(k, channel.UnifiedMessage{ChannelKind: "slack"}, "corr-3", ""); ids != nil {
		t.Errorf("no-image call should return nil, got %v", ids)
	}

	msg := channel.UnifiedMessage{
		ChannelKind: "slack",
		Sender:      "user-2",
		Images: []string{
			dataURL("image/png", "PNGBYTES"),
			"garbage",
		},
	}
	// One decodable image stored → at least one id; caption attached.
	if ids := persistInboundImages(k, msg, "corr-4", "a red square"); len(ids) == 0 {
		t.Error("expected at least one stored image id")
	}
}

// TestVisionGate wires the kernel-backed wrapper: no images passes, and an
// image with an unknown/non-vision model is rejected (mirrors the M255 gate).
func TestVisionGate(t *testing.T) {
	k, err := kernelruntime.Open(kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("unused")),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if err := visionGate(k, "some-model", nil); err != nil {
		t.Errorf("visionGate with no images should pass, got %v", err)
	}
	if err := visionGate(k, "definitely-not-a-vision-model", []string{"data:image/png;base64,AAA"}); err == nil {
		t.Error("visionGate should reject an image run on a non-vision model")
	}
}
