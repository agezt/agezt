// SPDX-License-Identifier: MIT

// Package voice is the OpenAI-compatible voice adapter — speech-to-text
// (transcription) and text-to-speech (synthesis) over the same wire shape the
// embeddings adapter uses (see plugins/providers/embed). One adapter covers
// api.openai.com, any OpenAI-compatible gateway, and a local server (e.g.
// faster-whisper / Kokoro / Piper behind an OpenAI shim), so an agent can hear
// voice notes and speak replies with a handful of settings and no extra deps.
//
// The kernel never imports this package (kernel-never-imports-plugins); the
// daemon builds an Adapter from AGEZT_STT_* / AGEZT_TTS_* and injects it via
// runtime.Config.Voice. Either half is independent: configure STT only, TTS
// only, or both. Unset → no voice tool is registered.
package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/netguard"
	"github.com/agezt/agezt/plugins/providers/internal/httpread"
)

// DefaultTimeout caps one voice request. Transcription/synthesis of a short
// message is quick even on a cold local model; two minutes is generous.
const DefaultTimeout = 2 * time.Minute

// maxAudioBytes bounds an audio payload (in or out) so a single request can't
// blow the response cap or memory. 25 MiB matches OpenAI's upload limit.
const maxAudioBytes = 25 << 20

// STTClient transcribes audio via POST <BaseURL>/audio/transcriptions.
type STTClient struct {
	BaseURL string // API root, with or without /v1
	Model   string // e.g. "whisper-1", "Systran/faster-whisper-base"
	APIKey  string // Bearer token when non-empty (local servers need none)
	HTTP    *http.Client
}

// TTSClient synthesizes speech via POST <BaseURL>/audio/speech.
type TTSClient struct {
	BaseURL string // API root, with or without /v1
	Model   string // e.g. "tts-1", "kokoro"
	Voice   string // e.g. "alloy"; defaults to "alloy" when empty
	APIKey  string // Bearer token when non-empty
	HTTP    *http.Client
}

// STTBackend transcribes audio → text. Implemented by the OpenAI-compatible
// STTClient and by native providers that speak their own wire shape (ElevenLabs
// Scribe, Deepgram Listen).
type STTBackend interface {
	Transcribe(ctx context.Context, audio []byte, filename string) (string, error)
}

// TTSBackend synthesizes text → audio bytes + MIME type. Implemented by the
// OpenAI-compatible TTSClient and by native providers (ElevenLabs, Deepgram
// Aura, Cartesia Sonic).
type TTSBackend interface {
	Speak(ctx context.Context, text string) ([]byte, string, error)
}

// Adapter bundles the optional STT and TTS halves. Either may be nil. The halves
// are backends, so a hosted OpenAI-compatible endpoint and a native provider
// (ElevenLabs/Deepgram/Cartesia) are interchangeable behind the same seam.
type Adapter struct {
	STT STTBackend
	TTS TTSBackend
}

// Provider identifiers for NewSTT/NewTTS (case-insensitive; "" == openai).
const (
	ProviderOpenAI     = "openai"
	ProviderElevenLabs = "elevenlabs"
	ProviderDeepgram   = "deepgram"
	ProviderCartesia   = "cartesia"
)

// Config carries the settings for one half. BaseURL is optional for native
// providers — each supplies its own default API root.
type Config struct {
	BaseURL string
	Model   string
	Voice   string
	APIKey  string
	HTTP    *http.Client
}

