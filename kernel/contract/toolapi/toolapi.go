// SPDX-License-Identifier: MIT

// Package toolapi is the tool contract: what a tool is (Tool, ToolDef, Result)
// and the governance metadata it declares (ToolCapability, ToolEffect,
// ObservationTrust). Pure types — no logic beyond accessors on them, and no
// imports beyond the standard library, so any layer may depend on it
// (architecture/20-target-architecture.md §2, layer L1).
//
// These types used to live in kernel/agent, next to the agent loop, so every
// package that only wanted to DESCRIBE a tool (memory, worldmodel, every
// plugin tool) depended on the loop itself. kernel/agent keeps type aliases,
// so existing importers are unaffected.
package toolapi

import (
	"context"
	"encoding/json"
	"strings"
)

// Tool is implemented by anything the agent loop can invoke during a
// task. In-process by default (DECISIONS B0a); out-of-process plugins
// satisfy the same interface via a thin client.
type Tool interface {
	// Definition is the schema/description advertised to the model.
	Definition() ToolDef
	// Invoke executes the tool with the parsed input and returns the
	// textual result for the model. Implementations must honor ctx.
	Invoke(ctx context.Context, input json.RawMessage) (Result, error)
}

// ToolDef advertises a tool to the model.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
	Effect      ToolEffect      `json:"-"`
	// Capability declares which policy axis this tool's calls are gated on.
	// Governance metadata like Effect, so it stays off the provider wire.
	//
	// Declare it. The alternative is a name switch in the policy package, which
	// is a different package from the tool — and a tool missing from that switch
	// resolves to a capability the engine doesn't know, which is DEFAULT-DENIED.
	// That is not a degraded tool, it is a dead one, and it fails silently at run
	// time rather than at build time. Six tools shipped that way before this
	// field existed.
	Capability ToolCapability `json:"-"`
}

// ToolCapability is a tool's declared policy axis.
//
// Most tools exercise one capability for every call and set only Name. A tool
// whose risk depends on what the call ASKS FOR — reading a file versus deleting
// one — also names the input field that selects the axis and maps its values.
type ToolCapability struct {
	// Name is the axis for any call ByValue does not match.
	//
	// For a single-axis tool it is the whole declaration. For a multi-axis one it
	// is the fallback, and choosing it is a real decision the tool's author is
	// best placed to make: a reader should fall back to its READ axis so a
	// garbled call cannot gain write access, while an installer should fall back
	// to its GATED axis so a garbled call cannot slip past the grant. Leaving it
	// empty is not neutral — it defers to the policy package's name switch.
	Name string
	// Field is the top-level input field whose value picks the axis, e.g. "op",
	// "method", or "operation". Empty means single-axis.
	Field string
	// ByValue maps a Field value to its axis. Lookup trims and lower-cases, so
	// entries should be lower-case; a value absent here falls back to Name.
	ByValue map[string]string
}

// IsZero reports whether a tool declared no capability, in which case the
// caller must fall back to its own classification.
func (c ToolCapability) IsZero() bool {
	return c.Name == "" && len(c.ByValue) == 0
}

// For resolves the capability one call exercises. Unparseable input resolves to
// Name — the tool author's declared fallback — rather than failing, because the
// policy layer must reach a decision for every call the model makes, including
// a malformed one.
func (c ToolCapability) For(input json.RawMessage) string {
	if c.Field == "" || len(c.ByValue) == 0 {
		return c.Name
	}
	var probe map[string]any
	if err := json.Unmarshal(input, &probe); err != nil {
		return c.Name
	}
	raw, ok := probe[c.Field].(string)
	if !ok {
		return c.Name
	}
	if cap, ok := c.ByValue[strings.ToLower(strings.TrimSpace(raw))]; ok {
		return cap
	}
	return c.Name
}

// EffectClass classifies a tool call by operational reversibility. It is
// governance metadata, not provider-facing schema; runtime uses it to build HITL
// decision bundles and future compensation routing.
type EffectClass string

const (
	EffectUnknown      EffectClass = ""
	EffectReadOnly     EffectClass = "read_only"
	EffectReversible   EffectClass = "reversible"
	EffectCompensable  EffectClass = "compensable"
	EffectIrreversible EffectClass = "irreversible"
)

// ToolEffect is optional governance metadata supplied by a tool definition.
// Empty fields are filled by runtime capability defaults where possible.
type ToolEffect struct {
	Class             EffectClass
	PredictedEffects  []string
	AffectedResources []string
	RollbackNotes     string
	Confidence        float64
}

// Result is what a Tool returns. IsError signals to the loop that the model
// should see an error (still appended as a tool result message, so the
// model can retry or adjust).
type Result struct {
	Output  string
	IsError bool
	// ObservationTrust classifies tool output before it is fed back to the
	// model. Empty means "use the loop's default for this tool". External
	// world content should be ObservationUntrusted so it is rendered as data,
	// never as an instruction channel.
	ObservationTrust ObservationTrust
	// ObservationSource names the external source in operator-facing audit
	// metadata, e.g. "https://example.com" or "workspace:file.md".
	ObservationSource string
}

// ObservationTrust marks whether a tool result is trusted operational output or
// untrusted external-world data. The model may reason over untrusted data, but
// it must not treat it as an instruction source.
type ObservationTrust string

const (
	ObservationTrustDefault ObservationTrust = ""
	ObservationTrusted      ObservationTrust = "trusted"
	ObservationUntrusted    ObservationTrust = "untrusted"
)
