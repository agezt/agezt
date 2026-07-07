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

func TestCartesiaTTS_Errors(t *testing.T) {
	// Empty text.
	c := &cartesiaTTS{base: cartesiaBase, voice: "v", key: "k"}
	if _, _, err := c.Speak(context.Background(), "  "); err == nil {
		t.Fatal("empty text should error")
	}
	// Missing voice.
	c = &cartesiaTTS{base: cartesiaBase, key: "k"}
	if _, _, err := c.Speak(context.Background(), "hi"); err == nil {
		t.Fatal("missing voice should error")
	}
	// Missing key.
	c = &cartesiaTTS{base: cartesiaBase, voice: "v"}
	if _, _, err := c.Speak(context.Background(), "hi"); err == nil {
		t.Fatal("missing key should error")
	}
}

func TestCartesiaTTS_Success(t *testing.T) {
	var gotKey, gotVer, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-API-Key")
		gotVer = r.Header.Get("Cartesia-Version")
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "audio/mpeg")
		io.WriteString(w, "CARTAUDIO")
	}))
	defer srv.Close()
	c := &cartesiaTTS{base: srv.URL, model: "sonic-3.5", voice: "voiceX", key: "kk", http: srv.Client()}
	audio, mimeType, err := c.Speak(context.Background(), "hello sonic")
	if err != nil {
		t.Fatal(err)
	}
	if string(audio) != "CARTAUDIO" || mimeType != "audio/mpeg" {
		t.Fatalf("audio=%q mime=%q", audio, mimeType)
	}
	if gotKey != "kk" {
		t.Fatalf("X-API-Key = %q", gotKey)
	}
	if gotVer != cartesiaVersion {
		t.Fatalf("Cartesia-Version = %q", gotVer)
	}
	if !strings.HasSuffix(gotPath, "/tts/bytes") {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.Contains(gotBody, `"transcript":"hello sonic"`) {
		t.Fatalf("body = %q", gotBody)
	}
	if !strings.Contains(gotBody, `"id":"voiceX"`) {
		t.Fatalf("body missing voice id: %q", gotBody)
	}
}

func TestCartesiaTTS_DefaultMime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header()["Content-Type"] = nil
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "A")
	}))
	defer srv.Close()
	c := &cartesiaTTS{base: srv.URL, voice: "v", key: "k", http: srv.Client()}
	_, mimeType, err := c.Speak(context.Background(), "hi")
	if err != nil || mimeType != "audio/mpeg" {
		t.Fatalf("mime=%q err=%v", mimeType, err)
	}
}

func TestCartesiaTTS_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		io.WriteString(w, "pay up")
	}))
	defer srv.Close()
	c := &cartesiaTTS{base: srv.URL, voice: "v", key: "k", http: srv.Client()}
	_, _, err := c.Speak(context.Background(), "hi")
	if err == nil || !strings.Contains(err.Error(), "Cartesia TTS status 402") {
		t.Fatalf("expected status error, got %v", err)
	}
}
