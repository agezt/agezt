// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

func TestFinalizeSettlementRetainsRepresentation(t *testing.T) {
	for _, mode := range []string{"success", "reported-error", "memo", "skipped", "untrusted-offload"} {
		t.Run(mode, func(t *testing.T) {
			result := Result{Output: "finished", ObservationTrust: ObservationTrusted, ObservationSource: "fixture"}
			if mode == "reported-error" || mode == "skipped" {
				result.IsError = true
			}
			if mode == "untrusted-offload" {
				result.Output = strings.Repeat("external data ", 2000)
				result.ObservationTrust = ObservationUntrusted
			}
			store := &recordingPutter{}
			hooks := 0
			var record map[string]any
			s := newRunState(LoopConfig{Artifacts: store, ArtifactThreshold: 64, ToolResultHook: func(_ context.Context, call ToolCall, got Result) {
				hooks++
				if call.ID != "call" || call.Name != "alias" || !reflect.DeepEqual(got, result) {
					t.Errorf("hook call=%+v result=%+v", call, got)
				}
			}}, func(kind event.Kind, suffix string, payload any) (*event.Event, error) {
				if kind != event.KindToolResult || suffix != "tool" || record != nil {
					t.Errorf("terminal publication: kind=%s suffix=%s", kind, suffix)
				}
				record = payload.(map[string]any)
				return nil, nil
			})
			job := &toolJob{tc: ToolCall{ID: "call", Name: "alias"}, tool: &stubTool{}, result: result, memoHit: mode == "memo", skipped: mode == "skipped"}
			if mode == "memo" {
				job.tool = nil
			}
			messages, err := s.finalizeToolJobs(context.Background(), []*toolJob{job}, 3, nil)
			if err != nil || len(messages) != 1 || messages[0].ToolCallID != "call" || !strings.Contains(messages[0].Content, result.Output) {
				t.Fatalf("error=%v messages=%v", err, messages)
			}
			wantHooks := 1
			if mode == "memo" || mode == "skipped" {
				wantHooks = 0
			}
			if hooks != wantHooks {
				t.Errorf("hooks=%d want=%d", hooks, wantHooks)
			}
			base := map[string]any{"tool": "alias", "call_id": "call", "error": result.IsError, "observation_trust": result.ObservationTrust, "observation_source": "fixture", "directive_like": false, "directive_matches": []string(nil)}
			if mode == "untrusted-offload" {
				base["directive_matches"] = []string{}
			}
			for key, want := range base {
				if !reflect.DeepEqual(record[key], want) {
					t.Errorf("field %s=%v want=%v", key, record[key], want)
				}
			}
			wantFields := 8
			if mode == "memo" {
				wantFields++
				if record["memo_hit"] != true {
					t.Error("memo marker missing")
				}
			}
			if mode == "skipped" {
				wantFields++
				if record["not_executed"] != true {
					t.Error("skipped marker missing")
				}
			}
			if mode == "untrusted-offload" {
				wantFields += 2
				if record["raw_ref"] != "ref-"+itoa(len(result.Output)) || record["output_bytes"] != len(result.Output) || len(store.puts) != 1 || string(store.puts[0]) != result.Output || !strings.Contains(record["output"].(string), "offloaded") {
					t.Errorf("artifact record=%v", record)
				}
				if !reflect.DeepEqual(s.untrustedTaint.Sources, []string{"fixture"}) {
					t.Errorf("provenance=%+v", s.untrustedTaint)
				}
			} else if record["output"] != result.Output {
				t.Errorf("output=%v", record["output"])
			}
			if len(record) != wantFields {
				t.Errorf("fields=%d want=%d", len(record), wantFields)
			}
		})
	}
}
