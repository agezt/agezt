// SPDX-License-Identifier: MIT

package conductor

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestConductorCoverageParseAndRunnerError(t *testing.T) {
	// Malformed JSON: hard error.
	_, err := New().Invoke(context.Background(), json.RawMessage(`{`))
	if err == nil || !strings.Contains(err.Error(), "parse input") {
		t.Fatalf("parse error = %v", err)
	}

	// Runner error: surfaced as a soft error result.
	fr := &fakeRunner{err: errors.New("boom")}
	tool := New()
	tool.SetRunner(fr)
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"task":"x"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "boom") || !strings.HasPrefix(res.Output, "conductor:") {
		t.Fatalf("runner error = %+v", res)
	}
}
