// SPDX-License-Identifier: MIT

package webui

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
)

// handleArtifactRaw streams a stored artifact's raw bytes (M822) so an <img src>
// or a download link can render it. It proxies CmdArtifactGet (which re-verifies
// the bytes against the content ref) and serves them with a SANITIZED
// Content-Type — the stored mime is attacker-influenceable (it came off a channel
// message), so only a safe allowlist is honored; everything else is served as
// application/octet-stream. X-Content-Type-Options: nosniff (set globally) blocks
// type sniffing regardless. ?download=1 forces a save dialog.
func (s *Server) handleArtifactRaw(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		http.Error(w, "ref required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	res, err := s.client.Call(ctx, controlplane.CmdArtifactGet, map[string]any{"ref": ref})
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	b64, _ := res["data"].(string)
	data, derr := base64.StdEncoding.DecodeString(b64)
	if derr != nil {
		http.Error(w, "artifact decode failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", safeContentType(r.URL.Query().Get("mime")))
	if r.URL.Query().Get("download") == "1" {
		name := sanitizeFilename(r.URL.Query().Get("name"))
		w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	}
	// Content-addressed bytes are immutable; let the browser cache them privately.
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// safeContentType allows only a known image/document allowlist; anything else
// becomes application/octet-stream so a hostile stored mime can't coax the
// browser into an unexpected rendering.
func safeContentType(mime string) string {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/png", "image/jpeg", "image/jpg", "image/gif", "image/webp", "image/svg+xml",
		"application/pdf", "text/plain":
		if mime == "image/jpg" {
			return "image/jpeg"
		}
		return strings.ToLower(strings.TrimSpace(mime))
	default:
		return "application/octet-stream"
	}
}

// sanitizeFilename keeps a download filename to a safe single path segment.
func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\"", "_")
	name = strings.ReplaceAll(name, "\n", "_")
	name = strings.ReplaceAll(name, "\r", "_")
	if name == "" {
		return "artifact"
	}
	return name
}
