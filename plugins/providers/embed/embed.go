// SPDX-License-Identifier: MIT

// Package embed is the OpenAI-compatible embeddings client (M901) — the
// first real implementation of the kernel's memory.Embedder seam (M884,
// DECISIONS C5 "provider embeddings opt-in"). One wire shape covers the
// whole practical space: api.openai.com, every openai-compatible gateway,
// AND a local Ollama (which serves /v1/embeddings natively) — so "true
// semantic memory" can be zero-cost local (nomic-embed-text) or hosted
// (text-embedding-3-small) with the same three settings.
//
// The kernel never imports this package (kernel-never-imports-plugins); the
// daemon constructs a Client from AGEZT_EMBED_URL / AGEZT_EMBED_MODEL /
// AGEZT_EMBED_KEY and injects it via runtime.Config.MemoryEmbedder. Unset →
// the local feature-hash embedder keeps working exactly as before.
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/agezt/agezt/plugins/providers/internal/httpread"
)

// DefaultTimeout caps one embeddings request. Recall batches are small
// (query + cache misses); a minute is generous even for a cold local model.
const DefaultTimeout = time.Minute

// Client implements memory.Embedder over POST <BaseURL>/embeddings.
type Client struct {
	// BaseURL is the API root, with or without the /v1 suffix —
	// "https://api.openai.com/v1", "http://localhost:11434/v1", or the bare
	// host form of either.
	BaseURL string
	// Model is the embedding model id (e.g. "text-embedding-3-small",
	// "nomic-embed-text"). Required.
	Model string
	// APIKey is sent as a Bearer token when non-empty. A local Ollama needs
	// none.
	APIKey string
	// HTTP overrides the default client (tests). Nil → 1-minute timeout.
	HTTP *http.Client
}

// New constructs a Client with the default HTTP timeout.
func New(baseURL, model, apiKey string) *Client {
	return &Client{
		BaseURL: baseURL,
		Model:   model,
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: DefaultTimeout},
	}
}

func (c *Client) endpoint() string {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if strings.HasSuffix(base, "/v1") || strings.Contains(base, "/v1/") {
		return base + "/embeddings"
	}
	return base + "/v1/embeddings"
}

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// EmbedBatch implements memory.Embedder: one round trip, one L2-normalized
// vector per input text, in input order. The wire's index field is honoured
// (the spec allows out-of-order data), and every vector is re-normalized
// defensively — the kernel's cosine is a bare dot product and assumes unit
// length.
func (c *Client) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if c.Model == "" {
		return nil, errors.New("embed: model required")
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		return nil, errors.New("embed: base URL required")
	}
	if len(texts) == 0 {
		return nil, nil
	}
	body, err := json.Marshal(embedRequest{Model: c.Model, Input: texts})
	if err != nil {
		return nil, fmt.Errorf("embed: encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embed: build request: %w", err)
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
		return nil, fmt.Errorf("embed: http: %w", err)
	}
	defer resp.Body.Close()
	respBytes, err := httpread.All(resp.Body, httpread.DefaultMaxResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("embed: read body: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("embed: status %d: %s", resp.StatusCode, string(respBytes))
	}
	var er embedResponse
	if err := json.Unmarshal(respBytes, &er); err != nil {
		return nil, fmt.Errorf("embed: decode: %w", err)
	}
	if len(er.Data) != len(texts) {
		return nil, fmt.Errorf("embed: got %d embeddings for %d inputs", len(er.Data), len(texts))
	}
	out := make([][]float32, len(texts))
	for _, d := range er.Data {
		if d.Index < 0 || d.Index >= len(out) {
			return nil, fmt.Errorf("embed: embedding index %d out of range", d.Index)
		}
		out[d.Index] = normalize(d.Embedding)
	}
	for i, v := range out {
		if v == nil {
			return nil, fmt.Errorf("embed: missing embedding for input %d", i)
		}
	}
	return out, nil
}

// normalize scales v to unit L2 length (nil/zero vectors pass through —
// they cosine to 0 against everything, which is the right "no signal").
func normalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return v
	}
	scale := float32(1 / math.Sqrt(sum))
	for i := range v {
		v[i] *= scale
	}
	return v
}
