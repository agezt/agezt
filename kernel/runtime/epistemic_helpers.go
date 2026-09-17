// SPDX-License-Identifier: MIT

// epistemic_helpers.go: 12 supporting helpers (escalation policy, schema hashing,
// input-shape heuristics, history matching, decay math) split off from epistemic.go
// during the Day 211 god-file refactor (#130). Public API unchanged.
package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
)


func shouldEscalateEpistemic(class string, confidence float64, failures int, weightedFailures float64, signals []string) bool {
	if class == string(agent.EffectReadOnly) {
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

func confidenceFloor(class string) float64 {
	switch class {
	case string(agent.EffectIrreversible):
		return 0.8
	case string(agent.EffectCompensable):
		return 0.65
	case string(agent.EffectReversible):
		return 0.5
	case string(agent.EffectReadOnly):
		return 0.25
	default:
		return 0.7
	}
}

func hashSchema(schema json.RawMessage) string {
	trimmed := strings.TrimSpace(string(schema))
	if trimmed == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(trimmed))
	return hex.EncodeToString(sum[:])
}

func inputShape(raw json.RawMessage) string {
	var value any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&value); err != nil {
		return "invalid_json"
	}
	return shapeOf(value)
}

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

func containsSignal(signals []string, signal string) bool {
	for _, s := range signals {
		if s == signal || strings.HasPrefix(s, signal+":") {
			return true
		}
	}
	return false
}

type historicalPolicyMeta struct {
	Tool       string
	Capability string
	SchemaHash string
	InputShape string
	TS         time.Time
}

func (k *Kernel) matchHistoricalToolOutcomes(toolName, cap, schemaHash, shape, class string) (seenTool bool, failures int, weightedFailures float64) {
	if k == nil || k.journal == nil {
		return false, 0, 0
	}
	type histEvent struct {
		kind event.Kind
		ts   time.Time
		raw  json.RawMessage
	}
	events := make([]histEvent, 0, 256)
	_ = k.journal.Range(func(e *event.Event) error {
		if e.Kind != event.KindPolicyDecision && e.Kind != event.KindToolResult {
			return nil
		}
		events = append(events, histEvent{
			kind: e.Kind,
			ts:   time.UnixMilli(e.TSUnixMS),
			raw:  append(json.RawMessage(nil), e.Payload...),
		})
		if len(events) > epistemicHistoryLimit {
			copy(events[0:], events[len(events)-epistemicHistoryLimit:])
			events = events[:epistemicHistoryLimit]
		}
		return nil
	})
	byCall := map[string]historicalPolicyMeta{}
	now := time.Now()
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

func failureHalfLife(class string) time.Duration {
	switch class {
	case string(agent.EffectReadOnly):
		return 6 * time.Hour
	case string(agent.EffectReversible):
		return 24 * time.Hour
	case string(agent.EffectCompensable):
		return 72 * time.Hour
	case string(agent.EffectIrreversible):
		return 7 * 24 * time.Hour
	default:
		return 72 * time.Hour
	}
}

func decayWeight(age, halfLife time.Duration) float64 {
	if age <= 0 {
		return 1
	}
	if halfLife <= 0 {
		return 0
	}
	return math.Pow(0.5, float64(age)/float64(halfLife))
}
