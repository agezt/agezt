// SPDX-License-Identifier: MIT

package openaiapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeTranscriber struct {
	text     string
	gotName  string
	gotAudio []byte
}

func (f *fakeTranscriber) Transcribe(_ context.Context, name string, audio []byte) (string, error) {
	f.gotName = name
	f.gotAudio = audio
	return f.text, nil
}

func multipartAudio(t *testing.T, field, filename string, audio []byte) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if filename != "" {
		fw, err := mw.CreateFormFile(field, filename)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write(audio)
	}
	_ = mw.WriteField("model", "whisper-1")
	_ = mw.Close()
	return &body, mw.FormDataContentType()
}

func TestTranscription_UploadReturnsText(t *testing.T) {
	srv := newAPIServer(t, &fakeEngine{model: "m"}, "tok")
	ft := &fakeTranscriber{text: "the transcribed words"}
	srv.SetTranscriber(ft)

	body, ct := multipartAudio(t, "file", "clip.wav", []byte("RIFFfake-audio"))
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", body)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Text != "the transcribed words" {
		t.Errorf("text = %q", out.Text)
	}
	if ft.gotName != "clip.wav" || string(ft.gotAudio) != "RIFFfake-audio" {
		t.Errorf("transcriber got name=%q audio=%q", ft.gotName, ft.gotAudio)
	}
}

func TestTranscription_NotConfigured(t *testing.T) {
	srv := newAPIServer(t, &fakeEngine{model: "m"}, "tok") // no SetTranscriber
	body, ct := multipartAudio(t, "file", "a.wav", []byte("x"))
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", body)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("code = %d, want 501 (not configured)", rec.Code)
	}
}

func TestTranscription_MethodAndMissingFile(t *testing.T) {
	srv := newAPIServer(t, &fakeEngine{model: "m"}, "tok")
	srv.SetTranscriber(&fakeTranscriber{text: "x"})
	h := srv.Handler()

	// GET → 405
	get := httptest.NewRequest(http.MethodGet, "/v1/audio/transcriptions", nil)
	get.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, get)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET code = %d, want 405", rec.Code)
	}

	// POST without a file field → 400
	body, ct := multipartAudio(t, "file", "", nil)
	post := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", body)
	post.Header.Set("Content-Type", ct)
	post.Header.Set("Authorization", "Bearer tok")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, post)
	if rec2.Code != http.StatusBadRequest {
		t.Errorf("missing-file code = %d, want 400", rec2.Code)
	}
}

func TestTranscription_RequiresAuth(t *testing.T) {
	srv := newAPIServer(t, &fakeEngine{model: "m"}, "tok")
	srv.SetTranscriber(&fakeTranscriber{text: "x"})
	body, ct := multipartAudio(t, "file", "a.wav", []byte("x"))
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", body)
	req.Header.Set("Content-Type", ct) // no Authorization
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no-auth code = %d, want 401", rec.Code)
	}
}
