// SPDX-License-Identifier: MIT

package webui

import "net/http"

// handleVoiceStatus reports which server-side speech halves are wired without
// invoking either provider. It intentionally exposes no endpoint, model, or
// credential detail: the Web UI only needs to know whether recording can be
// transcribed and whether replies can use server-quality speech.
func (s *Server) handleVoiceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"stt": map[string]any{"configured": s.transcriber != nil},
		"tts": map[string]any{"configured": s.synthesizer != nil},
	})
}
