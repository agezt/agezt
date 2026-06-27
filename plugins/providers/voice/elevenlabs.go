// SPDX-License-Identifier: MIT

package voice

// ElevenLabs is not OpenAI-compatible for audio: TTS puts the voice in the URL
// path (/v1/text-to-speech/{voice_id}) with a {text, model_id} body and an
// xi-api-key header, and STT is /v1/speech-to-text with a model_id form field.
// These two clients translate the OpenAI-shaped seam (Speak / Transcribe) to
// ElevenLabs' native API so the rest of AGEZT doesn't have to care.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/agezt/agezt/plugins/providers/internal/httpread"
)

const elevenLabsBase = "https://api.elevenlabs.io"

// elevenLabsTTS speaks via POST <base>/v1/text-to-speech/{voice_id}.
type elevenLabsTTS struct {
	base, model, voice, key string
	http                    *http.Client
}

func (c *elevenLabsTTS) Speak(ctx context.Context, text string) ([]byte, string, error) {
	if strings.TrimSpace(text) == "" {
		return nil, "", errors.New("voice: empty text")
	}
	if strings.TrimSpace(c.voice) == "" {
		return nil, "", errors.New("voice: ElevenLabs voice id required (set the voice to a voice_id from your library)")
	}
	if strings.TrimSpace(c.key) == "" {
		return nil, "", errors.New("voice: ElevenLabs API key required")
	}
	body, err := json.Marshal(map[string]string{"text": text, "model_id": orDefault(c.model, "eleven_multilingual_v2")})
	if err != nil {
		return nil, "", fmt.Errorf("voice: encode: %w", err)
	}
	endpoint := strings.TrimRight(c.base, "/") + "/v1/text-to-speech/" + url.PathEscape(c.voice) + "?output_format=mp3_44100_128"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("voice: build request: %w", err)
	}
	req.Header.Set("xi-api-key", c.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/mpeg")
	resp, err := httpClient(c.http).Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("voice: http: %w", err)
	}
	defer resp.Body.Close()
	audio, err := httpread.All(resp.Body, maxAudioBytes)
	if err != nil {
		return nil, "", fmt.Errorf("voice: read body: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, "", fmt.Errorf("voice: ElevenLabs TTS status %d: %s", resp.StatusCode, string(audio))
	}
	mime := resp.Header.Get("Content-Type")
	if mime == "" {
		mime = "audio/mpeg"
	}
	return audio, mime, nil
}

// elevenLabsSTT transcribes via POST <base>/v1/speech-to-text (multipart).
type elevenLabsSTT struct {
	base, model, key string
	http             *http.Client
}

func (c *elevenLabsSTT) Transcribe(ctx context.Context, audio []byte, filename string) (string, error) {
	if len(audio) == 0 {
		return "", errors.New("voice: empty audio")
	}
	if len(audio) > maxAudioBytes {
		return "", fmt.Errorf("voice: audio too large (%d bytes, max %d)", len(audio), maxAudioBytes)
	}
	if strings.TrimSpace(c.key) == "" {
		return "", errors.New("voice: ElevenLabs API key required")
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
	if err := mw.WriteField("model_id", orDefault(c.model, "scribe_v2")); err != nil {
		return "", fmt.Errorf("voice: multipart field: %w", err)
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("voice: multipart close: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.base, "/")+"/v1/speech-to-text", &body)
	if err != nil {
		return "", fmt.Errorf("voice: build request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("xi-api-key", c.key)
	resp, err := httpClient(c.http).Do(req)
	if err != nil {
		return "", fmt.Errorf("voice: http: %w", err)
	}
	defer resp.Body.Close()
	raw, err := httpread.All(resp.Body, httpread.DefaultMaxResponseBytes)
	if err != nil {
		return "", fmt.Errorf("voice: read body: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("voice: ElevenLabs STT status %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("voice: decode: %w", err)
	}
	return strings.TrimSpace(out.Text), nil
}
