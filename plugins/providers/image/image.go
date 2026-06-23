// SPDX-License-Identifier: MIT

// Package image is the OpenAI-compatible image-generation client (M997) — the
// image-modality sibling of plugins/providers/embed and plugins/providers/voice.
// One wire shape (POST <BaseURL>/images/generations with response_format=
// b64_json) covers OpenAI (dall-e-3, gpt-image-1) and the many OpenAI-compatible
// image gateways. The kernel never imports this package; the daemon builds a
// Client from AGEZT_IMAGE_URL / AGEZT_IMAGE_MODEL / AGEZT_IMAGE_KEY and injects
// it via runtime.Config.ImageGenerator. Unset → no image_generate tool.
package image

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/agezt/agezt/plugins/providers/internal/httpread"
)

// DefaultTimeout caps one image request. Generation is slower than chat, so the
// budget is generous.
const DefaultTimeout = 2 * time.Minute

// Client generates images over POST <BaseURL>/images/generations.
type Client struct {
	// BaseURL is the API root, with or without the /v1 suffix.
	BaseURL string
	// Model is the image model id (e.g. "dall-e-3", "gpt-image-1"). Required.
	Model string
	// APIKey is sent as a Bearer token when non-empty.
	APIKey string
	// HTTP overrides the default client (tests). Nil → DefaultTimeout.
	HTTP *http.Client
}

// New constructs a Client with the default HTTP timeout.
func New(baseURL, model, apiKey string) *Client {
	return &Client{BaseURL: baseURL, Model: model, APIKey: apiKey, HTTP: &http.Client{Timeout: DefaultTimeout}}
}

// HasImage reports whether the client is configured enough to generate.
func (c *Client) HasImage() bool {
	return c != nil && strings.TrimSpace(c.BaseURL) != "" && strings.TrimSpace(c.Model) != ""
}

func (c *Client) endpoint() string {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if strings.HasSuffix(base, "/v1") || strings.Contains(base, "/v1/") {
		return base + "/images/generations"
	}
	return base + "/v1/images/generations"
}

type imageRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	ResponseFormat string `json:"response_format"`
}

type imageResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
	} `json:"data"`
}

// GenerateImage requests n images for prompt and returns the decoded bytes of
// each (PNG) plus the shared MIME type. size/quality are passed through when
// non-empty; n defaults to 1. The signature uses only stdlib types so the
// kernel's runtime.ImageGen seam is satisfied structurally without this package
// importing the kernel.
func (c *Client) GenerateImage(ctx context.Context, prompt, size, quality string, n int) ([][]byte, string, error) {
	if !c.HasImage() {
		return nil, "", errors.New("image: base URL and model required")
	}
	if strings.TrimSpace(prompt) == "" {
		return nil, "", errors.New("image: prompt required")
	}
	if n <= 0 {
		n = 1
	}
	body, err := json.Marshal(imageRequest{
		Model: c.Model, Prompt: prompt, N: n, Size: size, Quality: quality, ResponseFormat: "b64_json",
	})
	if err != nil {
		return nil, "", fmt.Errorf("image: encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("image: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: DefaultTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("image: http: %w", err)
	}
	defer resp.Body.Close()
	respBytes, err := httpread.All(resp.Body, httpread.DefaultMaxResponseBytes)
	if err != nil {
		return nil, "", fmt.Errorf("image: read body: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, "", fmt.Errorf("image: status %d: %s", resp.StatusCode, string(respBytes))
	}
	var ir imageResponse
	if err := json.Unmarshal(respBytes, &ir); err != nil {
		return nil, "", fmt.Errorf("image: decode: %w", err)
	}
	if len(ir.Data) == 0 {
		return nil, "", errors.New("image: response carried no images")
	}
	out := make([][]byte, 0, len(ir.Data))
	for i, d := range ir.Data {
		if strings.TrimSpace(d.B64JSON) == "" {
			return nil, "", fmt.Errorf("image: image %d had no b64_json payload", i)
		}
		raw, err := base64.StdEncoding.DecodeString(d.B64JSON)
		if err != nil {
			return nil, "", fmt.Errorf("image: decode image %d: %w", i, err)
		}
		out = append(out, raw)
	}
	return out, "image/png", nil
}
