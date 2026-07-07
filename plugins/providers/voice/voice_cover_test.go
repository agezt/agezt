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

// --- normProvider ---

func TestNormProvider(t *testing.T) {
	if got := normProvider(""); got != ProviderOpenAI {
		t.Fatalf("empty = %q, want openai", got)
	}
	if got := normProvider("  "); got != ProviderOpenAI {
		t.Fatalf("blank = %q, want openai", got)
	}
	if got := normProvider("ElevenLabs"); got != ProviderElevenLabs {
		t.Fatalf("mixed case = %q", got)
	}
	if got := normProvider("weird"); got != "weird" {
		t.Fatalf("passthrough = %q", got)
	}
}

// --- NewSTT ---

func TestNewSTT_Providers(t *testing.T) {
	// OpenAI needs a BaseURL.
	if _, err := NewSTT(ProviderOpenAI, Config{}); err == nil {
		t.Fatal("openai STT without URL should error")
	}
	if b, err := NewSTT(ProviderOpenAI, Config{BaseURL: "http://x"}); err != nil || b == nil {
		t.Fatalf("openai STT: %v", err)
	}
	// Native providers default their base.
	if b, err := NewSTT(ProviderElevenLabs, Config{}); err != nil || b == nil {
		t.Fatalf("elevenlabs STT: %v", err)
	}
	if b, err := NewSTT(ProviderDeepgram, Config{}); err != nil || b == nil {
		t.Fatalf("deepgram STT: %v", err)
	}
	// Unknown provider errors.
	if _, err := NewSTT("bogus", Config{}); err == nil {
		t.Fatal("unknown STT provider should error")
	}
	// Empty provider → openai (needs URL).
	if _, err := NewSTT("", Config{}); err == nil {
		t.Fatal("empty provider defaults to openai, needs URL")
	}
}

// --- NewTTS ---

func TestNewTTS_Providers(t *testing.T) {
	if _, err := NewTTS(ProviderOpenAI, Config{}); err == nil {
		t.Fatal("openai TTS without URL should error")
	}
	if b, err := NewTTS(ProviderOpenAI, Config{BaseURL: "http://x"}); err != nil || b == nil {
		t.Fatalf("openai TTS: %v", err)
	}
	if b, err := NewTTS(ProviderElevenLabs, Config{}); err != nil || b == nil {
		t.Fatalf("elevenlabs TTS: %v", err)
	}
	if b, err := NewTTS(ProviderDeepgram, Config{}); err != nil || b == nil {
		t.Fatalf("deepgram TTS: %v", err)
	}
	if b, err := NewTTS(ProviderCartesia, Config{}); err != nil || b == nil {
		t.Fatalf("cartesia TTS: %v", err)
	}
	if _, err := NewTTS("bogus", Config{}); err == nil {
		t.Fatal("unknown TTS provider should error")
	}
}

// --- STTClient.Transcribe error branches ---

func TestSTTClient_Transcribe_Errors(t *testing.T) {
	// Missing model.
	c := &STTClient{BaseURL: "http://x"}
	if _, err := c.Transcribe(context.Background(), []byte("a"), "a.ogg"); err == nil {
		t.Fatal("missing model should error")
	}
	// Missing base URL.
	c = &STTClient{Model: "whisper-1"}
	if _, err := c.Transcribe(context.Background(), []byte("a"), "a.ogg"); err == nil {
		t.Fatal("missing base URL should error")
	}
	// Empty audio.
	c = &STTClient{BaseURL: "http://x", Model: "whisper-1"}
	if _, err := c.Transcribe(context.Background(), nil, "a.ogg"); err == nil {
		t.Fatal("empty audio should error")
	}
	// Too-large audio.
	big := make([]byte, maxAudioBytes+1)
	if _, err := c.Transcribe(context.Background(), big, "a.ogg"); err == nil {
		t.Fatal("too-large audio should error")
	}
}

func TestSTTClient_Transcribe_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "bad request")
	}))
	defer srv.Close()
	c := &STTClient{BaseURL: srv.URL, Model: "whisper-1", HTTP: srv.Client()}
	_, err := c.Transcribe(context.Background(), []byte("audio"), "a.ogg")
	if err == nil || !strings.Contains(err.Error(), "STT status 400") {
		t.Fatalf("expected STT status error, got %v", err)
	}
}

