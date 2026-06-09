// SPDX-License-Identifier: MIT

package webui

// Speech-to-text upload for the chat mic button (M689). POST audio as
// multipart/form-data (field "file"); the server hands it to the configured STT
// backend and returns {"text": …}. Mirrors the OpenAI-API surface's
// /v1/audio/transcriptions, but token-gated through the same Web UI auth so the
// browser can transcribe without knowing the OpenAI-API address. Read-shaped
// (no daemon state changes); degrades to 501 when STT isn't configured so the UI
// can show a friendly "voice isn't set up" message instead of failing opaquely.

import (
	"io"
	"net/http"
)

// audioMaxBytes caps an uploaded clip — matches the OpenAI-API surface's 25 MiB.
const audioMaxBytes = 25 << 20

func (s *Server) handleTranscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	if s.transcriber == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]any{
			"error": "speech-to-text is not configured: set AGEZT_STT_API_KEY (and optionally AGEZT_STT_API_URL / AGEZT_STT_MODEL)",
		})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, audioMaxBytes)
	if err := r.ParseMultipartForm(audioMaxBytes); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "expected multipart/form-data with a 'file' field: " + err.Error()})
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing 'file' form field"})
		return
	}
	defer f.Close()
	audio, err := io.ReadAll(f)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "read upload: " + err.Error()})
		return
	}
	filename := hdr.Filename
	if filename == "" {
		filename = "audio.webm"
	}
	text, err := s.transcriber.Transcribe(r.Context(), filename, audio)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": text})
}
