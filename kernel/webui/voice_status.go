// SPDX-License-Identifier: MIT

package webui

import (
	"net/http"

	"github.com/agezt/agezt/kernel/contract/consoleapi"
)

// handleVoiceStatus reports which server-side speech halves are wired without
// invoking either provider. It intentionally exposes no endpoint, model, or
// credential detail: the Web UI only needs to know whether recording can be
// transcribed and whether replies can use server-quality speech.
func (s *Server) handleVoiceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	writeJSON(w, http.StatusOK, consoleapi.VoiceStatus{
		STT: consoleapi.VoiceHalf{Configured: s.transcriber != nil},
		TTS: consoleapi.VoiceHalf{Configured: s.synthesizer != nil},
	})
}
