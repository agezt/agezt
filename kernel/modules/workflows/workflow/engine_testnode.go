// SPDX-License-Identifier: MIT

package workflow

// Single-node test (M811): run ONE node of a workflow with caller-supplied
// upstream data — n8n's "execute node". The node executes under the exact
// machinery of a real run (policy gates, reliability settings, governed
// tools, metered LLM calls); only the graph traversal is skipped. The
// journal gets a workflow.node event flagged test:true so the canvas's
// live panel lights up, while the run-history fold skips it (a test is not
// an arc).

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/event"
)

// TestNode executes one node of w against the supplied data map
// (keys are upstream node ids, values their {"output": …} shape; the
// "trigger" entry is derived from payload). The graph must validate — a
// half-edited canvas is refused with the validator's reason, not run on a
// guess. Halted kernels refuse.
func (e *Engine) TestNode(ctx context.Context, corr string, w Workflow, nodeID string, data map[string]any, payload any) (NodeTestResult, error) {
	if e.halted() {
		return NodeTestResult{}, ErrHalted
	}
	if err := Validate(w); err != nil {
		return NodeTestResult{}, err
	}
	node := w.NodeByID(strings.TrimSpace(nodeID))
	if node == nil {
		return NodeTestResult{}, fmt.Errorf("workflow: no node %q", nodeID)
	}
	if node.Type == NodeTrigger {
		return NodeTestResult{}, errors.New("workflow: the trigger does not execute — run the workflow instead")
	}

	if data == nil {
		data = map[string]any{}
	}
	// The trigger's payload always resolves, mock data or not.
	if _, ok := data["trigger"]; !ok {
		data["trigger"] = map[string]any{"payload": payload}
	}

	inputPreview := nodeInputPreview(node, data)
	output, port, attempts, err := e.execNodeWithReliability(ctx, corr, node, w, data, payload, Provenance{})

	nodePayload := map[string]any{
		"workflow": w.Name, "node": node.ID, "type": node.Type, "ok": err == nil,
		// test:true keeps single-node probes OUT of the run-history fold —
		// a test is not an arc — while the live canvas still reacts.
		"test": true,
	}
	if node.Label != "" {
		nodePayload["label"] = node.Label
	}
	if port != "" {
		nodePayload["port"] = port
	}
	if attempts > 1 {
		nodePayload["attempts"] = attempts
	}
	if inputPreview != "" {
		nodePayload["input"], _ = wfSnippet(inputPreview)
	}
	if err == nil {
		if snip, truncated := wfSnippet(output); snip != "" {
			nodePayload["output"] = snip
			if truncated {
				nodePayload["output_truncated"] = true
			}
		}
	} else {
		nodePayload["error"] = err.Error()
	}
	e.publish(event.Spec{
		Subject: "workflow." + w.Name, Kind: event.KindWorkflowNode, Actor: "workflow",
		CorrelationID: corr,
		Payload:       nodePayload,
	})
	if err != nil {
		return NodeTestResult{}, err
	}
	return NodeTestResult{Output: output, Port: port, Attempts: attempts}, nil
}
