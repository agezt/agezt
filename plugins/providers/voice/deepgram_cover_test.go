// SPDX-License-Identifier: MIT

package voice

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAudioContentType(t *testing.T) {
	cases := map[string]string{
		"note.ogg":      "audio/ogg",
		"a.opus":        "audio/ogg",
		"a.oga":         "audio/ogg",
		"clip.webm":     "audio/webm",
		"song.mp3":      "audio/mpeg",
		"rec.wav":       "audio/wav",
		"voice.m4a":     "audio/mp4",
		"movie.mp4":     "audio/mp4",
		"lossless.flac": "audio/flac",
		"unknown.xyz":   "application/octet-stream",
		"noext":         "application/octet-stream",
	}
	for name, want := range cases {
		if got := audioContentType(name); got != want {
			t.Errorf("audioContentType(%q) = %q, want %q", name, got, want)
		}
	}
}

// --- deepgramSTT.Transcribe ---

func TestDeepgramSTT_Errors(t *testing.T) {
	c := &deepgramSTT{base: deepgramBase, key: "k"}
	if _, err := c.Transcribe(context.Background(), nil, "a.ogg"); err == nil {
		t.Fatal("empty audio should error")
	}
	big := make([]byte, maxAudioBytes+1)
	if _, err := c.Transcribe(context.Background(), big, "a.ogg"); err == nil {
		t.Fatal("too-large audio should error")
	}
	// Missing key.
	c = &deepgramSTT{base: deepgramBase}
	if _, err := c.Transcribe(context.Background(), []byte("a"), "a.ogg"); err == nil {
		t.Fatal("missing key should error")
	}
}

func TestDeepgramSTT_Success(t *testing.T) {
	var gotAuth, gotCT, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"results":{"channels":[{"alternatives":[{"transcript":" hi there "}]}]}}`)
	}))
	defer srv.Close()
	c := &deepgramSTT{base: srv.URL, model: "nova-3", key: "secret", http: srv.Client()}
	text, err := c.Transcribe(context.Background(), []byte("audio"), "note.ogg")
	if err != nil {
		t.Fatal(err)
	}
	if text != "hi there" {
		t.Fatalf("text = %q", text)
	}
	if gotAuth != "Token secret" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if gotCT != "audio/ogg" {
		t.Fatalf("content-type = %q", gotCT)
	}
	if !strings.Contains(gotQuery, "model=nova-3") {
		t.Fatalf("query = %q", gotQuery)
	}
}

func TestDeepgramSTT_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, "nope")
	}))
	defer srv.Close()
	c := &deepgramSTT{base: srv.URL, key: "k", http: srv.Client()}
	_, err := c.Transcribe(context.Background(), []byte("a"), "a.ogg")
	if err == nil || !strings.Contains(err.Error(), "Deepgram STT status 401") {
		t.Fatalf("expected status error, got %v", err)
	}
}

func TestDeepgramSTT_BadJSONAndEmptyChannels(t *testing.T) {
	// Bad JSON.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{bad`)
	}))
	c := &deepgramSTT{base: srv.URL, key: "k", http: srv.Client()}
	if _, err := c.Transcribe(context.Background(), []byte("a"), "a.ogg"); err == nil {
		t.Fatal("bad JSON should error")
	}
	srv.Close()

	// Empty channels → "" with no error.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"results":{"channels":[]}}`)
	}))
	defer srv2.Close()
	c2 := &deepgramSTT{base: srv2.URL, key: "k", http: srv2.Client()}
	text, err := c2.Transcribe(context.Background(), []byte("a"), "a.ogg")
	if err != nil || text != "" {
		t.Fatalf("empty channels: text=%q err=%v", text, err)
	}
}

// --- deepgramTTS.Speak ---

func TestDeepgramTTS_Errors(t *testing.T) {
	c := &deepgramTTS{base: deepgramBase, key: "k"}
	if _, _, err := c.Speak(context.Background(), "   "); err == nil {
		t.Fatal("empty text should error")
	}
	c = &deepgramTTS{base: deepgramBase}
	if _, _, err := c.Speak(context.Background(), "hi"); err == nil {
		t.Fatal("missing key should error")
	}
}

func TestDeepgramTTS_Success(t *testing.T) {
	var gotAuth, gotQuery, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		// Suppress auto content-type to exercise the audio/mpeg default.
		w.Header()["Content-Type"] = nil
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "AUDIO")
	}))
	defer srv.Close()
	c := &deepgramTTS{base: srv.URL, model: "aura-2-thalia-en", key: "k", http: srv.Client()}
	audio, mime, err := c.Speak(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	if string(audio) != "AUDIO" || mime != "audio/mpeg" {
		t.Fatalf("audio=%q mime=%q", audio, mime)
	}
	if gotAuth != "Token k" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if !strings.Contains(gotQuery, "model=aura-2-thalia-en") {
		t.Fatalf("query = %q", gotQuery)
	}
	if !strings.Contains(gotBody, `"text":"hello"`) {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestDeepgramTTS_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		io.WriteString(w, "down")
	}))
	defer srv.Close()
	c := &deepgramTTS{base: srv.URL, key: "k", http: srv.Client()}
	_, _, err := c.Speak(context.Background(), "hi")
	if err == nil || !strings.Contains(err.Error(), "Deepgram TTS status 503") {
		t.Fatalf("expected status error, got %v", err)
	}
}
