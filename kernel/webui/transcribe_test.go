// SPDX-License-Identifier: MIT

package webui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
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
	err         error
}

func (s *stubTranscriber) Transcribe(_ context.Context, filename string, audio []byte) (string, error) {
	s.gotFilename = filename
	s.gotBytes = len(audio)
	return s.text, s.err
}

type failingMultipartFile struct{}

func (failingMultipartFile) Read([]byte) (int, error)          { return 0, errors.New("broken upload") }
func (failingMultipartFile) ReadAt([]byte, int64) (int, error) { return 0, errors.New("broken upload") }
func (failingMultipartFile) Seek(int64, int) (int64, error)    { return 0, nil }
func (failingMultipartFile) Close() error                      { return nil }

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

func TestTranscribe_RejectsMalformedAndMissingFile(t *testing.T) {
	fc := &fakeCaller{}
	s, _ := newServer(t, fc, "secret")
	s.SetTranscriber(&stubTranscriber{text: "x"})

	t.Run("malformed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/transcribe?token=secret", strings.NewReader("not multipart"))
		req.Header.Set("Content-Type", "multipart/form-data; boundary=missing")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "expected multipart") {
			t.Fatalf("malformed upload: status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("missing file", func(t *testing.T) {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		_ = mw.WriteField("model", "whisper-1")
		_ = mw.Close()
		req := httptest.NewRequest(http.MethodPost, "/api/transcribe?token=secret", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "missing 'file'") {
			t.Fatalf("missing file: status=%d body=%s", rec.Code, rec.Body.String())
		}
	})
}

func TestTranscribe_EnforcesUploadLimit(t *testing.T) {
	fc := &fakeCaller{}
	s, _ := newServer(t, fc, "secret")
	s.SetTranscriber(&stubTranscriber{text: "x"})
	body, ct := multipartAudio(t, "huge.webm", bytes.Repeat([]byte("x"), audioMaxBytes+1))
	req := httptest.NewRequest(http.MethodPost, "/api/transcribe?token=secret", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "request body too large") {
		t.Fatalf("oversized upload: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTranscribeFile_DefaultNameReadAndBackendErrors(t *testing.T) {
	fc := &fakeCaller{}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodPost, "/api/transcribe", nil)

	t.Run("default filename", func(t *testing.T) {
		stub := &stubTranscriber{text: "heard"}
		s.SetTranscriber(stub)
		rec := httptest.NewRecorder()
		s.transcribeFile(rec, req, &multipart.FileHeader{}, multipartFile{Reader: strings.NewReader("audio")})
		if rec.Code != http.StatusOK || stub.gotFilename != "audio.webm" {
			t.Fatalf("status=%d filename=%q", rec.Code, stub.gotFilename)
		}
	})

	t.Run("read error", func(t *testing.T) {
		rec := httptest.NewRecorder()
		s.transcribeFile(rec, req, &multipart.FileHeader{Filename: "x.webm"}, failingMultipartFile{})
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "read upload") {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("backend error", func(t *testing.T) {
		s.SetTranscriber(&stubTranscriber{err: errors.New("provider down")})
		rec := httptest.NewRecorder()
		s.transcribeFile(rec, req, &multipart.FileHeader{Filename: "x.webm"}, multipartFile{Reader: strings.NewReader("audio")})
		if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "provider down") {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})
}

type multipartFile struct{ io.Reader }

func (multipartFile) ReadAt([]byte, int64) (int, error) { return 0, io.EOF }
func (multipartFile) Seek(int64, int) (int64, error)    { return 0, nil }
func (multipartFile) Close() error                      { return nil }
