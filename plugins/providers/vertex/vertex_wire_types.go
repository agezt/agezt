// SPDX-License-Identifier: MIT

// vertex_wire_types.go holds the JSON wire types mirroring the
// Vertex AI REST shape. The Provider implementation lives in
// vertex_wire.go. Carved out during the Day-110 god-file split.
// Public API unchanged.
package vertex

import (
	"encoding/json"

	"github.com/agezt/agezt/kernel/agent"
)

// ----- dialect translation (canonical ↔ Vertex generateContent) -----
//
// Identical shape to plugins/providers/google. Duplicated rather
// than shared via an internal package; Vertex evolves independently.

type vxRequest struct {
	Contents          []vxContent  `json:"contents"`
	Tools             []vxTool     `json:"tools,omitempty"`
	SystemInstruction *vxContent   `json:"systemInstruction,omitempty"`
	GenerationConfig  *vxGenConfig `json:"generationConfig,omitempty"`
}

type vxGenConfig struct {
	MaxOutputTokens  int               `json:"maxOutputTokens,omitempty"`
	ResponseMimeType string            `json:"responseMimeType,omitempty"` // "application/json" → JSON mode (M312)
	ThinkingConfig   *vxThinkingConfig `json:"thinkingConfig,omitempty"`   // M320
	// Per-request sampling knobs (M997). Gemini-on-Vertex nests these inside
	// generationConfig (NOT top-level), and has no seed / penalties. An unset
	// agent.Params leaves every field nil/empty (omitempty), so the request
	// stays byte-for-byte unchanged.
	Temperature   *float64 `json:"temperature,omitempty"`
	TopP          *float64 `json:"topP,omitempty"`
	TopK          *int     `json:"topK,omitempty"`
	StopSequences []string `json:"stopSequences,omitempty"`
}

// applyParams copies the universal sampling knobs Gemini understands into the
// generationConfig. Reasoning is handled separately (mapped to a thinking
// budget), so it is ignored here. An unset Params leaves the config unchanged.
func (gc *vxGenConfig) applyParams(p agent.Params) {
	if p.IsZero() {
		return
	}
	gc.Temperature = p.Temperature
	gc.TopP = p.TopP
	gc.TopK = p.TopK
	gc.StopSequences = p.Stop
}

// vxThinkingConfig is Gemini-on-Vertex's per-request thinking control (M320,
// 2.5-series). Mirrors plugins/providers/google's geminiThinkingConfig:
// IncludeThoughts asks Vertex to return thought summaries as parts flagged
// `thought:true`; ThinkingBudget caps the thinking tokens (-1 = dynamic).
type vxThinkingConfig struct {
	IncludeThoughts bool `json:"includeThoughts"`
	ThinkingBudget  int  `json:"thinkingBudget"`
}

type vxContent struct {
	Role  string   `json:"role,omitempty"` // "user" | "model"; absent for systemInstruction
	Parts []vxPart `json:"parts"`
}

type vxPart struct {
	Text             string              `json:"text,omitempty"`
	Thought          bool                `json:"thought,omitempty"` // M320: a thought-summary part (reasoning, not answer)
	InlineData       *vxInlineData       `json:"inlineData,omitempty"`
	FunctionCall     *vxFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *vxFunctionResponse `json:"functionResponse,omitempty"`
}

// vxInlineData is an inline base64 blob part — how Gemini-on-Vertex's
// generateContent API carries an image attachment (M245).
type vxInlineData struct {
	MimeType string `json:"mimeType"` // image/png, image/jpeg, image/gif, image/webp
	Data     string `json:"data"`     // base64-encoded image bytes
}

type vxFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type vxFunctionResponse struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

type vxTool struct {
	FunctionDeclarations []vxFunctionDecl `json:"functionDeclarations"`
}

type vxFunctionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type vxResponse struct {
	Candidates    []vxCandidate    `json:"candidates"`
	UsageMetadata *vxUsageMetadata `json:"usageMetadata,omitempty"`
}

type vxCandidate struct {
	Content      vxContent `json:"content"`
	FinishReason string    `json:"finishReason"`
}

type vxUsageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount"` // Gemini context cache (M294-cache)
	// ThoughtsTokenCount is the thinking-token count (M320), reported
	// separately from CandidatesTokenCount but billed at the output rate;
	// folded into Usage.OutputTokens on decode.
	ThoughtsTokenCount int `json:"thoughtsTokenCount"`
}
