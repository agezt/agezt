// SPDX-License-Identifier: MIT

// Package rerank is the Cohere/Jina-style document reranking client (M997) — the
// retrieval-quality sibling of plugins/providers/embed. One wire shape (POST
// <BaseURL>/rerank with {model, query, documents, top_n}) covers Cohere's
// /v2/rerank, Jina, Voyage and the OpenAI-compatible rerank gateways. The kernel
// never imports this package; the daemon builds a Client from AGEZT_RERANK_URL /
// AGEZT_RERANK_MODEL / AGEZT_RERANK_KEY and injects it via
// runtime.Config.Reranker. Unset → no rerank tool.
package rerank

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/agezt/agezt/plugins/providers/internal/httpread"
)

// DefaultTimeout caps one rerank request.
const DefaultTimeout = time.Minute

// Client reranks documents over POST <BaseURL>/rerank.
type Client struct {
	// BaseURL is the API root, with or without the /v1 or /v2 suffix.
	BaseURL string
	// Model is the rerank model id (e.g. "rerank-english-v3.0"). Required.
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

// HasRerank reports whether the client is configured enough to rerank.
func (c *Client) HasRerank() bool {
	return c != nil && strings.TrimSpace(c.BaseURL) != "" && strings.TrimSpace(c.Model) != ""
}

func (c *Client) endpoint() string {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if strings.HasSuffix(base, "/rerank") {
		return base
	}
	if strings.HasSuffix(base, "/v1") || strings.HasSuffix(base, "/v2") ||
		strings.Contains(base, "/v1/") || strings.Contains(base, "/v2/") {
		return base + "/rerank"
	}
	return base + "/v1/rerank"
}

type rerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n,omitempty"`
}

type rerankResponse struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
}

// Rerank scores documents against query and returns, in descending relevance
// order, the original index of each document and its relevance score (parallel
// slices). topN > 0 caps how many are returned. The signature uses only stdlib
// types so the kernel's runtime.Reranker seam is satisfied structurally without
// this package importing the kernel.
func (c *Client) Rerank(ctx context.Context, query string, documents []string, topN int) ([]int, []float64, error) {
	if !c.HasRerank() {
		return nil, nil, errors.New("rerank: base URL and model required")
	}
	if strings.TrimSpace(query) == "" {
		return nil, nil, errors.New("rerank: query required")
	}
	if len(documents) == 0 {
		return nil, nil, nil
	}
	body, err := json.Marshal(rerankRequest{Model: c.Model, Query: query, Documents: documents, TopN: topN})
	if err != nil {
		return nil, nil, fmt.Errorf("rerank: encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("rerank: build request: %w", err)
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
		return nil, nil, fmt.Errorf("rerank: http: %w", err)
	}
	defer resp.Body.Close()
	respBytes, err := httpread.All(resp.Body, httpread.DefaultMaxResponseBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("rerank: read body: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, nil, fmt.Errorf("rerank: status %d: %s", resp.StatusCode, string(respBytes))
	}
	var rr rerankResponse
	if err := json.Unmarshal(respBytes, &rr); err != nil {
		return nil, nil, fmt.Errorf("rerank: decode: %w", err)
	}
	idx := make([]int, 0, len(rr.Results))
	scores := make([]float64, 0, len(rr.Results))
	for _, r := range rr.Results {
		if r.Index < 0 || r.Index >= len(documents) {
			return nil, nil, fmt.Errorf("rerank: result index %d out of range", r.Index)
		}
		idx = append(idx, r.Index)
		scores = append(scores, r.RelevanceScore)
	}
	return idx, scores, nil
}
