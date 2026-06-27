// SPDX-License-Identifier: MIT

package voice

// Deepgram is not OpenAI-compatible for audio: STT is POST /v1/listen with the
// model as a query parameter, raw audio as the body, "Authorization: Token …"
// auth, and a Deepgram-shaped JSON response; TTS is POST /v1/speak with the
// voice baked into the model name (e.g. aura-2-thalia-en) and a {text} body.
// These clients translate the OpenAI-shaped seam to that native API.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/plugins/providers/internal/httpread"
)

const deepgramBase = "https://api.deepgram.com"

// audioContentType guesses a request Content-Type from the audio filename so
// Deepgram's container detection has a hint (it can also sniff). Defaults to a
// generic octet-stream, which Deepgram tolerates.
func audioContentType(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".ogg", ".opus", ".oga":
		return "audio/ogg"
	case ".webm":
		return "audio/webm"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".m4a", ".mp4":
		return "audio/mp4"
	case ".flac":
		return "audio/flac"
	default:
		return "application/octet-stream"
	}
}

// deepgramSTT transcribes via POST <base>/v1/listen?model=…
type deepgramSTT struct {
	base, model, key string
	http             *http.Client
}

func (c *deepgramSTT) Transcribe(ctx context.Context, audio []byte, filename string) (string, error) {
	if len(audio) == 0 {
		return "", errors.New("voice: empty audio")
	}
	if len(audio) > maxAudioBytes {
		return "", fmt.Errorf("voice: audio too large (%d bytes, max %d)", len(audio), maxAudioBytes)
	}
	if strings.TrimSpace(c.key) == "" {
		return "", errors.New("voice: Deepgram API key required")
	}
	q := url.Values{}
	q.Set("model", orDefault(c.model, "nova-3"))
	q.Set("smart_format", "true")
	q.Set("punctuate", "true")
	endpoint := strings.TrimRight(c.base, "/") + "/v1/listen?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(audio))
	if err != nil {
		return "", fmt.Errorf("voice: build request: %w", err)
	}
	req.Header.Set("Authorization", "Token "+c.key)
	req.Header.Set("Content-Type", audioContentType(filename))
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
		return "", fmt.Errorf("voice: Deepgram STT status %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Results struct {
			Channels []struct {
				Alternatives []struct {
					Transcript string `json:"transcript"`
				} `json:"alternatives"`
			} `json:"channels"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("voice: decode: %w", err)
	}
	if len(out.Results.Channels) == 0 || len(out.Results.Channels[0].Alternatives) == 0 {
		return "", nil
	}
	return strings.TrimSpace(out.Results.Channels[0].Alternatives[0].Transcript), nil
}

// deepgramTTS speaks via POST <base>/v1/speak?model=… The voice is part of the
// model id (e.g. aura-2-thalia-en), so there is no separate voice field.
type deepgramTTS struct {
	base, model, key string
	http             *http.Client
}

func (c *deepgramTTS) Speak(ctx context.Context, text string) ([]byte, string, error) {
	if strings.TrimSpace(text) == "" {
		return nil, "", errors.New("voice: empty text")
	}
	if strings.TrimSpace(c.key) == "" {
		return nil, "", errors.New("voice: Deepgram API key required")
	}
	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return nil, "", fmt.Errorf("voice: encode: %w", err)
	}
	q := url.Values{}
	q.Set("model", orDefault(c.model, "aura-2-thalia-en"))
	endpoint := strings.TrimRight(c.base, "/") + "/v1/speak?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("voice: build request: %w", err)
	}
	req.Header.Set("Authorization", "Token "+c.key)
	req.Header.Set("Content-Type", "application/json")
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
		return nil, "", fmt.Errorf("voice: Deepgram TTS status %d: %s", resp.StatusCode, string(audio))
	}
	mime := resp.Header.Get("Content-Type")
	if mime == "" {
		mime = "audio/mpeg"
	}
	return audio, mime, nil
}