func TestSTTClient_Transcribe_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{not json`)
	}))
	defer srv.Close()
	c := &STTClient{BaseURL: srv.URL, Model: "whisper-1", HTTP: srv.Client()}
	_, err := c.Transcribe(context.Background(), []byte("audio"), "a.ogg")
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected decode error, got %v", err)
	}
}

func TestSTTClient_Transcribe_APIKeyAndDefaultFilename(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"text":" hi "}`)
	}))
	defer srv.Close()
	c := &STTClient{BaseURL: srv.URL, Model: "whisper-1", APIKey: "sekret", HTTP: srv.Client()}
	// Empty filename → default "audio.ogg" branch.
	text, err := c.Transcribe(context.Background(), []byte("audio"), "  ")
	if err != nil {
		t.Fatal(err)
	}
	if text != "hi" { // trimmed
		t.Fatalf("text = %q, want trimmed hi", text)
	}
	if gotAuth != "Bearer sekret" {
		t.Fatalf("auth = %q", gotAuth)
	}
}

// --- TTSClient.Speak branches ---

func TestTTSClient_Speak_Errors(t *testing.T) {
	// Missing model.
	c := &TTSClient{BaseURL: "http://x"}
	if _, _, err := c.Speak(context.Background(), "hi"); err == nil {
		t.Fatal("missing model should error")
	}
	// Empty text.
	c = &TTSClient{BaseURL: "http://x", Model: "tts-1"}
	if _, _, err := c.Speak(context.Background(), "   "); err == nil {
		t.Fatal("empty text should error")
	}
}

func TestTTSClient_Speak_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "boom")
	}))
	defer srv.Close()
	c := &TTSClient{BaseURL: srv.URL + "/v1", Model: "tts-1", HTTP: srv.Client()}
	_, _, err := c.Speak(context.Background(), "hi")
	if err == nil || !strings.Contains(err.Error(), "TTS status 500") {
		t.Fatalf("expected TTS status error, got %v", err)
	}
}

func TestTTSClient_Speak_DefaultVoiceAndMime(t *testing.T) {
	var gotVoice, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), `"voice":"alloy"`) {
			gotVoice = "alloy"
		}
		// Suppress the auto-detected Content-Type so Speak takes its
		// mime == "" default (audio/mpeg) branch.
		w.Header()["Content-Type"] = nil
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "AUDIODATA")
	}))
	defer srv.Close()
	// Voice empty → defaults to "alloy"; APIKey set → Bearer header.
	c := &TTSClient{BaseURL: srv.URL + "/v1", Model: "tts-1", APIKey: "k", HTTP: srv.Client()}
	audio, mime, err := c.Speak(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	if string(audio) != "AUDIODATA" {
		t.Fatalf("audio = %q", audio)
	}
	if mime != "audio/mpeg" {
		t.Fatalf("mime = %q, want default audio/mpeg", mime)
	}
	if gotVoice != "alloy" {
		t.Fatal("voice should default to alloy")
	}
	if gotAuth != "Bearer k" {
		t.Fatalf("auth = %q", gotAuth)
	}
}

// --- endpoint ---

func TestEndpoint(t *testing.T) {
	cases := map[string]string{
		"http://x":        "http://x/v1/audio/speech",
		"http://x/":       "http://x/v1/audio/speech",
		"http://x/v1":     "http://x/v1/audio/speech",
		"http://x/v1/":    "http://x/v1/audio/speech",
		"http://x/v1/sub": "http://x/v1/sub/audio/speech",
	}
	for base, want := range cases {
		if got := endpoint(base, "/audio/speech"); got != want {
			t.Errorf("endpoint(%q) = %q, want %q", base, got, want)
		}
	}
}

// --- httpClient default ---

func TestHTTPClient_Default(t *testing.T) {
	if c := httpClient(nil); c == nil {
		t.Fatal("nil client should yield a netguard-backed default")
	}
	custom := &http.Client{}
	if c := httpClient(custom); c != custom {
		t.Fatal("non-nil client should be returned unchanged")
	}
}

// --- Adapter not-configured errors ---

func TestAdapter_NotConfigured(t *testing.T) {
	a := &Adapter{}
	if a.HasSTT() {
		t.Fatal("empty adapter should not have STT")
	}
	if a.HasTTS() {
		t.Fatal("empty adapter should not have TTS")
	}
	if _, err := a.Transcribe(context.Background(), []byte("a"), "a.ogg"); err == nil {
		t.Fatal("Transcribe without STT should error")
	}
	if _, _, err := a.Speak(context.Background(), "hi"); err == nil {
		t.Fatal("Speak without TTS should error")
	}
}

