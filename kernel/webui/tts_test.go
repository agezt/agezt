// SPDX-License-Identifier: MIT

package webui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubSynthesizer records the text it was handed and returns canned audio.
type stubSynthesizer struct {
	gotText string
	audio   []byte
	mime    string
}

func (s *stubSynthesizer) Speak(_ context.Context, text string) ([]byte, string, error) {
	s.gotText = text
	return s.audio, s.mime, nil
}

func TestTTS_HappyPath(t *testing.T) {
	fc := &fakeCaller{}
	s, _ := newServer(t, fc, "secret")
	stub := &stubSynthesizer{audio: []byte("fake-mp3-bytes"), mime: "audio/mpeg"}
	s.SetSynthesizer(stub)

	req := httptest.NewRequest(http.MethodPost, "/api/tts?token=secret", strings.NewReader(`{"text":"the kitchen light is off"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200; body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "audio/mpeg" {
		t.Errorf("content-type = %q want audio/mpeg", ct)
	}
	if rec.Body.String() != "fake-mp3-bytes" {
		t.Errorf("body = %q want the synthesized audio", rec.Body.String())
	}
	if stub.gotText != "the kitchen light is off" {
		t.Errorf("synthesizer got text = %q", stub.gotText)
	}
}

func TestTTS_DefaultsMime(t *testing.T) {
	fc := &fakeCaller{}
	s, _ := newServer(t, fc, "secret")
	s.SetSynthesizer(&stubSynthesizer{audio: []byte("x"), mime: ""}) // backend gave no MIME
	req := httptest.NewRequest(http.MethodPost, "/api/tts?token=secret", strings.NewReader(`{"text":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "audio/mpeg" {
		t.Errorf("content-type = %q want defaulted audio/mpeg", ct)
	}
}

func TestTTS_NotConfigured(t *testing.T) {
	fc := &fakeCaller{}
	s, _ := newServer(t, fc, "secret") // no SetSynthesizer
	req := httptest.NewRequest(http.MethodPost, "/api/tts?token=secret", strings.NewReader(`{"text":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d want 501", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "not configured") {
		t.Errorf("body should explain TTS is unconfigured: %s", rec.Body.String())
	}
}

func TestTTS_RejectsEmptyText(t *testing.T) {
	fc := &fakeCaller{}
	s, _ := newServer(t, fc, "secret")
	s.SetSynthesizer(&stubSynthesizer{audio: []byte("x"), mime: "audio/mpeg"})
	req := httptest.NewRequest(http.MethodPost, "/api/tts?token=secret", strings.NewReader(`{"text":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty text = %d want 400", rec.Code)
	}
}

func TestTTS_RejectsGET(t *testing.T) {
	fc := &fakeCaller{}
	s, _ := newServer(t, fc, "secret")
	s.SetSynthesizer(&stubSynthesizer{audio: []byte("x"), mime: "audio/mpeg"})
	req := httptest.NewRequest(http.MethodGet, "/api/tts?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d want 405", rec.Code)
	}
}

func TestTTS_RequiresToken(t *testing.T) {
	fc := &fakeCaller{}
	s, _ := newServer(t, fc, "secret")
	s.SetSynthesizer(&stubSynthesizer{audio: []byte("x"), mime: "audio/mpeg"})
	req := httptest.NewRequest(http.MethodPost, "/api/tts", strings.NewReader(`{"text":"hi"}`)) // no token
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Errorf("missing token must not succeed (got 200)")
	}
}
