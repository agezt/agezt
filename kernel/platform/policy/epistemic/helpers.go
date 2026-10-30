// SPDX-License-Identifier: MIT

package epistemic

// Provenance: the epistemic guard's helpers (the escalation policy, the
//             schema hashing, the input-shape heuristics, the history
//             matching and the decay math), moved out of
//             kernel/runtime with the guard itself (W3.73).
//
// The rules are unchanged by the move: a read-only tool escalates only on
// repeated weighted failures, an effectful tool escalates on a matching
// failure, on a confidence below its class's floor, or on a permissive schema
// behind a dynamic tool surface — and the failures' weight decays with their
// age.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
)

// shouldEscalate is the escalation policy: the guard's own rule for when a
// call needs a human.
func shouldEscalate(class string, confidence float64, failures int, weightedFailures float64, signals []string) bool {
	if class == string(toolapi.EffectReadOnly) {
		return weightedFailures >= 2
	}
	if failures > 0 && weightedFailures >= 0.75 {
		return true
	}
	if confidence < confidenceFloor(class) {
		return true
	}
	if containsSignal(signals, "permissive_schema_effectful_tool") && containsSignal(signals, "dynamic_tool_surface") {
		return true
	}
	return false
}

// confidenceFloor is the confidence below which a class is not trusted to run
// unattended.
func confidenceFloor(class string) float64 {
	switch class {
	case string(toolapi.EffectIrreversible):
		return 0.8
	case string(toolapi.EffectCompensable):
		return 0.65
	case string(toolapi.EffectReversible):
		return 0.5
	case string(toolapi.EffectReadOnly):
		return 0.25
	default:
		return 0.7
	}
}

// hashSchema fingerprints a tool's input schema, so the guard can tell
// "the same tool under the same conditions" from "the same tool under a
// different one".
func hashSchema(schema json.RawMessage) string {
	trimmed := strings.TrimSpace(string(schema))
	if trimmed == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(trimmed))
	return hex.EncodeToString(sum[:])
}

// inputShape is the call input's SHAPE — its structure, not its values. Two
// calls with the same shape are the same kind of call.
func inputShape(raw json.RawMessage) string {
	var value any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&value); err != nil {
		return "invalid_json"
	}
	return shapeOf(value)
}

// shapeOf is the recursive shape of a decoded value.
func shapeOf(v any) string {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for key := range x {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, key+":"+shapeOf(x[key]))
		}
		return "object{" + strings.Join(parts, ",") + "}"
	case []any:
		if len(x) == 0 {
			return "array[]"
		}
		return "array[" + shapeOf(x[0]) + "]"
	case string:
		return "string"
	case bool:
		return "bool"
	case json.Number, float64, int, int64:
		return "number"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", v)
	}
}

