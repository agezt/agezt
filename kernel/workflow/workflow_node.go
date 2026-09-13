// SPDX-License-Identifier: MIT

package workflow

// Per-node Config types (ToolConfig..PipelineStepConfig) + the Workflow
// accessors (TriggerNode, NodeByID) + the persistent Store struct.
// Carved out of workflow_validate.go during the Day 147 god-file split so
// the validators file can stay focused on validation logic.
// Public API unchanged.

import (
	"encoding/json"
	"sync"
	"time"
)

type ToolConfig struct {
	Tool string          `json:"tool"`
	Args json.RawMessage `json:"args,omitempty"` // templated JSON
}

type LLMConfig struct {
	Prompt string `json:"prompt"` // templated
	System string `json:"system,omitempty"`
	Model  string `json:"model,omitempty"`
}

type ConditionConfig struct {
	Left  string `json:"left"` // templated
	Op    string `json:"op"`   // equals|not_equals|contains|not_empty|empty|gt|lt
	Right string `json:"right,omitempty"`
}

type TransformConfig struct {
	Template string `json:"template"` // templated → becomes the node's output
}

type DelayConfig struct {
	Seconds float64 `json:"seconds"`
}

// HTTPConfig rides the registered `http` tool — same egress guard and
// CapHTTPGet/Post policy as an agent's own call. URL/headers/body are
// templated.
type HTTPConfig struct {
	Method      string            `json:"method"` // GET | POST
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        string            `json:"body,omitempty"`
	ContentType string            `json:"content_type,omitempty"`
}

// CodeConfig runs a script in the code-exec sandbox (the M794 runner):
// the interpolated Input lands as ./stdin.txt, stdout becomes the output.
type CodeConfig struct {
	Language string `json:"language"`
	Code     string `json:"code"`
	Input    string `json:"input,omitempty"` // templated; default "{}"
}

// MapConfig applies Template to every element of the array at Items
// (a {{...}}-style path); inside the template {{item}} / {{item.field}} /
// {{index}} address the current element.
type MapConfig struct {
	Items    string `json:"items"`
	Template string `json:"template"`
}

// FilterConfig keeps the elements of Items for which left/op/right holds
// ({{item}} / {{index}} usable in left and right).
type FilterConfig struct {
	Items string `json:"items"`
	Left  string `json:"left"`
	Op    string `json:"op"`
	Right string `json:"right,omitempty"`
}

// SwitchConfig routes by value: the first case whose Equals matches fires
// its Port; otherwise the "default" port fires.
type SwitchCase struct {
	Equals string `json:"equals"`
	Port   string `json:"port"`
}

type SwitchConfig struct {
	Value string       `json:"value"` // templated
	Cases []SwitchCase `json:"cases"`
}

// MergeConfig joins branches: "any" (default) runs on the first incoming
// token; "all" waits for a token on EVERY incoming edge — if an upstream
// branch never fires (an untaken condition path), the merge never runs.
type MergeConfig struct {
	Mode string `json:"mode,omitempty"`
}

// ApprovalConfig blocks the run on a human decision via the HITL approval
// registry (`agt approvals` / channels / console). Deny or timeout fails
// the node (wire an "error" port to branch instead).
type ApprovalConfig struct {
	Description string `json:"description"` // templated; what the operator reads
	Capability  string `json:"capability,omitempty"`
}

// SubflowConfig runs another stored workflow; the interpolated Payload
// becomes its {{trigger.payload}}. Nesting is depth-capped by the engine.
type SubflowConfig struct {
	Workflow string `json:"workflow"`
	Payload  string `json:"payload,omitempty"` // templated; JSON becomes structured
}

// PipelineConfig runs several governed tool calls inside one deterministic node,
// without an LLM round-trip between steps. Step args are templated against the
// workflow data plus prior step outputs under {{steps.<id>.output}}.
type PipelineConfig struct {
	Steps []PipelineStepConfig `json:"steps"`
}

type PipelineStepConfig struct {
	ID           string          `json:"id"`
	Tool         string          `json:"tool"`
	Args         json.RawMessage `json:"args,omitempty"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
}

// TriggerNode returns the workflow's single trigger (Validate guarantees it).
func (w Workflow) TriggerNode() *Node {
	for i := range w.Nodes {
		if w.Nodes[i].Type == NodeTrigger {
			return &w.Nodes[i]
		}
	}
	return nil
}

// NodeByID resolves one node.
func (w Workflow) NodeByID(id string) *Node {
	for i := range w.Nodes {
		if w.Nodes[i].ID == id {
			return &w.Nodes[i]
		}
	}
	return nil
}

// Store is the persistent workflow registry, a single JSON file rewritten
// atomically on change. Safe for concurrent use. Mirrors kernel/standing.
type Store struct {
	path  string
	mu    sync.Mutex
	now   func() time.Time
	items []*Workflow
}

