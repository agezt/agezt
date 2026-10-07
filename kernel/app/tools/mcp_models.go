// SPDX-License-Identifier: MIT
package tools

// MCPServerView deliberately has no environment/header value fields.
// ToolCount distinguishes an attached zero-tool server from a detached server.
type MCPServerView struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Command     string   `json:"command,omitempty"`
	Args        []string `json:"args,omitempty"`
	URL         string   `json:"url,omitempty"`
	Enabled     bool     `json:"enabled"`
	Description string   `json:"description,omitempty"`
	Lazy        bool     `json:"lazy,omitempty"`
	ToolAllow   []string `json:"tool_allow,omitempty"`
	CreatedMS   int64    `json:"created_ms"`
	UpdatedMS   int64    `json:"updated_ms"`
	Transport   string   `json:"transport"`
	Attached    bool     `json:"attached"`
	EnvKeys     []string `json:"env_keys,omitempty"`
	HeaderKeys  []string `json:"header_keys,omitempty"`
	ToolCount   *int     `json:"tool_count,omitempty"`
}
