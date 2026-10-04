// SPDX-License-Identifier: MIT

package toolpipeline_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/toolpipeline"
)

func TestSettleRetainsTerminalContract(t *testing.T) {
	for _, result := range []toolapi.Result{{}, {Output: "failure", IsError: true}} {
		for _, fail := range []bool{false, true} {
			fields := map[string]any{"tool": "spoof", "call_id": "spoof", "output": "spoof", "error": "spoof", "directive_matches": []string{}, "raw_ref": "blob", "output_bytes": 22, "memo_hit": true, "not_executed": true, "observation_trust": toolapi.ObservationUntrusted}
			original := make(map[string]any, len(fields))
			for key, value := range fields {
				original[key] = value
			}
			cause := errors.New("typed terminal audit cause")
			count := 0
			err := toolpipeline.Settle(llm.ToolCall{ID: "call", Name: "alias"}, result, fields, func(kind event.Kind, payload map[string]any) error {
				count++
				if kind != event.KindToolResult || len(payload) != len(fields) || payload["tool"] != "alias" || payload["call_id"] != "call" || payload["output"] != result.Output || payload["error"] != result.IsError {
					t.Errorf("terminal kind=%s payload=%v", kind, payload)
				}
				for _, key := range []string{"directive_matches", "raw_ref", "output_bytes", "memo_hit", "not_executed", "observation_trust"} {
					if !reflect.DeepEqual(payload[key], fields[key]) {
						t.Errorf("metadata %s lost", key)
					}
				}
				payload["raw_ref"] = "publisher-local"
				if fail {
					return cause
				}
				return nil
			})
			if count != 1 || (fail && err != cause) || (!fail && err != nil) || !reflect.DeepEqual(fields, original) {
				t.Fatalf("count=%d error=%v fields=%v", count, err, fields)
			}
		}
	}
}

func TestSettleInlineWithoutMetadata(t *testing.T) {
	err := toolpipeline.Settle(llm.ToolCall{ID: "call", Name: "tool"}, toolapi.Result{Output: "inline"}, nil, func(kind event.Kind, payload map[string]any) error {
		if kind != event.KindToolResult || !reflect.DeepEqual(payload, map[string]any{"tool": "tool", "call_id": "call", "output": "inline", "error": false}) {
			t.Errorf("inline kind=%s payload=%v", kind, payload)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
