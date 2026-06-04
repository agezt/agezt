// SPDX-License-Identifier: MIT

package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
)

// TestRemoteTool_InvokeOnDeadPluginFailsCleanly locks in the SPEC-04 §0.2 /
// SPEC-04 §1.6 crash-isolation contract: when a plugin process has died, an
// invocation of one of its tools must return a clear "unavailable" error that
// surfaces the death cause (diagnosable, never silent) — and must NOT panic,
// hang, or try to write to the dead process's pipe. This is what lets one
// plugin crash without taking down the run or the daemon (the agent loop turns
// the error into a tool-result the model can react to).
func TestRemoteTool_InvokeOnDeadPluginFailsCleanly(t *testing.T) {
	p := &Plugin{
		pending:  make(map[string]chan *Response),
		progress: make(map[string]func(string)),
		cbSem:    make(chan struct{}, 1),
		// cmd/stdin deliberately nil — a write would panic; the dead-check must
		// short-circuit before ever touching them.
	}
	p.markDead(errors.New("simulated crash"))

	rt := &remoteTool{
		plugin:     p,
		def:        agent.ToolDef{Name: "demo.tool"},
		remoteName: "tool",
	}

	// Run in a goroutine with a deadline so a regression that blocks (e.g.
	// waiting on a never-answered pending channel) fails loudly instead of
	// hanging the suite.
	type outcome struct {
		res agent.Result
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := rt.Invoke(context.Background(), json.RawMessage(`{}`))
		done <- outcome{res, err}
	}()

	select {
	case o := <-done:
		if o.err == nil {
			t.Fatal("Invoke on a dead plugin must return an error, not a result")
		}
		msg := o.err.Error()
		if !strings.Contains(msg, "unavailable") {
			t.Errorf("error %q should report the tool as unavailable", msg)
		}
		if !strings.Contains(msg, "simulated crash") {
			t.Errorf("error %q should surface the death cause (diagnosable, not silent)", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Invoke on a dead plugin hung — the dead-check must short-circuit, never block")
	}
}
