// SPDX-License-Identifier: MIT

package voice

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestElevenLabsTTS(t *testing.T) {
	var gotPath, gotKey, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("xi-api-key")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "audio/mpeg")
		io.WriteString(w, "ID3AUDIO")
	}))
	defer srv.Close()

	tts, err := NewTTS(ProviderElevenLabs, Config{BaseURL: srv.URL, Model: "eleven_v3", Voice: "Rachel123", APIKey: "xi-k", HTTP: srv.Client()})
	if err != nil {
		t.Fatalf("NewTTS: %v", err)
	}
	audio, mime, err := tts.Speak(context.Background(), "hello")
	if err != nil {
		t.Fatalf("speak: %v", err)
	}
	if string(audio) != "ID3AUDIO" || mime != "audio/mpeg" {
		t.Fatalf("audio=%q mime=%q", audio, mime)
	}
	if gotPath != "/v1/text-to-speech/Rachel123" {
		t.Fatalf("path = %q (voice id must be in the URL path)", gotPath)
	}
	if gotKey != "xi-k" {
		t.Fatalf("xi-api-key = %q", gotKey)
	}
	if !strings.Contains(gotBody, `"model_id":"eleven_v3"`) || !strings.Contains(gotBody, `"text":"hello"`) {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestElevenLabsTTSRequiresVoice(t *testing.T) {
	tts, _ := NewTTS(ProviderElevenLabs, Config{APIKey: "k"})
	if _, _, err := tts.Speak(context.Background(), "hi"); err == nil {
		t.Fatal("ElevenLabs TTS without a voice id must error")
	}
}

func TestElevenLabsSTT(t *testing.T) {
	var gotPath, gotKey, gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("xi-api-key")
		_ = r.ParseMultipartForm(1 << 20)
		gotModel = r.FormValue("model_id")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"text":"hi there"}`)
	}))
	defer srv.Close()

	stt, err := NewSTT(ProviderElevenLabs, Config{BaseURL: srv.URL, APIKey: "xi-k", HTTP: srv.Client()})
	if err != nil {
		t.Fatalf("NewSTT: %v", err)
	}
	text, err := stt.Transcribe(context.Background(), []byte("OGG"), "note.ogg")
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if text != "hi there" {
		t.Fatalf("text = %q", text)
	}
	if gotPath != "/v1/speech-to-text" || gotKey != "xi-k" || gotModel != "scribe_v2" {
		t.Fatalf("path=%q key=%q model=%q", gotPath, gotKey, gotModel)
	}
}

func TestDeepgramSTT(t *testing.T) {
	var gotPath, gotModel, gotAuth, gotBody, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotModel = r.URL.Query().Get("model")
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"results":{"channels":[{"alternatives":[{"transcript":"deep heard you"}]}]}}`)
	}))
	defer srv.Close()

	stt, _ := NewSTT(ProviderDeepgram, Config{BaseURL: srv.URL, Model: "nova-3", APIKey: "dg", HTTP: srv.Client()})
	text, err := stt.Transcribe(context.Background(), []byte("WEBMDATA"), "note.webm")
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if text != "deep heard you" {
		t.Fatalf("text = %q", text)
	}
	if gotPath != "/v1/listen" || gotModel != "nova-3" || gotAuth != "Token dg" || gotBody != "WEBMDATA" || gotCT != "audio/webm" {
		t.Fatalf("path=%q model=%q auth=%q ct=%q body=%q", gotPath, gotModel, gotAuth, gotCT, gotBody)
	}
}

func TestDeepgramTTS(t *testing.T) {
	var gotPath, gotModel, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotModel = r.URL.Query().Get("model")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "audio/mpeg")
		io.WriteString(w, "AUDIO")
	}))
	defer srv.Close()

	tts, _ := NewTTS(ProviderDeepgram, Config{BaseURL: srv.URL, Model: "aura-2-andromeda-en", APIKey: "dg", HTTP: srv.Client()})
	audio, _, err := tts.Speak(context.Background(), "speak up")
	if err != nil {
		t.Fatalf("speak: %v", err)
	}
	if string(audio) != "AUDIO" {
		t.Fatalf("audio = %q", audio)
	}
	if gotPath != "/v1/speak" || gotModel != "aura-2-andromeda-en" || !strings.Contains(gotBody, `"text":"speak up"`) {
		t.Fatalf("path=%q model=%q body=%q", gotPath, gotModel, gotBody)
	}
}

func TestCartesiaTTS(t *testing.T) {
	var gotPath, gotKey, gotVer string
	var payload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("X-API-Key")
		gotVer = r.Header.Get("Cartesia-Version")
		_ = json.NewDecoder(r.Body).Decode(&payload)
		w.Header().Set("Content-Type", "audio/mpeg")
		io.WriteString(w, "SONIC")
	}))
	defer srv.Close()

	tts, _ := NewTTS(ProviderCartesia, Config{BaseURL: srv.URL, Model: "sonic-3.5", Voice: "voice-uuid", APIKey: "ck", HTTP: srv.Client()})
	audio, _, err := tts.Speak(context.Background(), "low latency")
	if err != nil {
		t.Fatalf("speak: %v", err)
	}
	if string(audio) != "SONIC" {
		t.Fatalf("audio = %q", audio)
	}
	if gotPath != "/tts/bytes" || gotKey != "ck" || gotVer != cartesiaVersion {
		t.Fatalf("path=%q key=%q ver=%q", gotPath, gotKey, gotVer)
	}
	if payload["model_id"] != "sonic-3.5" || payload["transcript"] != "low latency" {
		t.Fatalf("payload = %+v", payload)
	}
	voice, _ := payload["voice"].(map[string]any)
	if voice["mode"] != "id" || voice["id"] != "voice-uuid" {
		t.Fatalf("voice = %+v", payload["voice"])
	}
}

func TestFactoryRouting(t *testing.T) {
	// Unknown providers error; Cartesia has no STT half.
	if _, err := NewSTT("nope", Config{}); err == nil {
		t.Fatal("unknown STT provider must error")
	}
	if _, err := NewTTS("nope", Config{}); err == nil {
		t.Fatal("unknown TTS provider must error")
	}
	if _, err := NewSTT(ProviderCartesia, Config{}); err == nil {
		t.Fatal("Cartesia has no STT — must error")
	}
	// OpenAI requires a base URL; native providers default it.
	if _, err := NewSTT(ProviderOpenAI, Config{Model: "whisper-1"}); err == nil {
		t.Fatal("OpenAI STT without a URL must error")
	}
	if _, err := NewTTS(ProviderElevenLabs, Config{}); err != nil {
		t.Fatalf("ElevenLabs TTS should construct without a URL: %v", err)
	}
}
