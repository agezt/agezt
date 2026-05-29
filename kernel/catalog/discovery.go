// SPDX-License-Identifier: MIT

package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	resp, err := http.DefaultClient.Do(req)
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
		provider.Models[id] = &Model{
			ID:         id,
			Name:       m.Name,
			Family:     fam,
			ToolCall:   true, // Ollama exposes tool-use uniformly
			OpenWeight: true,
			Modalities: Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			// Cost: nil → free/local; matches our existing $0 semantics.
		}
	}

	cat := NewEmpty()
	cat.Providers[provider.ID] = provider
	return cat, nil
}
