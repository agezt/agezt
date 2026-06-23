// SPDX-License-Identifier: MIT

package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// TestEncodeRequest_JSONMode (M311): JSONMode sets OpenAI's
// response_format:{type:json_object}; off omits it; streaming honours it too.
func TestEncodeRequest_JSONMode(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "return json"}}

	on, err := encodeRequest("gpt-4o", "", msgs, nil, 0, true, agent.Params{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		ResponseFormat *struct {
			Type string `json:"type"`
		} `json:"response_format"`
	}
	if err := json.Unmarshal(on, &r); err != nil {
		t.Fatal(err)
	}
	if r.ResponseFormat == nil || r.ResponseFormat.Type != "json_object" {
		t.Errorf("JSONMode should set response_format type=json_object: %s", on)
	}

	off, _ := encodeRequest("gpt-4o", "", msgs, nil, 0, false, agent.Params{}, nil)
	if strings.Contains(string(off), "response_format") {
		t.Errorf("JSONMode=false must omit response_format: %s", off)
	}

	st, _ := encodeStreamRequest("gpt-4o", "", msgs, nil, 0, true, agent.Params{}, nil)
	if !strings.Contains(string(st), `"response_format"`) || !strings.Contains(string(st), "json_object") {
		t.Errorf("streaming JSONMode missing response_format: %s", st)
	}
}
