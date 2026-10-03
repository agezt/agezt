package main

// Provenance: SPDX-License-Identifier: MIT cmd/agezt channel small helpers
//             (voiceReplyEnabled, audioExt, extForMime,
//             splitNonEmpty). Extracted from main_channels.go during Day 211
//             god-file refactor (#43, #66). Public API unchanged.

import (
	"os"
	"strings"

	"github.com/agezt/agezt/internal/brand"
)

func voiceReplyEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "VOICE_REPLY")))
	return v != "off" && v != "0" && v != "false" && v != "no"
}
func audioExt(mime string) string {
	switch {
	case strings.Contains(mime, "ogg"), strings.Contains(mime, "opus"):
		return ".ogg"
	case strings.Contains(mime, "mpeg"), strings.Contains(mime, "mp3"):
		return ".mp3"
	case strings.Contains(mime, "wav"):
		return ".wav"
	case strings.Contains(mime, "aac"), strings.Contains(mime, "m4a"), strings.Contains(mime, "mp4"):
		return ".m4a"
	default:
		return ".ogg"
	}
}
func extForMime(mime string) string {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "audio/ogg", "audio/opus":
		return ".ogg"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/mp4", "audio/m4a", "audio/x-m4a":
		return ".m4a"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	default:
		return ""
	}
}
func splitNonEmpty(s string) []string {
	var out []string
	for part := range strings.SplitSeq(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
