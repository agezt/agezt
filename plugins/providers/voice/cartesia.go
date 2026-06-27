// SPDX-License-Identifier: MIT

package voice

// Cartesia (Sonic) is TTS-only and not OpenAI-compatible: POST /tts/bytes with
// an X-API-Key header, a mandatory Cartesia-Version date header, and a body of
// {model_id, transcript, voice:{mode,id}, output_format:{…}}. This client maps
// the OpenAI-shaped Speak seam onto it. There is no Cartesia STT.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/agezt/agezt/plugins/providers/internal/httpread"
)

const (
	cartesiaBase = "https://api.cartesia.ai"
	// cartesiaVersion pins the dated API contract Cartesia requires on every
	// request. Bump when adopting a newer Cartesia API revision.
	cartesiaVersion = "2025-04-16"
)

type cartesiaTTS struct {
	base, model, voice, key string
	http                    *http.Client
}

func (c *cartesiaTTS) Speak(ctx context.Context, text string) ([]byte, string, error) {
	if strings.TrimSpace(text) == "" {
		return nil, "", errors.New("voice: empty text")
	}
	if strings.TrimSpace(c.voice) == "" {
		return nil, "", errors.New("voice: Cartesia voice id required (set the voice to a Cartesia voice id)")
	}
	if strings.TrimSpace(c.key) == "" {
		return nil, "", errors.New("voice: Cartesia API key required")
	}
	payload := map[string]any{
		"model_id":   orDefault(c.model, "sonic-3.5"),
		"transcript": text,
		"voice":      map[string]string{"mode": "id", "id": c.voice},
		"output_format": map[string]any{
			"container":   "mp3",
			"sample_rate": 44100,
			"bit_rate":    128000,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, "", fmt.Errorf("voice: encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.base, "/")+"/tts/bytes", bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("voice: build request: %w", err)
	}
	req.Header.Set("X-API-Key", c.key)
	req.Header.Set("Cartesia-Version", cartesiaVersion)
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
		return nil, "", fmt.Errorf("voice: Cartesia TTS status %d: %s", resp.StatusCode, string(audio))
	}
	mime := resp.Header.Get("Content-Type")
	if mime == "" {
		mime = "audio/mpeg"
	}
	return audio, mime, nil
}
