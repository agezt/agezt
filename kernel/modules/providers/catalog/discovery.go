// SPDX-License-Identifier: MIT

package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultOllamaEndpoint is the default base for Ollama local
// discovery. Override via AGEZT_OLLAMA_ENDPOINT.
const DefaultOllamaEndpoint = "http://localhost:11434"

// OllamaProviderID is the catalog ID used for the synthesised local
// Ollama provider entry. Stable across runs so custom.json overrides
// stick.
const OllamaProviderID = "ollama-local"

// DiscoverOllama probes <endpoint>/api/tags and returns a Catalog
// fragment with one provider entry (OllamaProviderID) containing
// every locally-installed model. Failure is non-fatal — the daemon
// should log it and continue; absence of Ollama is the common case.
func DiscoverOllama(ctx context.Context, endpoint string) (*Catalog, error) {
	if endpoint == "" {
		endpoint = DefaultOllamaEndpoint
	}
	url := endpoint + "/api/tags"

	reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// Guarded client: Ollama is local (loopback/private allowed) but a
	// redirect to the cloud-metadata endpoint is still refused (SSRF).
	resp, err := guardedClient(3 * time.Second).Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama discovery: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var tagsResp struct {
		Models []struct {
			Name    string `json:"name"`
			Model   string `json:"model"`
			Size    int64  `json:"size"`
			Details struct {
				Family            string   `json:"family"`
				Families          []string `json:"families"`
				ParameterSize     string   `json:"parameter_size"`
				QuantizationLevel string   `json:"quantization_level"`
			} `json:"details"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &tagsResp); err != nil {
		return nil, fmt.Errorf("ollama discovery: parse: %w", err)
	}

	provider := &Provider{
		ID:     OllamaProviderID,
		Name:   "Ollama (local)",
		NPM:    "@ai-sdk/ollama",
		API:    endpoint,
		Doc:    "https://ollama.com/docs",
		Models: map[string]*Model{},
	}
	for _, m := range tagsResp.Models {
		id := m.Model
		if id == "" {
			id = m.Name
		}
		if id == "" {
			continue
		}
		fam := m.Details.Family
		if fam == "" && len(m.Details.Families) > 0 {
			fam = m.Details.Families[0]
		}
		// Vision detection (M309): a local multimodal model (llava,
		// llama3.2-vision, moondream, …) accepts image input. Ollama signals it
		// via a vision projector in `details.families` ("clip"/"mllama") or a
		// recognisable model id. Mark such models image-capable so the M91 vision
		// gate lets attachments through to the (now image-forwarding) provider.
		input := []string{"text"}
		attachment := false
		if ollamaModelHasVision(id, m.Details.Families) {
			input = []string{"text", "image"}
			attachment = true
		}
		provider.Models[id] = &Model{
			ID:         id,
			Name:       m.Name,
			Family:     fam,
			ToolCall:   true, // Ollama exposes tool-use uniformly
			OpenWeight: true,
			Attachment: attachment,
			Modalities: Modalities{
				Input:  input,
				Output: []string{"text"},
			},
			// Cost: nil → free/local; matches our existing $0 semantics.
		}
	}

	cat := NewEmpty()
	cat.Providers[provider.ID] = provider
	return cat, nil
}

// ollamaModelHasVision reports whether a discovered Ollama model accepts image
// input. Two signals: a vision projector family ("clip" — llava/bakllava;
// "mllama" — llama3.2-vision) reported in `details.families`, or a recognisable
// vision model id. Heuristic by necessity — Ollama's /api/tags has no explicit
// modality field — but it covers the common local vision models; an operator can
// always pin capabilities in custom.json for anything missed.
func ollamaModelHasVision(id string, families []string) bool {
	for _, f := range families {
		switch strings.ToLower(strings.TrimSpace(f)) {
		case "clip", "mllama":
			return true
		}
	}
	lid := strings.ToLower(id)
	for _, marker := range []string{"llava", "vision", "moondream", "bakllava", "minicpm-v", "llama3.2-vision"} {
		if strings.Contains(lid, marker) {
			return true
		}
	}
	return false
}
