// SPDX-License-Identifier: MIT

// Package stt is a minimal speech-to-text client for the OpenAI-compatible
// `/v1/audio/transcriptions` endpoint, using net/http only — no dependency. The
// same API is spoken by OpenAI, Groq, and a locally-run whisper.cpp server, so
// pointing `AGEZT_STT_API_URL` at any of them transcribes audio the same way.
// It is the foundation of Agezt's voice input: `agt transcribe <file>` and
// `agt listen` (mic) both turn audio into text here, then drive the agent.
package stt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// sttRespMaxBytes bounds the JSON response — a transcript of a short clip is
// small text; this refuses a runaway body from a buggy or MITM'd endpoint.
const sttRespMaxBytes = 4 << 20

// Config constructs a Client.
type Config struct {
	APIURL     string // base, e.g. https://api.openai.com/v1 (default if empty)
	APIKey     string // bearer token for the STT provider
	Model      string // transcription model (default "whisper-1")
	HTTPClient *http.Client
}

// Client transcribes audio via an OpenAI-compatible endpoint.
type Client struct {
	base   string
	key    string
	model  string
	client *http.Client
}

// New builds a Client, applying defaults (OpenAI base + whisper-1).
func New(cfg Config) *Client {
	base := strings.TrimRight(strings.TrimSpace(cfg.APIURL), "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "whisper-1"
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second} // transcription can be slow
	}
	return &Client{base: base, key: cfg.APIKey, model: model, client: client}
}

// Model reports the configured transcription model (for status/logging).
func (c *Client) Model() string { return c.model }

// Transcribe sends audio (named filename, so the server infers the format) to
// `<base>/audio/transcriptions` and returns the recognised text.
func (c *Client) Transcribe(ctx context.Context, filename string, audio []byte) (string, error) {
	if len(audio) == 0 {
		return "", fmt.Errorf("stt: empty audio")
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(audio); err != nil {
		return "", err
	}
	if err := mw.WriteField("model", c.model); err != nil {
		return "", err
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/audio/transcriptions", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", c.scrub(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, sttRespMaxBytes))
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("stt: transcription failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Text  string `json:"text"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("stt: decode response: %w", err)
	}
	if out.Error != nil && out.Error.Message != "" {
		return "", fmt.Errorf("stt: %s", out.Error.Message)
	}
	return strings.TrimSpace(out.Text), nil
}

// scrub removes the API key from an error message — defense in depth.
func (c *Client) scrub(err error) error {
	if err == nil || c.key == "" {
		return err
	}
	if msg := err.Error(); strings.Contains(msg, c.key) {
		return fmt.Errorf("%s", strings.ReplaceAll(msg, c.key, "<redacted>"))
	}
	return err
}
