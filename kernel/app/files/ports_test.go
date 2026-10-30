// SPDX-License-Identifier: MIT

package files

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// The runner REFUSES when the handle is not the daemon's own.
//
// A host with no daemon holds no tools to run, and the runner says so rather
// than answering as if a tool had answered.
func TestTheRunnerRefusesWithoutADaemon(t *testing.T) {
	runner := RunnerFromHandle("not a daemon")

	res, err := runner.RunToolWithLookup(context.Background(), "corr", "agent", "file", json.RawMessage(`{}`), nil)
	if err == nil {
		t.Fatal("a host with no daemon ran a tool")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("error = %v, want the runner's refusal", err)
	}
	if res.Output != "" {
		t.Errorf("output = %q, want nothing", res.Output)
	}
}