// schemaPermissive says whether a tool's schema accepts anything — a
// permissive schema behind an effectful tool is a signal on its own.
func schemaPermissive(schema json.RawMessage) bool {
	s := strings.TrimSpace(string(schema))
	if s == "" {
		return true
	}
	var root struct {
		AdditionalProperties any            `json:"additionalProperties"`
		Properties           map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(schema, &root); err != nil {
		return true
	}
	if v, ok := root.AdditionalProperties.(bool); ok && v {
		return true
	}
	return len(root.Properties) == 0 && root.AdditionalProperties == nil
}

// temporalSensitive says whether the call reads the world's time — a shape
// whose meaning decays, so yesterday's success says little about tomorrow.
func temporalSensitive(toolName, desc string, input json.RawMessage) bool {
	hay := strings.ToLower(toolName + " " + desc + " " + string(input))
	for _, word := range []string{
		"today", "current", "currently", "latest", "recent", "now", "yesterday", "tomorrow",
		"price", "schedule", "version", "release", "weather", "news", "deadline",
	} {
		if strings.Contains(hay, word) {
			return true
		}
	}
	return false
}

// containsSignal says whether the signals carry one (a signal may carry a
// count after a colon).
func containsSignal(signals []string, signal string) bool {
	for _, s := range signals {
		if s == signal || strings.HasPrefix(s, signal+":") {
			return true
		}
	}
	return false
}

// historicalPolicyMeta is one journaled policy decision: which tool, under
// which conditions, and when.
type historicalPolicyMeta struct {
	Tool       string
	Capability string
	SchemaHash string
	InputShape string
	TS         time.Time
}

// matchHistoricalToolOutcomes reads the journal's memory: has this tool been
// seen under these conditions, and how many of those calls failed.
//
// The failures are weighted by their age (a failure from yesterday counts
// less than one from an hour ago).
func (g *Gate) matchHistoricalToolOutcomes(toolName, cap, schemaHash, shape, class string) (seenTool bool, failures int, weightedFailures float64) {
	if g == nil || g.ports.Scan == nil {
		return false, 0, 0
	}
	type histEvent struct {
		kind event.Kind
		ts   time.Time
		raw  json.RawMessage
	}
	events := make([]histEvent, 0, 256)
	_ = g.ports.Scan(func(e *event.Event) error {
		if e.Kind != event.KindPolicyDecision && e.Kind != event.KindToolResult {
			return nil
		}
		events = append(events, histEvent{
			kind: e.Kind,
			ts:   time.UnixMilli(e.TSUnixMS),
			raw:  append(json.RawMessage(nil), e.Payload...),
		})
		if len(events) > historyLimit {
			copy(events[0:], events[len(events)-historyLimit:])
			events = events[:historyLimit]
		}
		return nil
	})
	byCall := map[string]historicalPolicyMeta{}
	now := time.Now()
	if g.ports.Now != nil {
		now = g.ports.Now()
	}
	for _, e := range events {
		switch e.kind {
		case event.KindPolicyDecision:
			var p struct {
				Tool       string `json:"tool"`
				CallID     string `json:"call_id"`
				Capability string `json:"capability"`
				SchemaHash string `json:"schema_hash"`
				InputShape string `json:"input_shape"`
			}
			if json.Unmarshal(e.raw, &p) != nil || p.CallID == "" {
				continue
			}
			if p.Tool == toolName {
				seenTool = true
			}
			byCall[p.Tool+"\x00"+p.CallID] = historicalPolicyMeta{
				Tool:       p.Tool,
				Capability: p.Capability,
				SchemaHash: p.SchemaHash,
				InputShape: p.InputShape,
				TS:         e.ts,
			}
		case event.KindToolResult:
			var p struct {
				Tool   string `json:"tool"`
				CallID string `json:"call_id"`
				Error  bool   `json:"error"`
			}
			if json.Unmarshal(e.raw, &p) != nil || !p.Error || p.CallID == "" {
				continue
			}
			meta, ok := byCall[p.Tool+"\x00"+p.CallID]
			if !ok || !conditionMatches(meta, toolName, cap, schemaHash, shape) {
				continue
			}
			failures++
			weightedFailures += decayWeight(now.Sub(meta.TS), failureHalfLife(class))
		}
	}
	return seenTool, failures, weightedFailures
}

// conditionMatches says whether a journaled decision was made under the
// call's conditions: the same tool, and — when the journal recorded them —
// the same capability, schema and input shape.
func conditionMatches(meta historicalPolicyMeta, toolName, cap, schemaHash, shape string) bool {
	if meta.Tool != toolName {
		return false
	}
	if meta.Capability != "" && cap != "" && meta.Capability != cap {
		return false
	}
	if meta.SchemaHash != "" && schemaHash != "" && meta.SchemaHash != schemaHash {
		return false
	}
	if meta.InputShape != "" && shape != "" && meta.InputShape != shape {
		return false
	}
	return true
}

// failureHalfLife is how long a failure keeps half its weight, per effect
// class: the more dangerous the class, the longer the memory.
func failureHalfLife(class string) time.Duration {
	switch class {
	case string(toolapi.EffectReadOnly):
		return 6 * time.Hour
	case string(toolapi.EffectReversible):
		return 24 * time.Hour
	case string(toolapi.EffectCompensable):
		return 72 * time.Hour
	case string(toolapi.EffectIrreversible):
		return 7 * 24 * time.Hour
	default:
		return 72 * time.Hour
	}
}

// decayWeight is a failure's weight at a given age.
func decayWeight(age, halfLife time.Duration) float64 {
	if age <= 0 {
		return 1
	}
	if halfLife <= 0 {
		return 0
	}
	return math.Pow(0.5, float64(age)/float64(halfLife))
}
