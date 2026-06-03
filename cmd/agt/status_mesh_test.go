// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

// TestMeshSummary_NoneOrMalformed: no peers (or a malformed spec) yields nil, so the
// status output stays quiet for single-node operators.
func TestMeshSummary_NoneOrMalformed(t *testing.T) {
	t.Setenv("AGEZT_PEERS", "")
	if m := meshSummary(); m != nil {
		t.Errorf("no peers should yield nil, got %v", m)
	}
	t.Setenv("AGEZT_PEERS", "broken-no-equals")
	if m := meshSummary(); m != nil {
		t.Errorf("malformed spec should yield nil, got %v", m)
	}
}

// TestMeshSummary_SortedNamesURLsNoToken: configured peers are returned sorted by
// name with their URL, and never a token.
func TestMeshSummary_SortedNamesURLsNoToken(t *testing.T) {
	t.Setenv("AGEZT_PEERS", "bravo=http://b:2|secret-b,alpha=http://a:1|secret-a")
	m := meshSummary()
	if len(m) != 2 {
		t.Fatalf("want 2 peers, got %d: %v", len(m), m)
	}
	if m[0]["name"] != "alpha" || m[1]["name"] != "bravo" {
		t.Errorf("peers should be sorted by name, got %v", m)
	}
	if m[0]["url"] != "http://a:1" {
		t.Errorf("url = %v", m[0]["url"])
	}
	// No token must leak into the summary, under any key or value.
	for _, e := range m {
		if _, ok := e["token"]; ok {
			t.Errorf("token key must not be present: %v", e)
		}
		for k, v := range e {
			if s, ok := v.(string); ok && strings.Contains(s, "secret-") {
				t.Errorf("token value leaked in %s: %q", k, s)
			}
		}
	}
}
