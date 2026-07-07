// SPDX-License-Identifier: MIT

package sdk

import "testing"

// TestParseApprovals_SkipsNonMapEntries covers the `if !ok { continue }` guard
// in parseApprovals: a rows slice containing a non-map element (a bare string)
// must be tolerated and skipped, not panic, with only the valid entries kept.
func TestParseApprovals_SkipsNonMapEntries(t *testing.T) {
	res := map[string]any{"pending": []any{
		"not-a-map", // skipped
		map[string]any{"id": "ap-ok", "capability": "shell.exec"},
	}}
	aps := parseApprovals(res)
	if len(aps) != 1 || aps[0].ID != "ap-ok" {
		t.Errorf("parseApprovals should skip the non-map entry, got %+v", aps)
	}
}

// TestParseRuns_SkipsNonMapEntries covers parseRuns' analogous non-map guard.
func TestParseRuns_SkipsNonMapEntries(t *testing.T) {
	res := map[string]any{"runs": []any{
		42, // skipped (not a map)
		map[string]any{"correlation_id": "r-ok", "status": "running"},
	}}
	runs := parseRuns(res)
	if len(runs) != 1 || runs[0].CorrelationID != "r-ok" {
		t.Errorf("parseRuns should skip the non-map entry, got %+v", runs)
	}
}

// TestAnyToString_MarshalFallback covers anyToString's final branch: when
// json.Marshal fails (a channel can't be marshalled), it falls back to
// fmt.Sprintf("%v", v) rather than returning empty. We only assert the result
// is non-empty — the exact fmt rendering of a channel is address-dependent.
func TestAnyToString_MarshalFallback(t *testing.T) {
	ch := make(chan int)
	if got := anyToString(ch); got == "" {
		t.Error("anyToString of an unmarshalable value should fall back to a non-empty fmt string")
	}
}
