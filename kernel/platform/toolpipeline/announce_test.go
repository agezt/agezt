// SPDX-License-Identifier: MIT

package toolpipeline_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/toolpipeline"
)

func TestAnnounceRetainsAuditContract(t *testing.T) {
	for _, input := range []json.RawMessage{nil, json.RawMessage(" {\n \"n\": [1, 2] } ")} {
		for _, fail := range []bool{false, true} {
			call := llm.ToolCall{ID: "call", Name: "alias", Input: input}
			cause := errors.New("typed audit cause")
			count := 0
			err := toolpipeline.Announce(call, func(kind event.Kind, payload map[string]any) error {
				count++
				if kind != event.KindToolInvoked || !reflect.DeepEqual(payload, map[string]any{"tool": "alias", "call_id": "call", "input": input}) {
					t.Errorf("kind=%s payload=%v", kind, payload)
				}
				if fail {
					return cause
				}
				return nil
			})
			if count != 1 || (fail && (err != cause || !errors.Is(err, cause))) || (!fail && err != nil) {
				t.Fatalf("audit count=%d fail=%t error=%v", count, fail, err)
			}
		}
	}
}
