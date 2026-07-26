// SPDX-License-Identifier: MIT

package webui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVoiceStatusReportsConfiguredSpeechHalvesWithoutCallingProviders(t *testing.T) {
	s, _ := newServer(t, &fakeCaller{}, "secret")
	s.SetTranscriber(statusTranscriber{})

	req := httptest.NewRequest(http.MethodGet, "/api/voice/status?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		STT struct {
			Configured bool `json:"configured"`
		} `json:"stt"`
		TTS struct {
			Configured bool `json:"configured"`
		} `json:"tts"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !got.STT.Configured || got.TTS.Configured {
		t.Fatalf("status = %#v, want STT configured and TTS unconfigured", got)
	}
}

func TestVoiceStatusRequiresGETAndAuthentication(t *testing.T) {
	s, _ := newServer(t, &fakeCaller{}, "secret")

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/voice/status?token=secret", strings.NewReader("{}")))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", rec.Code)
	}

	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/voice/status", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", rec.Code)
	}
}

type statusTranscriber struct{}

func (statusTranscriber) Transcribe(context.Context, string, []byte) (string, error) {
	panic("voice status must not invoke the provider")
}
