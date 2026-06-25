// SPDX-License-Identifier: MIT

package webui

// Text-to-speech for the console Voice Mode (M998). POST a JSON body {"text": …}
// and the server hands it to the configured TTS backend (the OpenAI-compatible
// voice adapter — the same one agents use to speak replies) and streams back the
// synthesized audio with the backend's Content-Type. Token-gated through the
// same Web UI auth so the browser can speak without knowing the provider address.
// Read-shaped (no daemon state changes); degrades to 501 when TTS isn't
// configured so the UI can fall back to the browser's built-in voice instead of
// failing opaquely.

import (
	"encoding/json"
	"net/http"
)

// ttsTextMaxBytes caps the text to synthesize. A spoken chat reply is short;
// this refuses a runaway request before it reaches the backend.
const ttsTextMaxBytes = 8 << 10 // 8 KiB of text

func (s *Server) handleTTS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	if s.synthesizer == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]any{
			"error": "text-to-speech is not configured: set AGEZT_TTS_URL + AGEZT_TTS_MODEL (and optionally AGEZT_TTS_VOICE / AGEZT_TTS_KEY)",
		})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, ttsTextMaxBytes)
	var in struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "expected JSON body {\"text\": …}: " + err.Error()})
		return
	}
	if in.Text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "text is required"})
		return
	}
	audio, mime, err := s.synthesizer.Speak(r.Context(), in.Text)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	if mime == "" {
		mime = "audio/mpeg"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(audio)
}
