// SPDX-License-Identifier: MIT

package ollama

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// TestEncodeRequest_JSONMode (M311): JSONMode sets Ollama's native format="json";
// off omits it; the streaming encoder honours it too.
func TestEncodeRequest_JSONMode(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "return json"}}

	on, err := encodeRequest("llama3", "", msgs, nil, 0, true, agent.Params{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(on), `"format":"json"`) {
		t.Errorf("JSONMode should set format=json: %s", on)
	}

	off, _ := encodeRequest("llama3", "", msgs, nil, 0, false, agent.Params{}, nil)
	if strings.Contains(string(off), `"format"`) {
		t.Errorf("JSONMode=false must omit format: %s", off)
	}

	st, _ := encodeStreamRequest("llama3", "", msgs, nil, 0, true, agent.Params{}, nil)
	if !strings.Contains(string(st), `"format":"json"`) {
		t.Errorf("streaming JSONMode missing format=json: %s", st)
	}
}
