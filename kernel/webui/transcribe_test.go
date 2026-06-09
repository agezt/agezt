// SPDX-License-Identifier: MIT

package webui

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubTranscriber records the audio it was handed and returns a canned text.
type stubTranscriber struct {
	gotFilename string
	gotBytes    int
	text        string
}

func (s *stubTranscriber) Transcribe(_ context.Context, filename string, audio []byte) (string, error) {
	s.gotFilename = filename
	s.gotBytes = len(audio)
	return s.text, nil
}

// multipartAudio builds a multipart/form-data body with a single "file" field.
func multipartAudio(t *testing.T, filename string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	mw.Close()
	return &buf, mw.FormDataContentType()
}

func TestTranscribe_HappyPath(t *testing.T) {
	fc := &fakeCaller{}
	s, _ := newServer(t, fc, "secret")
	stub := &stubTranscriber{text: "turn off the kitchen light"}
	s.SetTranscriber(stub)

	body, ct := multipartAudio(t, "clip.webm", []byte("fake-audio-bytes"))
	req := httptest.NewRequest(http.MethodPost, "/api/transcribe?token=secret", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200; body=%s", rec.Code, rec.Body.String())
	}
	var out struct{ Text string }
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Text != "turn off the kitchen light" {
		t.Errorf("text = %q", out.Text)
	}
	if stub.gotFilename != "clip.webm" || stub.gotBytes != len("fake-audio-bytes") {
		t.Errorf("transcriber got filename=%q bytes=%d", stub.gotFilename, stub.gotBytes)
	}
}

func TestTranscribe_NotConfigured(t *testing.T) {
	fc := &fakeCaller{}
	s, _ := newServer(t, fc, "secret") // no SetTranscriber
	body, ct := multipartAudio(t, "clip.webm", []byte("x"))
	req := httptest.NewRequest(http.MethodPost, "/api/transcribe?token=secret", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d want 501", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "not configured") {
		t.Errorf("body should explain STT is unconfigured: %s", rec.Body.String())
	}
}

func TestTranscribe_RejectsGET(t *testing.T) {
	fc := &fakeCaller{}
	s, _ := newServer(t, fc, "secret")
	s.SetTranscriber(&stubTranscriber{text: "x"})
	req := httptest.NewRequest(http.MethodGet, "/api/transcribe?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d want 405", rec.Code)
	}
}

func TestTranscribe_RequiresToken(t *testing.T) {
	fc := &fakeCaller{}
	s, _ := newServer(t, fc, "secret")
	s.SetTranscriber(&stubTranscriber{text: "x"})
	body, ct := multipartAudio(t, "clip.webm", []byte("x"))
	req := httptest.NewRequest(http.MethodPost, "/api/transcribe", body) // no token
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Errorf("missing token must not succeed (got 200)")
	}
}
