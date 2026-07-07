// SPDX-License-Identifier: MIT

package workflowtool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestInvoke_ParseError(t *testing.T) {
	tool := New()
	_, err := tool.Invoke(context.Background(), json.RawMessage(`not json`))
	if err == nil {
		t.Fatal("parse error should return a hard error")
	}
	if !strings.Contains(err.Error(), "workflow:") {
		t.Errorf("parse error should carry prefix, got %v", err)
	}
}

func TestOkJSON_MarshalError(t *testing.T) {
	res := okJSON(make(chan int))
	if !res.IsError {
		t.Error("okJSON(channel) should return error")
	}
}

func TestSaveUpdatePath(t *testing.T) {
	fk := newFakeKernel(t)
	tool := New()
	tool.Bind(fk)

	// First save: create.
	out1 := invoke(t, tool, `{"op":"save","workflow":`+graphJSON+`}`)
	if !strings.Contains(out1, "created") {
		t.Fatalf("first save should create, got:\n%s", out1)
	}

	// Second save with same name: update.
	out2 := invoke(t, tool, `{"op":"save","workflow":`+graphJSON+`}`)
	if !strings.Contains(out2, "updated") {
		t.Fatalf("second save should update, got:\n%s", out2)
	}
}

var _ = agent.Tool(New()) // compile-time check
