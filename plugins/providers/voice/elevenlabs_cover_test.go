// SPDX-License-Identifier: MIT

package voice

import (
	"context"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- elevenLabsTTS.Speak ---

func TestElevenLabsTTS_Errors(t *testing.T) {
	// Empty text.
	c := &elevenLabsTTS{base: elevenLabsBase, voice: "v", key: "k"}
	if _, _, err := c.Speak(context.Background(), " "); err == nil {
		t.Fatal("empty text should error")
	}
	// Missing voice id.
	c = &elevenLabsTTS{base: elevenLabsBase, key: "k"}
	if _, _, err := c.Speak(context.Background(), "hi"); err == nil {
		t.Fatal("missing voice id should error")
	}
	// Missing key.
	c = &elevenLabsTTS{base: elevenLabsBase, voice: "v"}
	if _, _, err := c.Speak(context.Background(), "hi"); err == nil {
		t.Fatal("missing key should error")
	}
}

func TestElevenLabsTTS_Success(t *testing.T) {
	var gotKey, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("xi-api-key")
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "audio/mpeg")
		io.WriteString(w, "ELAUDIO")
	}))
	defer srv.Close()
	c := &elevenLabsTTS{base: srv.URL, model: "eleven_multilingual_v2", voice: "voice42", key: "kk", http: srv.Client()}
	audio, mimeType, err := c.Speak(context.Background(), "hi there")
	if err != nil {
		t.Fatal(err)
	}
	if string(audio) != "ELAUDIO" || mimeType != "audio/mpeg" {
		t.Fatalf("audio=%q mime=%q", audio, mimeType)
	}
	if gotKey != "kk" {
		t.Fatalf("xi-api-key = %q", gotKey)
	}
	if !strings.Contains(gotPath, "/v1/text-to-speech/voice42") {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.Contains(gotBody, `"text":"hi there"`) {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestElevenLabsTTS_DefaultMime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header()["Content-Type"] = nil
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "A")
	}))
	defer srv.Close()
	c := &elevenLabsTTS{base: srv.URL, voice: "v", key: "k", http: srv.Client()}
	_, mimeType, err := c.Speak(context.Background(), "hi")
	if err != nil || mimeType != "audio/mpeg" {
		t.Fatalf("mime=%q err=%v", mimeType, err)
	}
}

func TestElevenLabsTTS_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, "forbidden")
	}))
	defer srv.Close()
	c := &elevenLabsTTS{base: srv.URL, voice: "v", key: "k", http: srv.Client()}
	_, _, err := c.Speak(context.Background(), "hi")
	if err == nil || !strings.Contains(err.Error(), "ElevenLabs TTS status 403") {
		t.Fatalf("expected status error, got %v", err)
	}
}

// --- elevenLabsSTT.Transcribe ---

func TestElevenLabsSTT_Errors(t *testing.T) {
	c := &elevenLabsSTT{base: elevenLabsBase, key: "k"}
	if _, err := c.Transcribe(context.Background(), nil, "a.ogg"); err == nil {
		t.Fatal("empty audio should error")
	}
	big := make([]byte, maxAudioBytes+1)
	if _, err := c.Transcribe(context.Background(), big, "a.ogg"); err == nil {
		t.Fatal("too-large audio should error")
	}
	c = &elevenLabsSTT{base: elevenLabsBase}
	if _, err := c.Transcribe(context.Background(), []byte("a"), "a.ogg"); err == nil {
		t.Fatal("missing key should error")
	}
}

func TestElevenLabsSTT_Success(t *testing.T) {
	var gotKey, gotModel, gotFile, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("xi-api-key")
		gotPath = r.URL.Path
		_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		mr := readMultipart(t, r.Body, params["boundary"])
		gotModel = mr["model_id"]
		gotFile = mr["file"]
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"text":"  spoken words "}`)
	}))
	defer srv.Close()
	c := &elevenLabsSTT{base: srv.URL, model: "scribe_v2", key: "kk", http: srv.Client()}
	// Empty filename → default branch.
	text, err := c.Transcribe(context.Background(), []byte("AUDIOBYTES"), "")
	if err != nil {
		t.Fatal(err)
	}
	if text != "spoken words" {
		t.Fatalf("text = %q", text)
	}
	if gotKey != "kk" || gotModel != "scribe_v2" || gotFile != "AUDIOBYTES" {
		t.Fatalf("key=%q model=%q file=%q", gotKey, gotModel, gotFile)
	}
	if !strings.HasSuffix(gotPath, "/v1/speech-to-text") {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestElevenLabsSTT_Non2xxAndBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "bad")
	}))
	c := &elevenLabsSTT{base: srv.URL, key: "k", http: srv.Client()}
	if _, err := c.Transcribe(context.Background(), []byte("a"), "a.ogg"); err == nil ||
		!strings.Contains(err.Error(), "ElevenLabs STT status 400") {
		t.Fatalf("expected status error")
	}
	srv.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{bad json`)
	}))
	defer srv2.Close()
	c2 := &elevenLabsSTT{base: srv2.URL, key: "k", http: srv2.Client()}
	if _, err := c2.Transcribe(context.Background(), []byte("a"), "a.ogg"); err == nil {
		t.Fatal("bad JSON should error")
	}
}