func normProvider(p string) string {
	p = strings.ToLower(strings.TrimSpace(p))
	if p == "" {
		return ProviderOpenAI
	}
	return p
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// NewSTT builds a transcription backend for the named provider. OpenAI (and any
// OpenAI-compatible endpoint) needs a BaseURL; native providers default it.
func NewSTT(provider string, cfg Config) (STTBackend, error) {
	switch normProvider(provider) {
	case ProviderOpenAI:
		if strings.TrimSpace(cfg.BaseURL) == "" {
			return nil, errors.New("voice: STT URL required")
		}
		return &STTClient{BaseURL: cfg.BaseURL, Model: cfg.Model, APIKey: cfg.APIKey, HTTP: cfg.HTTP}, nil
	case ProviderElevenLabs:
		return &elevenLabsSTT{base: orDefault(cfg.BaseURL, elevenLabsBase), model: orDefault(cfg.Model, "scribe_v2"), key: cfg.APIKey, http: cfg.HTTP}, nil
	case ProviderDeepgram:
		return &deepgramSTT{base: orDefault(cfg.BaseURL, deepgramBase), model: orDefault(cfg.Model, "nova-3"), key: cfg.APIKey, http: cfg.HTTP}, nil
	default:
		return nil, fmt.Errorf("voice: unknown STT provider %q", provider)
	}
}

// NewTTS builds a synthesis backend for the named provider.
func NewTTS(provider string, cfg Config) (TTSBackend, error) {
	switch normProvider(provider) {
	case ProviderOpenAI:
		if strings.TrimSpace(cfg.BaseURL) == "" {
			return nil, errors.New("voice: TTS URL required")
		}
		return &TTSClient{BaseURL: cfg.BaseURL, Model: cfg.Model, Voice: cfg.Voice, APIKey: cfg.APIKey, HTTP: cfg.HTTP}, nil
	case ProviderElevenLabs:
		return &elevenLabsTTS{base: orDefault(cfg.BaseURL, elevenLabsBase), model: orDefault(cfg.Model, "eleven_multilingual_v2"), voice: cfg.Voice, key: cfg.APIKey, http: cfg.HTTP}, nil
	case ProviderDeepgram:
		return &deepgramTTS{base: orDefault(cfg.BaseURL, deepgramBase), model: orDefault(cfg.Model, "aura-2-thalia-en"), key: cfg.APIKey, http: cfg.HTTP}, nil
	case ProviderCartesia:
		return &cartesiaTTS{base: orDefault(cfg.BaseURL, cartesiaBase), model: orDefault(cfg.Model, "sonic-3.5"), voice: cfg.Voice, key: cfg.APIKey, http: cfg.HTTP}, nil
	default:
		return nil, fmt.Errorf("voice: unknown TTS provider %q", provider)
	}
}

// HasSTT reports whether transcription is configured.
func (a *Adapter) HasSTT() bool { return a != nil && a.STT != nil }

// HasTTS reports whether synthesis is configured.
func (a *Adapter) HasTTS() bool { return a != nil && a.TTS != nil }

// Transcribe implements the runtime voice seam: audio bytes → text.
func (a *Adapter) Transcribe(ctx context.Context, audio []byte, filename string) (string, error) {
	if !a.HasSTT() {
		return "", errors.New("voice: transcription not configured")
	}
	return a.STT.Transcribe(ctx, audio, filename)
}

// Speak implements the runtime voice seam: text → audio bytes + MIME type.
func (a *Adapter) Speak(ctx context.Context, text string) ([]byte, string, error) {
	if !a.HasTTS() {
		return nil, "", errors.New("voice: synthesis not configured")
	}
	return a.TTS.Speak(ctx, text)
}

func endpoint(base, path string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(base, "/v1") || strings.Contains(base, "/v1/") {
		return base + path
	}
	return base + "/v1" + path
}

func httpClient(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	// Netguard-protected for parity with every other provider adapter: link-local
	// / cloud-metadata (169.254.169.254) and other dangerous ranges are blocked on
	// the initial dial and each redirect hop. Loopback and private ranges are
	// intentionally ALLOWED — a local STT/TTS server (faster-whisper / Kokoro /
	// Piper behind an OpenAI shim at http://localhost:…) is a documented, first-
	// class destination for this adapter.
	return netguard.New(netguard.AllowLoopback(), netguard.AllowPrivate()).HTTPClient(DefaultTimeout)
}

// Transcribe POSTs the audio as multipart/form-data and returns the text.
func (c *STTClient) Transcribe(ctx context.Context, audio []byte, filename string) (string, error) {
	if c.Model == "" {
		return "", errors.New("voice: STT model required")
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		return "", errors.New("voice: STT base URL required")
	}
	if len(audio) == 0 {
		return "", errors.New("voice: empty audio")
	}
	if len(audio) > maxAudioBytes {
		return "", fmt.Errorf("voice: audio too large (%d bytes, max %d)", len(audio), maxAudioBytes)
	}
	if strings.TrimSpace(filename) == "" {
		filename = "audio.ogg"
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("voice: multipart: %w", err)
	}
	if _, err := fw.Write(audio); err != nil {
		return "", fmt.Errorf("voice: multipart write: %w", err)
	}
	if err := mw.WriteField("model", c.Model); err != nil {
		return "", fmt.Errorf("voice: multipart field: %w", err)
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("voice: multipart close: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(c.BaseURL, "/audio/transcriptions"), &body)
	if err != nil {
		return "", fmt.Errorf("voice: build request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := httpClient(c.HTTP).Do(req)
	if err != nil {
		return "", fmt.Errorf("voice: http: %w", err)
	}
	defer resp.Body.Close()
	respBytes, err := httpread.All(resp.Body, httpread.DefaultMaxResponseBytes)
	if err != nil {
		return "", fmt.Errorf("voice: read body: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("voice: STT status %d: %s", resp.StatusCode, string(respBytes))
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(respBytes, &out); err != nil {
		return "", fmt.Errorf("voice: decode: %w", err)
	}
	return strings.TrimSpace(out.Text), nil
}

type speakRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
	Voice string `json:"voice"`
}

// Speak POSTs the text and returns the synthesized audio bytes + MIME type.
func (c *TTSClient) Speak(ctx context.Context, text string) ([]byte, string, error) {
	if c.Model == "" {
		return nil, "", errors.New("voice: TTS model required")
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		return nil, "", errors.New("voice: TTS base URL required")
	}
	if strings.TrimSpace(text) == "" {
		return nil, "", errors.New("voice: empty text")
	}
	voiceName := c.Voice
	if voiceName == "" {
		voiceName = "alloy"
	}
	body, err := json.Marshal(speakRequest{Model: c.Model, Input: text, Voice: voiceName})
	if err != nil {
		return nil, "", fmt.Errorf("voice: encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(c.BaseURL, "/audio/speech"), bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("voice: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := httpClient(c.HTTP).Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("voice: http: %w", err)
	}
	defer resp.Body.Close()
	audio, err := httpread.All(resp.Body, maxAudioBytes)
	if err != nil {
		return nil, "", fmt.Errorf("voice: read body: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, "", fmt.Errorf("voice: TTS status %d: %s", resp.StatusCode, string(audio))
	}
	mime := resp.Header.Get("Content-Type")
	if mime == "" {
		mime = "audio/mpeg"
	}
	return audio, mime, nil
}
