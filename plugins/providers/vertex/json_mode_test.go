// SPDX-License-Identifier: MIT

package vertex

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// TestEncodeRequest_JSONMode (M312): JSONMode sets Gemini-on-Vertex's
// generationConfig.responseMimeType=application/json; off omits it; it composes
// with maxOutputTokens.
func TestEncodeRequest_JSONMode(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "return json"}}

	on, err := encodeRequest("", msgs, nil, 0, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(on), `"responseMimeType":"application/json"`) {
		t.Errorf("JSONMode should set responseMimeType: %s", on)
	}

	off, _ := encodeRequest("", msgs, nil, 0, false, 0)
	if strings.Contains(string(off), "responseMimeType") {
		t.Errorf("JSONMode=false must omit responseMimeType: %s", off)
	}

	both, _ := encodeRequest("", msgs, nil, 500, true, 0)
	if !strings.Contains(string(both), "responseMimeType") || !strings.Contains(string(both), "maxOutputTokens") {
		t.Errorf("JSONMode should compose with maxOutputTokens: %s", both)
	}
}
