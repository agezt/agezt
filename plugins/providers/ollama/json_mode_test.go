// SPDX-License-Identifier: MIT

package ollama

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
)

// TestEncodeRequest_JSONMode (M311): JSONMode sets Ollama's native format="json";
// off omits it; the streaming encoder honours it too.
func TestEncodeRequest_JSONMode(t *testing.T) {
	msgs := []llm.Message{{Role: llm.RoleUser, Content: "return json"}}

	on, err := encodeRequest("llama3", "", msgs, nil, 0, true, llm.Params{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(on), `"format":"json"`) {
		t.Errorf("JSONMode should set format=json: %s", on)
	}

	off, _ := encodeRequest("llama3", "", msgs, nil, 0, false, llm.Params{}, nil)
	if strings.Contains(string(off), `"format"`) {
		t.Errorf("JSONMode=false must omit format: %s", off)
	}

	st, _ := encodeStreamRequest("llama3", "", msgs, nil, 0, true, llm.Params{}, nil)
	if !strings.Contains(string(st), `"format":"json"`) {
		t.Errorf("streaming JSONMode missing format=json: %s", st)
	}
}
