// SPDX-License-Identifier: MIT

package voice

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTranscribeMultipart(t *testing.T) {
	var gotModel, gotFile string
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		mediaType, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if !strings.HasPrefix(mediaType, "multipart/") {
			t.Errorf("content-type = %q", r.Header.Get("Content-Type"))
		}
		mr := readMultipart(t, r.Body, params["boundary"])
		gotModel = mr["model"]
		gotFile = mr["file"]
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"text":"hello world"}`)
	}))
	defer srv.Close()

	c := &STTClient{BaseURL: srv.URL, Model: "whisper-1", HTTP: srv.Client()}
	text, err := c.Transcribe(context.Background(), []byte("OGGDATA"), "note.ogg")
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if text != "hello world" {
		t.Fatalf("text = %q", text)
	}
	if gotModel != "whisper-1" || gotFile != "OGGDATA" {
		t.Fatalf("model=%q file=%q", gotModel, gotFile)
	}
	if !strings.HasSuffix(gotPath, "/v1/audio/transcriptions") {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestSpeakReturnsAudio(t *testing.T) {
	var gotBody, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "audio/mpeg")
		io.WriteString(w, "ID3AUDIO")
	}))
	defer srv.Close()

	c := &TTSClient{BaseURL: srv.URL + "/v1", Model: "tts-1", HTTP: srv.Client()}
	audio, mimeType, err := c.Speak(context.Background(), "say this")
	if err != nil {
		t.Fatalf("speak: %v", err)
	}
	if string(audio) != "ID3AUDIO" || mimeType != "audio/mpeg" {
		t.Fatalf("audio=%q mime=%q", audio, mimeType)
	}
	if !strings.Contains(gotBody, `"input":"say this"`) || !strings.Contains(gotBody, `"voice":"alloy"`) {
		t.Fatalf("body = %q (voice should default to alloy)", gotBody)
	}
	if !strings.HasSuffix(gotPath, "/v1/audio/speech") {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestAdapterAvailabilityAndErrors(t *testing.T) {
	a := &Adapter{}
	if a.HasSTT() || a.HasTTS() {
		t.Fatal("empty adapter should have neither half")
	}
	if _, err := a.Transcribe(context.Background(), []byte("x"), ""); err == nil {
		t.Fatal("transcribe without STT must error")
	}
	if _, _, err := a.Speak(context.Background(), "x"); err == nil {
		t.Fatal("speak without TTS must error")
	}
	a.STT = &STTClient{BaseURL: "http://x", Model: "m"}
	if !a.HasSTT() {
		t.Fatal("STT should be available")
	}
	if _, err := a.STT.Transcribe(context.Background(), nil, ""); err == nil {
		t.Fatal("empty audio must error")
	}
}

// readMultipart reads a multipart form into a {field: value} map (file fields
// keyed by their form name, value = file contents).
func readMultipart(t *testing.T, body io.Reader, boundary string) map[string]string {
	t.Helper()
	out := map[string]string{}
	mr := multipart.NewReader(body, boundary)
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		b, _ := io.ReadAll(p)
		out[p.FormName()] = string(b)
	}
	return out
}