// TestAdapter_Configured covers the happy-path delegation through Adapter to
// the underlying STT/TTS backends.
func TestAdapter_Configured(t *testing.T) {
	sttSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"text":"heard"}`)
	}))
	defer sttSrv.Close()
	ttsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		io.WriteString(w, "SND")
	}))
	defer ttsSrv.Close()

	a := &Adapter{
		STT: &STTClient{BaseURL: sttSrv.URL, Model: "whisper-1", HTTP: sttSrv.Client()},
		TTS: &TTSClient{BaseURL: ttsSrv.URL + "/v1", Model: "tts-1", HTTP: ttsSrv.Client()},
	}
	if !a.HasSTT() || !a.HasTTS() {
		t.Fatal("adapter should report both configured")
	}
	text, err := a.Transcribe(context.Background(), []byte("audio"), "a.ogg")
	if err != nil || text != "heard" {
		t.Fatalf("Transcribe: text=%q err=%v", text, err)
	}
	audio, mimeType, err := a.Speak(context.Background(), "hi")
	if err != nil || string(audio) != "SND" || mimeType != "audio/mpeg" {
		t.Fatalf("Speak: audio=%q mime=%q err=%v", audio, mimeType, err)
	}
}

// TestClients_HTTPError covers the transport-error branch (`voice: http:` /
// `voice: read body`) by pointing at a closed server.
func TestClients_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // now connections are refused

	stt := &STTClient{BaseURL: url, Model: "whisper-1", HTTP: &http.Client{}}
	if _, err := stt.Transcribe(context.Background(), []byte("a"), "a.ogg"); err == nil {
		t.Fatal("STT against closed server should error")
	}
	tts := &TTSClient{BaseURL: url + "/v1", Model: "tts-1", HTTP: &http.Client{}}
	if _, _, err := tts.Speak(context.Background(), "hi"); err == nil {
		t.Fatal("TTS against closed server should error")
	}
	dstt := &deepgramSTT{base: url, key: "k", http: &http.Client{}}
	if _, err := dstt.Transcribe(context.Background(), []byte("a"), "a.ogg"); err == nil {
		t.Fatal("deepgram STT against closed server should error")
	}
	dtts := &deepgramTTS{base: url, key: "k", http: &http.Client{}}
	if _, _, err := dtts.Speak(context.Background(), "hi"); err == nil {
		t.Fatal("deepgram TTS against closed server should error")
	}
	estt := &elevenLabsSTT{base: url, key: "k", http: &http.Client{}}
	if _, err := estt.Transcribe(context.Background(), []byte("a"), "a.ogg"); err == nil {
		t.Fatal("elevenlabs STT against closed server should error")
	}
	etts := &elevenLabsTTS{base: url, voice: "v", key: "k", http: &http.Client{}}
	if _, _, err := etts.Speak(context.Background(), "hi"); err == nil {
		t.Fatal("elevenlabs TTS against closed server should error")
	}
	ctts := &cartesiaTTS{base: url, voice: "v", key: "k", http: &http.Client{}}
	if _, _, err := ctts.Speak(context.Background(), "hi"); err == nil {
		t.Fatal("cartesia TTS against closed server should error")
	}
}

// TestClients_BuildRequestError covers the `http.NewRequestWithContext` failure
// branch by supplying a base URL containing a control character (which the URL
// parser rejects).
func TestClients_BuildRequestError(t *testing.T) {
	bad := "http://exa\x7fmple" // DEL control byte → invalid URL

	stt := &STTClient{BaseURL: bad, Model: "whisper-1", HTTP: &http.Client{}}
	if _, err := stt.Transcribe(context.Background(), []byte("a"), "a.ogg"); err == nil {
		t.Fatal("STT with bad URL should fail to build request")
	}
	tts := &TTSClient{BaseURL: bad, Model: "tts-1", HTTP: &http.Client{}}
	if _, _, err := tts.Speak(context.Background(), "hi"); err == nil {
		t.Fatal("TTS with bad URL should fail to build request")
	}
	dstt := &deepgramSTT{base: bad, key: "k", http: &http.Client{}}
	if _, err := dstt.Transcribe(context.Background(), []byte("a"), "a.ogg"); err == nil {
		t.Fatal("deepgram STT with bad URL should fail to build request")
	}
	dtts := &deepgramTTS{base: bad, key: "k", http: &http.Client{}}
	if _, _, err := dtts.Speak(context.Background(), "hi"); err == nil {
		t.Fatal("deepgram TTS with bad URL should fail to build request")
	}
	estt := &elevenLabsSTT{base: bad, key: "k", http: &http.Client{}}
	if _, err := estt.Transcribe(context.Background(), []byte("a"), "a.ogg"); err == nil {
		t.Fatal("elevenlabs STT with bad URL should fail to build request")
	}
	etts := &elevenLabsTTS{base: bad, voice: "v", key: "k", http: &http.Client{}}
	if _, _, err := etts.Speak(context.Background(), "hi"); err == nil {
		t.Fatal("elevenlabs TTS with bad URL should fail to build request")
	}
	ctts := &cartesiaTTS{base: bad, voice: "v", key: "k", http: &http.Client{}}
	if _, _, err := ctts.Speak(context.Background(), "hi"); err == nil {
		t.Fatal("cartesia TTS with bad URL should fail to build request")
	}
}
