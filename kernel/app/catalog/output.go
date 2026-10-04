// SPDX-License-Identifier: MIT

package catalog

type SyncOutput struct {
	URL                 string `json:"url"`
	Bytes               int    `json:"bytes"`
	ProviderCount       int    `json:"provider_count"`
	ModelCount          int    `json:"model_count"`
	DurationMS          int64  `json:"duration_ms"`
	ProvidersReloaded   bool   `json:"providers_reloaded"`
	ProviderReloadError string `json:"provider_reload_error,omitempty"`
}

type DiscoverOutput struct {
	Endpoint            string `json:"endpoint"`
	ModelCount          int    `json:"model_count"`
	ProvidersReloaded   bool   `json:"providers_reloaded"`
	ProviderReloadError string `json:"provider_reload_error,omitempty"`
}

type ListOutput struct {
	Providers     []ProviderOutput `json:"providers"`
	Sources       []string         `json:"sources"`
	APISyncedAt   string           `json:"api_synced_at"`
	APISourceURL  string           `json:"api_source_url"`
	LocalSyncedAt string           `json:"local_synced_at"`
	LocalSource   string           `json:"local_source"`
	ProviderCount int              `json:"provider_count"`
}

type ProviderOutput struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Family       string        `json:"family"`
	API          string        `json:"api"`
	Doc          string        `json:"doc"`
	Env          []string      `json:"env"`
	Credentialed bool          `json:"credentialed"`
	ModelCount   int           `json:"model_count"`
	Models       []ModelOutput `json:"models"`
}

type ModelOutput struct {
	ID                         string   `json:"id"`
	Name                       string   `json:"name"`
	Family                     string   `json:"family"`
	ToolCall                   bool     `json:"tool_call"`
	StrictToolArgs             bool     `json:"strict_tool_args"`
	SchemaConstrainedDecoding  bool     `json:"schema_constrained_decoding"`
	GrammarConstrainedDecoding bool     `json:"grammar_constrained_decoding"`
	Reasoning                  bool     `json:"reasoning"`
	Context                    int      `json:"context"`
	Output                     int      `json:"output"`
	CostInputUSDPerMTok        *float64 `json:"cost_input_usd_per_mtok,omitempty"`
	CostOutputUSDPerMTok       *float64 `json:"cost_output_usd_per_mtok,omitempty"`
	CostInputMCPerMTok         *int64   `json:"cost_input_mc_per_mtok,omitempty"`
	CostOutputMCPerMTok        *int64   `json:"cost_output_mc_per_mtok,omitempty"`
}
